package handler

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/pkg/servicetoken"
)

// RequireAdmin protege as rotas de escrita de catálogo (/admin/*). Exige um JWT
// válido do auth-service com claim `role=admin`. Em DevMode, aceita o fallback
// dos headers X-User-Id + X-User-Role pra facilitar testes sem auth-service.
//
// Espelha o padrão do order-service (RequireUser + lock HS256), estendido pra
// checar o papel do usuário.
func RequireAdmin(jwtSecret string, devMode bool) gin.HandlerFunc {
	return RequireRole(jwtSecret, devMode, "admin")
}

// RequireRole é a versão genérica de RequireAdmin: aceita qualquer uma das
// roles listadas. Extraído quando as rotas internas de reserva de estoque
// passaram a precisar de `role=service` (token que o order-service assina com
// o JWT_SECRET compartilhado) além de `admin` — duplicar o middleware inteiro
// só pra trocar a string da role era convite a divergir na próxima mudança de
// segurança.
//
// A mensagem de erro cita as roles aceitas pra o operador não precisar ler o
// código quando toma 403 num script de integração.
func RequireRole(jwtSecret string, devMode bool, roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	wanted := strings.Join(roles, " or ")

	return func(c *gin.Context) {
		// 1) JWT (caminho de produção)
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			sub, role, err := parseJWTClaims(strings.TrimPrefix(auth, "Bearer "), jwtSecret)
			if err != nil {
				slog.Warn("auth: invalid jwt", "error", err.Error(), "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			// A1 (auditoria 2026-07-18): `role=service` NUNCA vale quando o token
			// foi verificado com o segredo de USUÁRIO. Identidade de serviço só
			// existe assinada com o SERVICE_JWT_SECRET, e essa checagem está em
			// RequireInternal. Sem esta recusa, qualquer processo com o
			// JWT_SECRET — a Alice, por exemplo — voltaria a poder se declarar
			// serviço.
			if role == servicetoken.Role {
				slog.Warn("auth: token de usuário com role=service recusado",
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

		// 2) Fallback dev — headers explícitos, só quando DevMode=true.
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

// RequireInternal protege as rotas /api/v1/internal (reserva de estoque),
// chamadas máquina-a-máquina pelo order-service.
//
// A1 (auditoria 2026-07-18) — a diferença que importa: são DOIS segredos com
// propósitos distintos, e cada caminho só aceita o seu.
//
//  1. Token de SERVIÇO: assinatura conferida com serviceSecret, `iss` e
//     `role=service` verificados (ver pkg/servicetoken). É o caminho normal.
//  2. Token de ADMIN humano: assinatura conferida com o jwtSecret de usuário,
//     mantido para operação manual e suporte.
//
// O que deixa de ser possível: um token assinado com o JWT_SECRET de usuário
// carregando `role=service`. O caminho (1) recusa por assinatura, o (2) só
// admite `admin`. Como o assistant-service (Alice) — o serviço mais exposto —
// não recebe o SERVICE_JWT_SECRET, comprometê-lo não dá mais acesso às rotas
// internas do catálogo.
//
// Em DevMode o fallback de header continua, igual ao RequireRole, para rodar
// teste e smoke sem subir o order-service. Isso é seguro porque o pkg/devguard
// recusa DEV_MODE em qualquer ambiente com sinal de produção.
func RequireInternal(jwtSecret, serviceSecret string, devMode bool) gin.HandlerFunc {
	return requireServiceOrRoles(jwtSecret, serviceSecret, devMode, "admin")
}

// RequireStore protege as rotas /api/v1/store — a leitura autenticada que o PDV
// de balcão faz.
//
// PORQUÊ existe separada de RequireAdmin: o custo de aquisição precisa chegar
// ao operador do balcão. Sem isso o PDV estima o custo como `preço × 0,72` e a
// barra de margem mente — num caso medido, custo real dava 60% de margem e a
// estimativa dava 28%. São 32 pontos, e é esse número que o vendedor usa pra
// decidir até onde pode dar desconto.
//
// PORQUÊ não foi só somar `store_operator` ao RequireAdmin do grupo /admin:
// aquele grupo tem ESCRITA (criar produto, mudar preço, importar planilha).
// Operador de balcão não pode escrever no catálogo. Rota nova, papel novo,
// superfície mínima.
//
// `role=service` também passa (mesma checagem de segredo separado do
// RequireInternal) porque o order-service precisa do custo pra registrar o CMV
// do pedido de balcão sem fazer SELECT no banco do catálogo.
func RequireStore(jwtSecret, serviceSecret string, devMode bool) gin.HandlerFunc {
	return requireServiceOrRoles(jwtSecret, serviceSecret, devMode, "store_operator", "admin")
}

// requireServiceOrRoles é o tronco comum de RequireInternal e RequireStore:
// aceita identidade de SERVIÇO (assinada com o serviceSecret) ou identidade de
// USUÁRIO com um dos papéis listados (assinada com o jwtSecret).
//
// Extraído quando a rota de custo do balcão precisou exatamente da mesma
// separação de segredos com outra lista de papéis. Duplicar o middleware seria
// convite a divergir na próxima mudança de segurança — e é justamente aqui que
// divergir custa caro.
func requireServiceOrRoles(jwtSecret, serviceSecret string, devMode bool, roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	wanted := "service or " + strings.Join(roles, " or ") + " role required"

	return func(c *gin.Context) {
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			raw := strings.TrimPrefix(auth, "Bearer ")

			// 1) Token de serviço — assinado com o segredo de SERVIÇO.
			if sub, err := servicetoken.Parse(raw, serviceSecret); err == nil {
				c.Set("user_id", sub)
				c.Set("user_role", servicetoken.Role)
				c.Next()
				return
			}

			// 2) Token de usuário — só os papéis da lista entram aqui.
			sub, role, err := parseJWTClaims(raw, jwtSecret)
			if err != nil {
				slog.Warn("auth: invalid jwt em rota interna",
					"error", err.Error(), "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			// `role=service` NUNCA vale aqui: o caminho (1) já teria aceitado se
			// a assinatura fosse a de serviço. Chegar até aqui com essa claim é
			// tentativa de usar o segredo de usuário como se fosse o de serviço —
			// exatamente o ataque que A1 descreve. A recusa é explícita para que
			// uma lista de papéis futura não possa incluir "service" por engano.
			if role == servicetoken.Role {
				slog.Warn("auth: token de usuário com role=service recusado em rota interna",
					"sub", sub, "request_id", c.GetString("request_id"))
				Unauthorized(c, "invalid token")
				c.Abort()
				return
			}
			if _, ok := allowed[role]; !ok {
				slog.Warn("auth: rota interna negada",
					"role", role, "sub", sub, "request_id", c.GetString("request_id"))
				Forbidden(c, wanted)
				c.Abort()
				return
			}
			c.Set("user_id", sub)
			c.Set("user_role", role)
			c.Next()
			return
		}

		// 3) Fallback dev — headers explícitos, só com DevMode.
		if devMode {
			hdr := c.GetHeader("X-User-Role")
			if _, ok := allowed[hdr]; ok || hdr == servicetoken.Role {
				c.Set("user_id", c.GetHeader("X-User-Id"))
				c.Set("user_role", hdr)
				c.Next()
				return
			}
		}

		Unauthorized(c, "missing or invalid Authorization header")
		c.Abort()
	}
}

// parseJWTClaims extrai `sub` e `role` do JWT HS256 (compatível com auth-service.Claims).
// Lock estrito no algoritmo HS256 (mesma defesa do order-service, A16-M7).
func parseJWTClaims(tokenStr, secret string) (sub, role string, err error) {
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
