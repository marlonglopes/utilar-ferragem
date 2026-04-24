package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
)

type SellerHandler struct{ db *sql.DB }

func NewSellerHandler(db *sql.DB) *SellerHandler { return &SellerHandler{db: db} }

// List GET /api/v1/sellers
func (h *SellerHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, name, rating, review_count, verified
		FROM sellers ORDER BY name ASC
	`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Seller, 0)
	for rows.Next() {
		var s model.Seller
		if err := rows.Scan(&s.ID, &s.Name, &s.Rating, &s.ReviewCount, &s.Verified); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, s)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
