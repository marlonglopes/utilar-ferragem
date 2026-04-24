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

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
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
