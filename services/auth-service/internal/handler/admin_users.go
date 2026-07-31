package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

func isKnownRole(r string) bool {
	switch r {
	case "customer", "seller", "admin", "store_operator":
		return true
	}
	return false
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
