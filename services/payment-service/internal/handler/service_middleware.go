package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/servicetoken"
)

// RequireService protege as rotas /internal — tráfego SERVIÇO→SERVIÇO.
//
// PORQUÊ um middleware próprio e não JWTMiddleware + checagem de papel:
// identidade de máquina vive noutro segredo (SERVICE_JWT_SECRET, ver
// pkg/servicetoken). JWTMiddleware recusa `role=service` de propósito — um
// token de USUÁRIO com essa claim só pode ser tentativa de escalar papel. Aqui
// é o inverso: só passa quem foi assinado com o segredo de serviço, e nenhum
// token de usuário (nem o de um admin) abre esta porta.
//
// Isso importa especialmente nesta rota: ela lança RECEITA no livro contábil
// sem que dinheiro nenhum tenha passado pelo nosso PSP. Se ela aceitasse token
// de usuário, um admin comprometido (ou o próprio front) poderia inflar o
// faturamento direto no livro.
//
// FAIL-CLOSED: sem segredo configurado o grupo inteiro não é registrado (ver
// cmd/server/main.go). Aqui, por garantia, segredo vazio recusa tudo — HS256
// aceita chave vazia normalmente, e "sem segredo" viraria "qualquer um assina".
func RequireService(serviceSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if serviceSecret == "" {
			slog.Error("rota interna sem SERVICE_JWT_SECRET — recusando (fail-closed)",
				"path", c.FullPath(), "request_id", c.GetString("request_id"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		hdr := c.GetHeader("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		subject, err := servicetoken.Parse(strings.TrimPrefix(hdr, "Bearer "), serviceSecret)
		if err != nil {
			slog.Warn("token de serviço recusado",
				"error", err.Error(), "path", c.FullPath(),
				"request_id", c.GetString("request_id"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		// `caller` é o serviço que chamou (sub do token), não a pessoa. Quem é a
		// PESSOA vem no corpo da requisição, vindo da trilha do order-service.
		c.Set("caller_service", subject)
		c.Set("user_role", servicetoken.Role)
		c.Next()
	}
}
