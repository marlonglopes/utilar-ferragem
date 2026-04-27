package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/auth"
)

const RequestIDHeader = "X-Request-Id"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

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

// CORS retorna um middleware com whitelist de origens. Se `allowed` é vazio,
// libera "*" (modo dev/legacy). Em prod, passar lista de origens explícita
// (ex: ["https://utilarferragem.com.br","https://www.utilarferragem.com.br"]).
//
// Esse padrão é replicado nos 4 services. Origens não-whitelisted recebem
// header `Access-Control-Allow-Origin` ausente — browser bloqueia request
// cross-origin.
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

		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, "+RequestIDHeader)
		c.Header("Access-Control-Expose-Headers", RequestIDHeader)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeaders adiciona um conjunto baseline de headers defensivos:
//   - X-Content-Type-Options: nosniff (MIME-sniffing)
//   - X-Frame-Options: DENY (clickjacking)
//   - Content-Security-Policy: default-src 'none' (esse serviço só serve JSON)
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains (HTTPS-only — só efetivo se servido via HTTPS)
//   - Referrer-Policy: no-referrer (não vaza URL pra third-parties)
//
// Esse padrão é transversal aos 4 services backend.
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

// JWTAuth valida `Authorization: Bearer <jwt>` e injeta user_id/email/role
// no contexto. 401 em token ausente/inválido/expirado.
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			Unauthorized(c, "missing bearer token")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(h, "Bearer ")
		claims, err := auth.ParseAccessToken(tokenStr, secret)
		if err != nil {
			Unauthorized(c, "invalid token: "+err.Error())
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
		c.Next()
	}
}

func newRequestID() string {
	const hex = "0123456789abcdef"
	n := time.Now().UnixNano()
	buf := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		buf[i] = hex[n&0xf]
		n >>= 4
	}
	return string(buf)
}
