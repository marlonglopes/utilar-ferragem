package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/model"
)

type AddressHandler struct{ db *sql.DB }

func NewAddressHandler(db *sql.DB) *AddressHandler { return &AddressHandler{db: db} }

// List GET /api/v1/addresses
func (h *AddressHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	rows, err := h.db.Query(`
		SELECT id, user_id, label, street, number, complement, neighborhood, city, state, cep, is_default, created_at
		FROM addresses WHERE user_id = $1 ORDER BY is_default DESC, created_at ASC
	`, userID)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()
	out := make([]model.Address, 0)
	for rows.Next() {
		var a model.Address
		if err := rows.Scan(&a.ID, &a.UserID, &a.Label, &a.Street, &a.Number, &a.Complement,
			&a.Neighborhood, &a.City, &a.State, &a.CEP, &a.IsDefault, &a.CreatedAt); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, a)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Create POST /api/v1/addresses
func (h *AddressHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")
	var req model.AddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	label := req.Label
	if label == "" {
		label = "Principal"
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	// se marcado como default, desfaz outros defaults primeiro
	if req.IsDefault {
		if _, err := tx.Exec(`UPDATE addresses SET is_default=false WHERE user_id=$1 AND is_default=true`, userID); err != nil {
			DBError(c, err)
			return
		}
	}

	var id string
	err = tx.QueryRow(`
		INSERT INTO addresses (user_id, label, street, number, complement, neighborhood, city, state, cep, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
	`, userID, label, req.Street, req.Number, req.Complement, req.Neighborhood, req.City, req.State, req.CEP, req.IsDefault).Scan(&id)
	if err != nil {
		DBError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	addr, err := h.loadAddress(userID, id)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, addr)
}

// Delete DELETE /api/v1/addresses/:id
func (h *AddressHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")

	res, err := h.db.Exec(`DELETE FROM addresses WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		DBError(c, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		NotFound(c, "address not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// -- helper -----------------------------------------------------------------

func (h *AddressHandler) loadAddress(userID, id string) (*model.Address, error) {
	var a model.Address
	err := h.db.QueryRow(`
		SELECT id, user_id, label, street, number, complement, neighborhood, city, state, cep, is_default, created_at
		FROM addresses WHERE id=$1 AND user_id=$2
	`, id, userID).Scan(&a.ID, &a.UserID, &a.Label, &a.Street, &a.Number, &a.Complement,
		&a.Neighborhood, &a.City, &a.State, &a.CEP, &a.IsDefault, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
