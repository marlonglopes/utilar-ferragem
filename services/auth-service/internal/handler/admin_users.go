package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/model"
)

// UserAdminHandler expõe a LEITURA de usuários para o admin — o que faltava para
// gerir staff: sem listar usuários, não dá para achar o userId de alguém e
// promovê-lo a operador (o CreateOperator exige o uuid). É a base das personas
// (contador/vendas/almoxarifado): primeiro achar a pessoa, depois dar o papel.
type UserAdminHandler struct{ db *sql.DB }

func NewUserAdminHandler(db *sql.DB) *UserAdminHandler { return &UserAdminHandler{db: db} }

type adminUserRow struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	Role          string  `json:"role"`
	CPF           *string `json:"cpf"`
	Phone         *string `json:"phone"`
	EmailVerified bool    `json:"emailVerified"`
	CreatedAt     string  `json:"createdAt"`
}

func isKnownRole(r string) bool { return model.IsKnownRole(r) }

type updateRoleReq struct {
	Role string `json:"role"`
}

// UpdateUserRole PATCH /api/v1/admin/users/:id/role  {role}
//
// É como o dono ATRIBUI uma persona (contador/vendas/almoxarife) a alguém —
// sem isto, os papéis novos existiriam no enum mas ninguém os receberia. Só
// admin (grupo de rota). Audita quem mudou o papel de quem (de→para), porque
// promover alguém a `vendas` passa a deixá-la ver custo, e a `admin` entrega a
// loja inteira: é exatamente o tipo de ação que precisa de trilha.
//
// Fail-closed: papel desconhecido → 400 (nunca chega no enum do Postgres, que
// vazaria schema). `service` não é papel de usuário (A1) e não passa em
// isKnownRole.
func (h *UserAdminHandler) UpdateUserRole(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		BadRequest(c, "id do usuário é obrigatório")
		return
	}
	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "corpo inválido")
		return
	}
	novo := strings.TrimSpace(req.Role)
	if !isKnownRole(novo) {
		BadRequest(c, "papel desconhecido")
		return
	}

	// Captura o papel antigo na MESMA transação do UPDATE: o old→new da trilha
	// tem que refletir a troca real, não uma leitura que corre com outro PATCH.
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		DBError(c, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var antigo string
	err = tx.QueryRowContext(c.Request.Context(),
		`SELECT role::text FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&antigo)
	if err == sql.ErrNoRows {
		NotFound(c, "usuário não encontrado")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if antigo == novo {
		c.JSON(http.StatusOK, gin.H{"id": id, "role": novo, "changed": false})
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(),
		`UPDATE users SET role = $2::user_role WHERE id = $1`, id, novo); err != nil {
		DBError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	logStoreEvent(c, h.db, storeEvent{
		Action:   "user.role.update",
		TargetID: &id,
		OldValue: map[string]any{"role": antigo},
		NewValue: map[string]any{"role": novo},
	})
	c.JSON(http.StatusOK, gin.H{"id": id, "role": novo, "changed": true})
}

// ListUsers GET /api/v1/admin/users?role=&q=&page=&per_page=
//
// Só admin (grupo de rota). Filtra por papel e busca por nome/e-mail/CPF.
// NUNCA devolve password_hash — a projeção é explícita, sem SELECT *.
func (h *UserAdminHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	where := "1=1"
	var args []any
	if r := strings.TrimSpace(c.Query("role")); r != "" && isKnownRole(r) {
		args = append(args, r)
		where += fmt.Sprintf(" AND role::text = $%d", len(args))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		args = append(args, "%"+q+"%")
		p := len(args)
		where += fmt.Sprintf(" AND (name ILIKE $%d OR email ILIKE $%d OR cpf ILIKE $%d)", p, p, p)
	}

	countArgs := append([]any{}, args...)
	offset := (page - 1) * perPage
	limitPH, offsetPH := len(args)+1, len(args)+2
	args = append(args, perPage, offset)

	rows, err := h.db.Query(fmt.Sprintf(`
		SELECT id, name, email, role::text, cpf, phone, email_verified, created_at
		FROM users
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, limitPH, offsetPH), args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]adminUserRow, 0, perPage)
	for rows.Next() {
		var u adminUserRow
		var created time.Time
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CPF, &u.Phone, &u.EmailVerified, &created); err != nil {
			DBError(c, err)
			return
		}
		u.CreatedAt = created.Format(time.RFC3339)
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var total int
	if err := h.db.QueryRow("SELECT count(*) FROM users WHERE "+where, countArgs...).Scan(&total); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page": page, "per_page": perPage, "total": total,
			"total_pages": (total + perPage - 1) / perPage,
		},
	})
}
