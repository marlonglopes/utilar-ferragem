package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RoleAdmin é o papel exigido pelas rotas contábeis.
const RoleAdmin = "admin"

// AdminOnly exige role=admin no JWT já validado por JWTMiddleware.
//
// POR QUE UM MIDDLEWARE SEPARADO: as rotas do livro contábil expõem o
// faturamento inteiro da empresa, taxa efetiva do gateway e divergências de
// dinheiro — é o conjunto de dados mais sensível do sistema depois das
// credenciais. Ownership scoping (user_id) NÃO serve aqui, porque o relatório é
// agregado e não pertence a um usuário; a única proteção possível é papel.
//
// Fail-closed: role ausente ou vazia é NEGADA. Um JWT antigo emitido antes de
// existir o claim `role` não pode virar passe livre pro faturamento.
//
// Responde 403 (e não 404 como o /metrics): aqui o usuário JÁ está autenticado,
// então não há o que esconder sobre a existência da rota, e um 403 explícito
// evita horas de debug de "por que a API do dashboard some".
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("user_role")
		if role != RoleAdmin {
			slog.Warn("acesso negado a rota contábil",
				"request_id", c.GetString("request_id"),
				"user_id", c.GetString("user_id"),
				"role", role,
				"path", c.FullPath(),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorEnvelope{
				Error:     "admin role required",
				Code:      "forbidden",
				RequestID: c.GetString("request_id"),
			})
			return
		}
		c.Next()
	}
}
