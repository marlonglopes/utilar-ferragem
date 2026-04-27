// Package ratelimit é um rate limiter de janela fixa baseado em Redis.
//
// Compartilhado entre auth-service, catalog-service, order-service e
// payment-service via Go workspace (go.work). Não é necessário extrair pra
// repositório separado — o pkg/ é específico do utilar.
//
// Por que janela fixa em vez de token bucket:
//   - Implementação trivial: INCR + EXPIRE no primeiro hit. 1 round-trip.
//   - Suficiente pra todos os endpoints listados na auditoria (5–100 req/min).
//   - Token bucket via Lua é mais preciso mas overkill aqui — o objetivo é
//     bloquear brute-force, não shaping fino de tráfego.
//
// Resolve A6-H2 (auth login/forgot/reset/verify), CT1-H1 (catalog search),
// H4 (payment create).
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Limit define a regra: até `Max` requisições em `Window` por chave.
type Limit struct {
	Max    int
	Window time.Duration
}

// Limiter aplica regras de Limit em cima de um Redis client.
type Limiter struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb} }

// Errors públicos pra callers em testes.
var (
	ErrLimitExceeded = errors.New("ratelimit: limit exceeded")
)

// Allow incrementa o contador da `key` e retorna (allowed, retryAfter).
//
// Janela fixa: a primeira requisição em uma janela cria a chave com TTL=Window.
// Próximas reqs na mesma janela só fazem INCR (não estendem TTL). Quando a
// janela expira, a chave some e o contador zera.
//
// retryAfter é o tempo restante até a chave expirar — informação opcional
// pro Retry-After header quando bloqueia.
func (l *Limiter) Allow(ctx context.Context, key string, lim Limit) (bool, time.Duration, error) {
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, lim.Window) // só "vinga" se a chave foi criada agora (NX-like via TTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("ratelimit redis: %w", err)
	}
	count := incr.Val()
	if count > int64(lim.Max) {
		ttl, _ := l.rdb.TTL(ctx, key).Result()
		if ttl < 0 {
			ttl = lim.Window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

// KeyFunc extrai a chave de identidade de uma requisição (IP, user_id, etc).
type KeyFunc func(*gin.Context) string

// IPKey usa o IP do cliente — bom pra endpoints sem auth (login, search).
// Em prod atrás de proxy, requer TrustedProxies + ForwardedByClientIP no gin.
func IPKey(c *gin.Context) string { return c.ClientIP() }

// UserKey usa o user_id setado no contexto pelo middleware de auth.
// Falla pra IP se user_id estiver vazio (ex: middleware ainda não rodou).
func UserKey(c *gin.Context) string {
	if uid := c.GetString("user_id"); uid != "" {
		return uid
	}
	return c.ClientIP()
}

// Middleware retorna um gin.HandlerFunc que aplica o limit no escopo do
// `prefix`. Bloqueio = HTTP 429 com Retry-After (segundos).
//
// `prefix` deve ser estável e único por rota lógica — ex: "auth:login".
// Chave final = "rl:{prefix}:{keyFunc(c)}".
//
// Se Redis estiver fora do ar, **fail-open** (allow): logar mas não bloquear.
// Justificativa: tirar Redis nunca pode quebrar /auth/login. Se o atacante
// estourar Redis pra bypass do limiter, ele já tem outros problemas piores.
func Middleware(l *Limiter, prefix string, lim Limit, keyFn KeyFunc) gin.HandlerFunc {
	if keyFn == nil {
		keyFn = IPKey
	}
	return func(c *gin.Context) {
		key := strings.Join([]string{"rl", prefix, keyFn(c)}, ":")
		allowed, retryAfter, err := l.Allow(c.Request.Context(), key, lim)
		if err != nil {
			// Fail-open + log
			c.Set("ratelimit_error", err.Error())
			c.Next()
			return
		}
		if !allowed {
			seconds := int(retryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  "rate_limited",
			})
			return
		}
		c.Next()
	}
}
