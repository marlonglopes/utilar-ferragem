// Package idempotency implementa o middleware Idempotency-Key (RFC inspired,
// padrão Stripe) sobre Redis.
//
// Caso de uso: o cliente envia `Idempotency-Key: <uuid>` em POST /payments.
// Se a mesma key chega duas vezes (retry, browser duplo-clique, network
// glitch), o servidor devolve a MESMA resposta (status + body) da primeira,
// sem reexecutar o handler. Evita pagamentos duplicados.
//
// Resolve H1 do payment-service.
//
// Estratégia de concorrência:
//
//	t=0:  cliente A: SETNX key "lock"  → OK, executa handler
//	t=10: cliente B: SETNX key "lock"  → falha, GET key
//	      se "lock":   responde 409 conflict (handler em voo)
//	      se {payload}: responde com o body cacheado
//
// Esse handshake garante que mesmo com 2 requisições disparadas em paralelo
// só uma executa o handler.
package idempotency

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	// HeaderName é o header HTTP que carrega a chave.
	HeaderName = "Idempotency-Key"

	// inFlightSentinel marca uma chave reservada (handler ainda executando).
	inFlightSentinel = "__inflight__"

	// minKeyLen / maxKeyLen rejeitam chaves obviamente inválidas.
	minKeyLen = 8
	maxKeyLen = 128
)

// Store grava e lê responses cacheadas no Redis.
type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

// New cria um Store. ttl recomendado: 24h (padrão Stripe).
func New(rdb *redis.Client, ttl time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl}
}

// CachedResponse é o que cacheamos. Headers são opcionais (alguns endpoints
// retornam Location, etc).
type CachedResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body"`
}

// Errors públicos.
var (
	ErrInFlight  = errors.New("idempotency: request in flight")
	ErrInvalidKey = errors.New("idempotency: invalid key")
)

// reservedResponseKey monta a chave Redis a partir do prefix da rota + key do header.
func reservedResponseKey(prefix, userKey string) string {
	return strings.Join([]string{"idem", prefix, userKey}, ":")
}

// Reserve tenta marcar a chave como em-voo (SETNX). Retorna:
//   - reserved=true: caller deve executar o handler.
//   - reserved=false: já existe. Caller deve buscar e replay.
func (s *Store) Reserve(ctx context.Context, key string) (bool, error) {
	ok, err := s.rdb.SetNX(ctx, key, inFlightSentinel, s.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency redis: %w", err)
	}
	return ok, nil
}

// Save sobrescreve o sentinel pelo response final.
func (s *Store) Save(ctx context.Context, key string, resp *CachedResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, key, b, s.ttl).Err(); err != nil {
		return fmt.Errorf("idempotency redis save: %w", err)
	}
	return nil
}

// Lookup retorna o CachedResponse, ou ErrInFlight se ainda não foi gravado.
func (s *Store) Lookup(ctx context.Context, key string) (*CachedResponse, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, redis.Nil
	}
	if err != nil {
		return nil, fmt.Errorf("idempotency redis lookup: %w", err)
	}
	if val == inFlightSentinel {
		return nil, ErrInFlight
	}
	var cr CachedResponse
	if err := json.Unmarshal([]byte(val), &cr); err != nil {
		return nil, fmt.Errorf("idempotency decode: %w", err)
	}
	return &cr, nil
}

// Release apaga a chave. Usado em fail-cases pra que o cliente possa retry.
func (s *Store) Release(ctx context.Context, key string) {
	_ = s.rdb.Del(ctx, key).Err()
}

// captureWriter é um ResponseWriter que armazena status + body pra cache.
type captureWriter struct {
	gin.ResponseWriter
	buf    bytes.Buffer
	status int
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	cw.buf.Write(b)
	return cw.ResponseWriter.Write(b)
}

// Middleware aplica idempotency em rotas POST. `prefix` deve ser único por
// rota lógica (ex: "payment:create"). Handler só roda uma vez por key+prefix
// dentro do TTL.
//
// Comportamento:
//   - Sem header `Idempotency-Key` → passa direto (idempotency é opt-in pelo cliente).
//   - Header presente, primeira vez → reserva, executa, cacheia response.
//   - Header presente, hit (200/4xx) → replay do response cacheado.
//   - Header presente, in-flight → 409 Conflict.
//
// Fail-open com Redis fora: se o store falhar, executa o handler sem cache
// (mesma justificativa do ratelimit).
func Middleware(s *Store, prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userKey := c.GetHeader(HeaderName)
		if userKey == "" {
			c.Next()
			return
		}
		if l := len(userKey); l < minKeyLen || l > maxKeyLen {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Idempotency-Key length must be %d..%d chars", minKeyLen, maxKeyLen),
				"code":  "invalid_idempotency_key",
			})
			return
		}

		ctx := c.Request.Context()
		key := reservedResponseKey(prefix, userKey)

		// Tenta reservar. Se já existe, replay ou conflict.
		reserved, err := s.Reserve(ctx, key)
		if err != nil {
			// Fail-open: log via context (caller pode adicionar middleware de log).
			c.Set("idempotency_error", err.Error())
			c.Next()
			return
		}
		if !reserved {
			cached, err := s.Lookup(ctx, key)
			if errors.Is(err, ErrInFlight) {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{
					"error": "request with this Idempotency-Key already in flight",
					"code":  "idempotency_conflict",
				})
				return
			}
			if err != nil && !errors.Is(err, redis.Nil) {
				c.Set("idempotency_error", err.Error())
				c.Next()
				return
			}
			if cached == nil {
				// Race extremamente improvável: reserva expirou entre Reserve e Lookup.
				// Fail-open: deixa o handler rodar.
				c.Next()
				return
			}
			for k, v := range cached.Headers {
				c.Header(k, v)
			}
			c.Header("Idempotent-Replayed", "true")
			c.Data(cached.Status, c.GetHeader("Accept"), cached.Body)
			c.Abort()
			return
		}

		// Reservado por nós — executa o handler capturando a resposta.
		cw := &captureWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = cw
		c.Next()

		// Salva apenas se o handler produziu resposta significativa (não ignora erros 5xx
		// — se 5xx é determinístico, replay também é correto. Se for transiente, cliente
		// deveria retry com nova key).
		resp := &CachedResponse{
			Status: cw.status,
			Body:   cw.buf.Bytes(),
		}
		if err := s.Save(ctx, key, resp); err != nil {
			c.Set("idempotency_error", err.Error())
		}
	}
}
