package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-Id, "+RequestIDHeader)
		c.Header("Access-Control-Expose-Headers", RequestIDHeader)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequireUser valida o usuário por JWT (Authorization: Bearer <token>).
// Quando devMode=true, aceita também o fallback X-User-Id pra facilitar tests
// e desenvolvimento sem auth-service rodando. Em produção devMode=false e o
// fallback é rejeitado (audit O1-C3 — IDOR trivial via header).
func RequireUser(jwtSecret string, devMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) JWT (caminho de produção)
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr := strings.TrimPrefix(auth, "Bearer ")
			sub, err := parseJWTSubject(tokenStr, jwtSecret)
			if err != nil {
				slog.Warn("invalid jwt", "error", err.Error(), "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			c.Set("user_id", sub)
			c.Set("auth_source", "jwt")
			c.Next()
			return
		}
		// 2) X-User-Id fallback — só em dev
		if devMode {
			if userID := c.GetHeader("X-User-Id"); userID != "" {
				c.Set("user_id", userID)
				c.Set("auth_source", "x-user-id")
				c.Next()
				return
			}
		}
		Unauthorized(c, "missing or invalid Authorization header")
		c.Abort()
	}
}

// parseJWTSubject extrai a claim `sub` do JWT HS256 (compatível com auth-service.Claims).
func parseJWTSubject(tokenStr, secret string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = errors.New("invalid token")
		}
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", errors.New("missing sub claim")
	}
	return sub, nil
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
