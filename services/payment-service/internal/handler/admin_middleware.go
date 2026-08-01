package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/roles"
)

// RoleAdmin é o papel exigido pelas rotas contábeis.
const RoleAdmin = roles.Admin

// LedgerRoles — quem entra no livro contábil. Admin e CONTADOR: a contabilidade
// (faturamento, taxa efetiva, conciliação, fechamento de período) é o trabalho
// do contador, então ele age aqui — é o único lugar onde a persona `contador`
// muta algo (fora daqui é só leitura). O livro é financeiro puro: NÃO expõe
// custo de aquisição por produto (isso mora no catalog), então não há
// vazamento de custo em conceder a rota ao contador.
var LedgerRoles = []string{roles.Admin, roles.Contador}

// RequireAnyRole exige que o JWT (já validado por JWTMiddleware) tenha um dos
// papéis dados. Fail-closed: role ausente/vazia ou fora da lista é NEGADA (403).
//
// POR QUE role e não ownership: as rotas contábeis expõem o faturamento inteiro,
// taxa efetiva do gateway e divergências de dinheiro — o dado mais sensível
// depois das credenciais. O relatório é agregado, não pertence a um usuário,
// então scoping por user_id não protege; só papel protege.
//
// Responde 403 (não 404 como o /metrics): o usuário JÁ está autenticado, não há
// o que esconder sobre a existência da rota.
func RequireAnyRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role := c.GetString("user_role")
		if _, ok := set[role]; !ok || role == "" {
			slog.Warn("acesso negado a rota contábil",
				"request_id", c.GetString("request_id"),
				"user_id", c.GetString("user_id"),
				"role", role,
				"path", c.FullPath(),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorEnvelope{
				Error:     "admin or contador role required",
				Code:      "forbidden",
				RequestID: c.GetString("request_id"),
			})
			return
		}
		c.Next()
	}
}

// AdminOnly mantém o contrato antigo (só admin) para quem precisar de uma rota
// exclusiva do dono. As rotas do livro usam LedgerRoles (admin + contador).
func AdminOnly() gin.HandlerFunc { return RequireAnyRole(RoleAdmin) }
