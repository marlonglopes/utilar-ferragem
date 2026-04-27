package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/requestid"
)

// RequestIDHeader é o header padrão de correlação entre serviços.
const RequestIDHeader = "X-Request-Id"

// RequestID gera/propaga X-Request-Id e injeta no contexto do Gin.
// Foundation para logging estruturado e tracing (ver Sprint 22).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = requestid.New()
		}
		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// AccessLog emite uma linha JSON por requisição com método, rota, status, duração.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			"request_id", c.GetString("request_id"),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", c.ClientIP(),
		)
	}
}

// CORS — whitelist via lista de origens. Vazio = wildcard (dev).
// Catalog é público read-only mas mesmo assim recomenda-se restringir em prod
// pra prevenir CSRF caso endpoints de write sejam adicionados depois.
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

		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, "+RequestIDHeader)
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

