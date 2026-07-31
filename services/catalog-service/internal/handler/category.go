package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
)

// categoryInput — payload de criar/atualizar categoria.
type categoryInput struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Icon      string  `json:"icon"`
	ParentID  *string `json:"parentId"`
	SortOrder *int    `json:"sortOrder"`
}

// O id da categoria é um SLUG e também a FK dos produtos — por isso é imutável
// e restrito: minúsculas, números e hífen. Um id com espaço/maiúscula quebraria
// URLs e o casamento da ingestão.
var categorySlugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *CategoryHandler) categoryExists(id string) bool {
	var x bool
	return h.db.QueryRow(`SELECT true FROM categories WHERE id=$1`, id).Scan(&x) == nil
}

// Create POST /api/v1/admin/categories — cria uma categoria (admin).
func (h *CategoryHandler) Create(c *gin.Context) {
	var in categoryInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	in.ID = strings.TrimSpace(strings.ToLower(in.ID))
	in.Name = strings.TrimSpace(in.Name)
	if !categorySlugRe.MatchString(in.ID) {
		BadRequest(c, "id deve ser um slug: minúsculas, números e hífen (ex.: 'eletrica')")
		return
	}
	if in.Name == "" {
		BadRequest(c, "nome é obrigatório")
		return
	}
	if h.categoryExists(in.ID) {
		Conflict(c, "já existe uma categoria com esse id")
		return
	}
	if in.ParentID != nil && *in.ParentID != "" && !h.categoryExists(*in.ParentID) {
		BadRequest(c, "categoria pai inexistente")
		return
	}
	icon := strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "▣"
	}
	sort := 0
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	if _, err := h.db.Exec(
		`INSERT INTO categories (id, name, icon, parent_id, sort_order) VALUES ($1,$2,$3,$4,$5)`,
		in.ID, in.Name, icon, in.ParentID, sort); err != nil {
		DBError(c, err)
		return
	}
	audit(h.db, c, "category.create", "category", in.ID, AuditChanges{
		"name": {Old: nil, New: in.Name},
		"icon": {Old: nil, New: icon},
	})
	c.JSON(http.StatusCreated, gin.H{"id": in.ID})
}

// Update PATCH /api/v1/admin/categories/:id — renomeia / muda ícone/ordem.
// O `id` NÃO muda (é a FK dos produtos).
func (h *CategoryHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var in categoryInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	var oldName, oldIcon string
	var oldSort int
	err := h.db.QueryRow(`SELECT name, icon, sort_order FROM categories WHERE id=$1`, id).
		Scan(&oldName, &oldIcon, &oldSort)
	if err == sql.ErrNoRows {
		NotFound(c, "categoria não encontrada")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	set := []string{}
	args := []any{}
	idx := 1
	changes := AuditChanges{}
	if n := strings.TrimSpace(in.Name); n != "" && n != oldName {
		changes.changed("name", oldName, n)
		set = append(set, fmt.Sprintf("name = $%d", idx))
		args = append(args, n)
		idx++
	}
	if ic := strings.TrimSpace(in.Icon); ic != "" && ic != oldIcon {
		changes.changed("icon", oldIcon, ic)
		set = append(set, fmt.Sprintf("icon = $%d", idx))
		args = append(args, ic)
		idx++
	}
	if in.SortOrder != nil && *in.SortOrder != oldSort {
		changes.changed("sort_order", oldSort, *in.SortOrder)
		set = append(set, fmt.Sprintf("sort_order = $%d", idx))
		args = append(args, *in.SortOrder)
		idx++
	}
	if len(set) == 0 {
		BadRequest(c, "nada para atualizar")
		return
	}
	args = append(args, id)
	if _, err := h.db.Exec(
		fmt.Sprintf(`UPDATE categories SET %s WHERE id = $%d`, strings.Join(set, ", "), idx), args...); err != nil {
		DBError(c, err)
		return
	}
	audit(h.db, c, "category.update", "category", id, changes)
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// Delete DELETE /api/v1/admin/categories/:id — só se NÃO houver produto na
// categoria (a FK não deixaria, e mesmo se deixasse, apagar produto por tabela
// pai é o tipo de estrago silencioso que a loja não pode ter).
func (h *CategoryHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	var n int
	if err := h.db.QueryRow(`SELECT count(*) FROM products WHERE category_id=$1`, id).Scan(&n); err != nil {
		DBError(c, err)
		return
	}
	if n > 0 {
		Conflict(c, fmt.Sprintf("categoria tem %d produto(s) — mova-os antes de excluir", n))
		return
	}
	res, err := h.db.Exec(`DELETE FROM categories WHERE id=$1`, id)
	if err != nil {
		DBError(c, err)
		return
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		NotFound(c, "categoria não encontrada")
		return
	}
	audit(h.db, c, "category.delete", "category", id, AuditChanges{})
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

type CategoryHandler struct{ db *sql.DB }

func NewCategoryHandler(db *sql.DB) *CategoryHandler { return &CategoryHandler{db: db} }

// List GET /api/v1/categories
func (h *CategoryHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, name, icon, parent_id, sort_order
		FROM categories
		ORDER BY sort_order ASC, name ASC
	`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Category, 0)
	for rows.Next() {
		var cat model.Category
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Icon, &cat.ParentID, &cat.SortOrder); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, cat)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
