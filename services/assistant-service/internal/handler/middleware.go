package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/pkg/servicetoken"
)

// MaxRequestBytes — teto do body cru do /chat, antes de qualquer parse.
//
// Dimensionado pelos tetos do handler: maxHistoryBytes (16k) + maxMessage
// (2000 runes, até 4 bytes cada = 8k) + overhead de JSON, com folga. Sem isso,
// um atacante faz o servidor ler e alocar centenas de MB só pra o handler
// descobrir depois que ia truncar tudo — o custo de memória já foi pago.
const MaxRequestBytes = 64 * 1024

// LimitBody envolve o Body em http.MaxBytesReader. Estourou o teto, a leitura
// falha e o ShouldBindJSON do handler devolve 400.
func LimitBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}

// CORS — whitelist por vírgula (ALLOWED_ORIGINS), igual aos outros serviços.
//
// Diferença deliberada em relação ao comportamento anterior: lista vazia NÃO é
// mais wildcard. `Access-Control-Allow-Origin: *` num endpoint que gasta
// orçamento de LLM significa que qualquer página da internet pode disparar
// requests com o navegador do visitante. Agora o wildcard só existe com
// DEV_MODE=true; em produção, lista vazia = nenhuma origem liberada.
func CORS(allowed string, devMode bool) gin.HandlerFunc {
	set := map[string]struct{}{}
	for _, o := range strings.Split(allowed, ",") {
		if v := strings.TrimSpace(o); v != "" {
			set[v] = struct{}{}
		}
	}
	wildcard := len(set) == 0 && devMode

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case wildcard:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "":
			if _, ok := set[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// OptionalAuth lê um Bearer JWT se houver e seta `user_id` no contexto.
//
// DECISÃO: a Alice NÃO exige autenticação. Ela existe pra atender o visitante
// que ainda não tem conta — é justamente aí que ela vende. Exigir JWT mataria o
// produto pra converter visitante em cliente.
//
// A compensação é o rate limit em dois níveis (ver TieredRateLimit): anônimo
// leva a cota apertada, autenticado leva a folgada. Por isso o token, quando
// vem, é VALIDADO de verdade (assinatura HS256, mesmo lock do order/catalog) —
// se bastasse mandar um Bearer qualquer, o atacante só teria que inventar um
// para pular direto pro balde folgado. Token inválido não derruba a request:
// vira anônimo e segue, porque um JWT expirado no meio de uma conversa não
// deve virar erro de chat.
func OptionalAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if jwtSecret != "" && strings.HasPrefix(auth, "Bearer ") {
			if sub, papel, loja, err := parseClaims(strings.TrimPrefix(auth, "Bearer "), jwtSecret); err == nil {
				// A1 (auditoria 2026-07-18): `role=service` é identidade de
				// máquina e vive noutro segredo (SERVICE_JWT_SECRET), que a Alice
				// deliberadamente NÃO recebe — ela é o serviço mais exposto do
				// conjunto e não precisa emitir nem consumir token de serviço.
				// Um token com essa claim chegando aqui vira anônimo, nunca um
				// papel privilegiado.
				if papel == servicetoken.Role {
					c.Next()
					return
				}
				c.Set("user_id", sub)
				// user_role decide o MODO da Alice (cliente vs. balcão) e, com
				// ele, se custo e margem podem ser vistos. Por isso só é setado
				// aqui, depois da verificação de assinatura — nunca a partir de
				// header, query ou corpo, que são entrada do usuário.
				c.Set("user_role", papel)
				c.Set("store_id", loja)
			}
		}
		c.Next()
	}
}

// parseClaims valida o JWT HS256 e extrai `sub`, `role` e `store_id`.
//
// O lock estrito no algoritmo importa ainda mais agora: com o modo vendedor, um
// token forjado não daria só cota folgada de rate limit — daria acesso ao custo
// e à margem. Aceitar `alg: none` ou confundir HS256 com RS256 aqui seria
// entregar a estrutura de custo da loja para qualquer um.
func parseClaims(tokenStr, secret string) (sub, papel, loja string, err error) {
	token, e := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if e != nil {
		return "", "", "", e
	}
	if !token.Valid {
		return "", "", "", errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", "", errors.New("invalid claims")
	}
	sub, _ = claims["sub"].(string)
	if sub == "" {
		return "", "", "", errors.New("missing sub claim")
	}
	papel, _ = claims["role"].(string)
	loja, _ = claims["store_id"].(string)
	return sub, papel, loja, nil
}

// parseSubject valida o JWT HS256 e extrai `sub`. Lock estrito no algoritmo
// (mesma defesa do order-service/catalog-service contra alg confusion).
func parseSubject(tokenStr, secret string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing sub claim")
	}
	return sub, nil
}

// TieredRateLimit escolhe entre dois limiters conforme a request esteja
// autenticada (user_id setado por OptionalAuth) ou não.
//
// pkg/ratelimit.Middleware carrega um Limit fixo, então em vez de mudar o pkg
// compartilhado (usado por 4 serviços) montamos os dois middlewares e
// despachamos aqui. Os baldes também são separados por prefixo, senão um IP
// compartilhado (NAT/escritório) consumiria a cota dos usuários logados.
func TieredRateLimit(anon, authed gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("user_id") != "" {
			authed(c)
			return
		}
		anon(c)
	}
}
