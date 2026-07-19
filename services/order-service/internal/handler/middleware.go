package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/pkg/requestid"
	"github.com/utilar/pkg/servicetoken"
)

const RequestIDHeader = "X-Request-Id"

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

// CORS — whitelist via lista de origens explicitas. Vazio = wildcard (dev/legacy).
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
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-Id, "+RequestIDHeader)
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

// RequireUser valida o usuário por JWT (Authorization: Bearer <token>).
// Quando devMode=true, aceita também o fallback X-User-Id pra facilitar tests
// e desenvolvimento sem auth-service rodando. Em produção devMode=false e o
// fallback é rejeitado (audit O1-C3 — IDOR trivial via header).
func RequireUser(jwtSecret string, devMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1) JWT (caminho de produção)
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr := strings.TrimPrefix(auth, "Bearer ")
			id, err := parseJWTIdentity(tokenStr, jwtSecret)
			if err != nil {
				slog.Warn("invalid jwt", "error", err.Error(), "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			// A1 (auditoria 2026-07-18): identidade de SERVIÇO não passa por
			// rota de usuário. O order-service não expõe nenhuma rota interna,
			// então um token role=service aqui só pode ser tentativa de usar o
			// JWT_SECRET de usuário como se fosse o de serviço.
			if id.Role == servicetoken.Role {
				slog.Warn("token role=service recusado em rota de usuário",
					"sub", id.Sub, "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			setIdentity(c, id)
			c.Set("auth_source", "jwt")
			c.Next()
			return
		}
		// 2) X-User-Id fallback — só em dev
		if devMode {
			if userID := c.GetHeader("X-User-Id"); userID != "" {
				setIdentity(c, identity{
					Sub:        userID,
					Role:       c.GetHeader("X-User-Role"),
					StoreID:    c.GetHeader("X-Store-Id"),
					StoreLevel: c.GetHeader("X-Store-Level"),
				})
				c.Set("auth_source", "x-user-id")
				c.Next()
				return
			}
		}
		Unauthorized(c, "missing or invalid Authorization header")
		c.Abort()
	}
}

// RequireRole protege as rotas de operação (/admin/orders/*). Exige JWT com
// claim `role` entre as aceitas.
//
// Espelha o RequireRole do catalog-service. Em DevMode aceita o fallback
// X-User-Id + X-User-Role, mesmo padrão do RequireUser acima — e pelo mesmo
// motivo: rodar smoke test sem subir o auth-service.
func RequireRole(jwtSecret string, devMode bool, roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	wanted := strings.Join(roles, " or ")

	return func(c *gin.Context) {
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			sub, role, err := parseJWTSubjectRole(strings.TrimPrefix(auth, "Bearer "), jwtSecret)
			if err != nil {
				slog.Warn("invalid jwt", "error", err.Error(), "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			// A1: mesma recusa do RequireUser — `role=service` assinado com o
			// segredo de usuário nunca vale.
			if role == servicetoken.Role {
				slog.Warn("token role=service recusado em rota de papel",
					"sub", sub, "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			if _, ok := allowed[role]; !ok {
				Forbidden(c, wanted+" role required")
				c.Abort()
				return
			}
			c.Set("user_id", sub)
			c.Set("user_role", role)
			c.Next()
			return
		}

		if devMode {
			if hdr := c.GetHeader("X-User-Role"); hdr != "" {
				if _, ok := allowed[hdr]; ok {
					c.Set("user_id", c.GetHeader("X-User-Id"))
					c.Set("user_role", hdr)
					c.Next()
					return
				}
			}
		}

		Unauthorized(c, "missing or invalid Authorization header")
		c.Abort()
	}
}

// parseJWTSubjectRole extrai `sub` e `role`. Mesmo lock HS256 (A16-M7).
func parseJWTSubjectRole(tokenStr, secret string) (sub, role string, err error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = errors.New("invalid token")
		}
		return "", "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", errors.New("invalid claims")
	}
	sub, _ = claims["sub"].(string)
	role, _ = claims["role"].(string)
	if sub == "" {
		return "", "", errors.New("missing sub claim")
	}
	return sub, role, nil
}

// identity é o que o order-service extrai do token.
//
// StoreID/StoreLevel são HINT DE ESCOPO do operador de balcão — nunca de valor.
// O teto de desconto (que é dinheiro) não está aqui de propósito: vem do
// auth-service no momento da decisão. Ver internal/authclient.
type identity struct {
	Sub        string
	Role       string
	StoreID    string
	StoreLevel string
}

func setIdentity(c *gin.Context, id identity) {
	c.Set("user_id", id.Sub)
	c.Set("user_role", id.Role)
	c.Set("store_id", id.StoreID)
	c.Set("store_level", id.StoreLevel)
}

// parseJWTIdentity extrai sub/role/store do JWT HS256 (compatível com
// auth-service.Claims). A16-M7: lock estrito no algoritmo HS256.
func parseJWTIdentity(tokenStr, secret string) (identity, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = errors.New("invalid token")
		}
		return identity{}, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return identity{}, errors.New("invalid claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return identity{}, errors.New("missing sub claim")
	}
	role, _ := claims["role"].(string)
	storeID, _ := claims["store_id"].(string)
	storeLevel, _ := claims["store_level"].(string)
	return identity{Sub: sub, Role: role, StoreID: storeID, StoreLevel: storeLevel}, nil
}

// parseJWTSubject continua existindo para os call sites que só querem o `sub`.
func parseJWTSubject(tokenStr, secret string) (string, error) {
	id, err := parseJWTIdentity(tokenStr, secret)
	return id.Sub, err
}
