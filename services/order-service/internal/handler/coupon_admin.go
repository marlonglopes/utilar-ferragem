package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/coupon"
)

// loadCoupon lê um cupom pelo código (já normalizado) para VALIDAR na criação do
// pedido. O incremento de uso NÃO é aqui — é o UPDATE condicional dentro da
// transação do pedido (order.go), que fecha a corrida.
func (h *OrderHandler) loadCoupon(ctx context.Context, code string) (coupon.Coupon, error) {
	var c coupon.Coupon
	var maxUses sql.NullInt64
	var expiresAt sql.NullTime
	err := h.db.QueryRowContext(ctx, `
		SELECT code, type, value, min_subtotal, max_uses, uses, active, expires_at
		FROM coupons WHERE code = $1`, code).
		Scan(&c.Code, &c.Type, &c.Value, &c.MinSubtotal, &maxUses, &c.Uses, &c.Active, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return coupon.Coupon{}, coupon.ErrNotFound
	}
	if err != nil {
		return coupon.Coupon{}, err
	}
	if maxUses.Valid {
		m := int(maxUses.Int64)
		c.MaxUses = &m
	}
	if expiresAt.Valid {
		c.ExpiresAt = &expiresAt.Time
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// CRUD admin de cupons — admin-only (dinheiro/marketing). Espelha shipping_admin.
// ---------------------------------------------------------------------------

// CouponHandler serve /api/v1/admin/coupons.
type CouponHandler struct {
	db *sql.DB
}

func NewCouponHandler(db *sql.DB) *CouponHandler {
	return &CouponHandler{db: db}
}

type couponRow struct {
	ID          string     `json:"id"`
	Code        string     `json:"code"`
	Type        string     `json:"type"`
	Value       float64    `json:"value"`
	MinSubtotal float64    `json:"minSubtotal"`
	MaxUses     *int       `json:"maxUses"`
	Uses        int        `json:"uses"`
	Active      bool       `json:"active"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type couponInput struct {
	Code        string     `json:"code"`
	Type        string     `json:"type"`
	Value       float64    `json:"value"`
	MinSubtotal float64    `json:"minSubtotal"`
	MaxUses     *int       `json:"maxUses"`
	Active      *bool      `json:"active"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

// validateForCreate confere as MESMAS regras do banco antes do INSERT, com
// mensagem acionável (o CHECK do banco daria um erro cru).
func (in couponInput) validateForCreate() (string, error) {
	code := coupon.NormalizeCode(in.Code)
	if code == "" {
		return "", errors.New("código é obrigatório")
	}
	if len(code) > 40 {
		return "", errors.New("código longo demais (máx 40)")
	}
	if in.Type != coupon.TypePercent && in.Type != coupon.TypeFixed {
		return "", errors.New("tipo deve ser 'percent' ou 'fixed'")
	}
	if in.Value < 0 {
		return "", errors.New("valor não pode ser negativo")
	}
	if in.Type == coupon.TypePercent && in.Value > 100 {
		return "", errors.New("percentual não pode passar de 100")
	}
	if in.MinSubtotal < 0 {
		return "", errors.New("pedido mínimo não pode ser negativo")
	}
	if in.MaxUses != nil && *in.MaxUses <= 0 {
		return "", errors.New("limite de usos deve ser maior que zero (ou vazio p/ ilimitado)")
	}
	return code, nil
}

func (h *CouponHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, code, type, value, min_subtotal, max_uses, uses, active, expires_at, created_at
		FROM coupons ORDER BY created_at DESC`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]couponRow, 0)
	for rows.Next() {
		var r couponRow
		var maxUses sql.NullInt64
		var exp sql.NullTime
		if err := rows.Scan(&r.ID, &r.Code, &r.Type, &r.Value, &r.MinSubtotal,
			&maxUses, &r.Uses, &r.Active, &exp, &r.CreatedAt); err != nil {
			DBError(c, err)
			return
		}
		if maxUses.Valid {
			m := int(maxUses.Int64)
			r.MaxUses = &m
		}
		if exp.Valid {
			r.ExpiresAt = &exp.Time
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"coupons": out})
}

func (h *CouponHandler) Create(c *gin.Context) {
	var in couponInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	code, err := in.validateForCreate()
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}

	var id string
	err = h.db.QueryRow(`
		INSERT INTO coupons (code, type, value, min_subtotal, max_uses, active, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		code, in.Type, in.Value, in.MinSubtotal, in.MaxUses, active, in.ExpiresAt).Scan(&id)
	if err != nil {
		// Código duplicado (UNIQUE) é erro de usuário, não de servidor.
		if isUniqueViolation(err) {
			Conflict(c, "já existe um cupom com esse código")
			return
		}
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "code": code})
}

// Update altera campos mutáveis (o mais comum é desativar). Só os campos
// enviados mudam; `uses` e `code` nunca são editados aqui.
func (h *CouponHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var in couponInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	res, err := h.db.Exec(`
		UPDATE coupons SET
			active       = COALESCE($2, active),
			max_uses     = $3,
			expires_at   = $4,
			min_subtotal = CASE WHEN $5 >= 0 THEN $5 ELSE min_subtotal END
		WHERE id = $1`,
		id, in.Active, in.MaxUses, in.ExpiresAt, in.MinSubtotal)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "cupom não encontrado")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *CouponHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM coupons WHERE id = $1`, id)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "cupom não encontrado")
		return
	}
	c.Status(http.StatusNoContent)
}
