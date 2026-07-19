package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/auth"
	"github.com/utilar/pkg/requestid"
	"github.com/utilar/pkg/servicetoken"
)

// InternalAuth protege as rotas /api/v1/internal, consultadas máquina-a-máquina
// pelo order-service (contexto autoritativo do operador de balcão).
//
// A1 (auditoria 2026-07-18) — dois caminhos, dois segredos:
//
//  1. Token de SERVIÇO assinado com serviceSecret (pkg/servicetoken): entra
//     como role=service. É o caminho normal.
//  2. Token de usuário assinado com userSecret: entra apenas se for `admin`,
//     para operação manual e suporte.
//
// O que deixa de existir: token assinado com o JWT_SECRET de usuário
// carregando `role=service`. O caminho (1) recusa pela assinatura e o (2) pela
// role — e o JWTAuth acima recusa a claim de saída. Como o assistant-service
// (Alice), que é público e o mais exposto, não recebe o SERVICE_JWT_SECRET,
// comprometê-lo não abre mais estas rotas.
func InternalAuth(userSecret, serviceSecret string, denyList *AccessTokenDenyList) gin.HandlerFunc {
	userAuth := JWTAuth(userSecret, denyList)

	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			Unauthorized(c, "missing bearer token")
			c.Abort()
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")

		// 1) Token de serviço.
		if sub, err := servicetoken.Parse(raw, serviceSecret); err == nil {
			c.Set("user_id", sub)
			c.Set("user_role", servicetoken.Role)
			c.Next()
			return
		}

		// 2) Token de usuário — JWTAuth valida assinatura, deny-list e já recusa
		// a claim role=service. RequireRole("admin") na cadeia faz a autorização.
		userAuth(c)
	}
}

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
//
// L-AUTH-2: se denyList é não-nil, consulta também o deny-list (logout via
// Redis). Tokens emitidos antes do logout do usuário são rejeitados mesmo
// dentro do TTL de 15min. denyList nil = comportamento clássico (sem lookup).
func JWTAuth(secret string, denyList *AccessTokenDenyList) gin.HandlerFunc {
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
		if denyList != nil && claims.IssuedAt != nil {
			if denyList.IsRevoked(c.Request.Context(), claims.UserID, claims.IssuedAt.Unix()) {
				Unauthorized(c, "token revoked")
				c.Abort()
				return
			}
		}
		// A1 (auditoria 2026-07-18): `role=service` é identidade de MÁQUINA e só
		// existe assinada com o SERVICE_JWT_SECRET (ver InternalAuth). Um token
		// de usuário carregando essa claim é, por construção, tentativa de usar
		// o segredo de usuário como se fosse o de serviço.
		if claims.Role == servicetoken.Role {
			slog.Warn("token de usuário com role=service recusado",
				"user_id", claims.UserID, "request_id", c.GetString("request_id"))
			Unauthorized(c, "invalid token")
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
		// Contexto de loja — presente só em tokens de operador de balcão.
		// É HINT de escopo, nunca de valor: o teto de desconto não vem daqui
		// (ver o comentário de Claims em internal/auth/jwt.go).
		c.Set("store_id", claims.StoreID)
		c.Set("store_level", claims.StoreLevel)
		c.Next()
	}
}

// RequireRole restringe uma rota a um conjunto de papéis. Deve vir DEPOIS de
// JWTAuth na cadeia — lê `user_role` do contexto, não do header, para que só
// exista um lugar onde o token é validado.
//
// 401 vs 403: sem `user_role` no contexto significa que JWTAuth não rodou ou
// não autenticou (401). Com papel presente mas fora da lista, é decisão de
// autorização (403) — distinguir importa para o frontend saber se manda o
// usuário para o login ou mostra "acesso negado".
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	wanted := strings.Join(roles, " or ")

	return func(c *gin.Context) {
		role := c.GetString("user_role")
		if role == "" {
			Unauthorized(c, "authentication required")
			c.Abort()
			return
		}
		if _, ok := allowed[role]; !ok {
			Forbidden(c, wanted+" role required")
			c.Abort()
			return
		}
		c.Next()
	}
}
