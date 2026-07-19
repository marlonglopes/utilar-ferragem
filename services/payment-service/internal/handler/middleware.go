package handler

import (
	"github.com/utilar/pkg/idempotency"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/requestid"
)

const RequestIDHeader = "X-Request-Id"

// RequestID gera/propaga X-Request-Id e injeta no contexto. Correlação entre
// payment-service, catalog-service e order-service via mesmo header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = requestid.New()
		}
		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)

		// Também no context.Context da request, não só no gin.Context.
		//
		// É ESTE passo que faz a correlação atravessar serviços: o transport de
		// pkg/httpclient lê o id do context e injeta X-Request-Id em toda chamada
		// service-to-service (payment→order, payment→auth, payment→PSP). Sem
		// isso o id morria aqui e um checkout virava 3 traços desconexos.
		c.Request = c.Request.WithContext(requestid.NewContext(c.Request.Context(), id))

		c.Next()
	}
}

// AccessLog emite uma linha JSON por requisição.
//
// SEGURANÇA (M5): logamos `c.FullPath()` (route pattern, ex `/payments/:id`)
// em vez de `c.Request.URL.Path` (path real com IDs). Isso evita PII em
// agregadores de log. Query string NÃO é logada por padrão; se for adicionar
// no futuro, passe por redactLogValue() primeiro.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		// user_id entra na linha de log (M-OBS): sem ele não dá pra responder
		// "o que este usuário fez na última hora" numa investigação, e é a
		// segunda dimensão de correlação depois do request_id. É um UUID
		// opaco — não é PII por si só (nome/email/CPF continuam fora).
		slog.Info("http",
			"request_id", c.GetString("request_id"),
			"user_id", c.GetString("user_id"),
			"role", c.GetString("user_role"),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", c.ClientIP(),
		)
	}
}

// CORS — whitelist via lista de origens. Vazio = wildcard (dev).
func CORS(allowed []string) gin.HandlerFunc {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		allowedSet[o] = struct{}{}
	}
	wildcard := len(allowed) == 0

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case wildcard:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "":
			if _, ok := allowedSet[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Idempotency-Key precisa estar aqui: o SPA passa a enviá-lo em POST
		// /orders e /payments pra evitar cobrança duplicada no duplo clique.
		// Header não declarado faz o PREFLIGHT falhar e a requisição nem sai —
		// o usuário vê "Failed to fetch" no Pix e no boleto, sem log no servidor.
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, "+RequestIDHeader+", "+idempotency.HeaderName)
		c.Header("Access-Control-Expose-Headers", RequestIDHeader)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeaders — baseline defensivo (CSP, HSTS, X-Frame-Options, etc).
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'none'")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}
