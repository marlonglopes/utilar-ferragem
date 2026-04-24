package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
)

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
