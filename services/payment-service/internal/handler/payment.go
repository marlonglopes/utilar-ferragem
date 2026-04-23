package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/mercadopago"
	"github.com/utilar/payment-service/internal/model"
)

type PaymentHandler struct {
	db  *sql.DB
	mp  *mercadopago.Client
}

func NewPaymentHandler(db *sql.DB, mp *mercadopago.Client) *PaymentHandler {
	return &PaymentHandler{db: db, mp: mp}
}

// Create handles POST /api/v1/payments
func (h *PaymentHandler) Create(c *gin.Context) {
	var req model.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")
	userEmail := c.GetString("user_email")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Insert payment row (pending)
	var paymentID string
	err := h.db.QueryRow(`
		INSERT INTO payments (order_id, user_id, method, amount)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.OrderID, userID, req.Method, req.Amount).Scan(&paymentID)
	if err != nil {
		slog.Error("create payment: db insert", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create payment"})
		return
	}

	// Call Mercado Pago
	var mpRaw json.RawMessage
	var mpErr error

	switch req.Method {
	case model.MethodPix:
		mpRaw, mpErr = h.mp.CreatePixPayment(req.OrderID, req.Amount, userEmail)
	case model.MethodBoleto:
		mpRaw, mpErr = h.mp.CreateBoleto(req.OrderID, req.Amount, userEmail, "", "")
	case model.MethodCard:
		mpRaw, mpErr = h.mp.CreatePreference(req.OrderID, req.Amount, "Pedido UtiLar Ferragem")
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported method"})
		return
	}

	if mpErr != nil {
		slog.Error("create payment: mp call", "method", req.Method, "error", mpErr)
		// Mark payment as failed
		h.db.Exec(`UPDATE payments SET status='failed', updated_at=now() WHERE id=$1`, paymentID)
		c.JSON(http.StatusBadGateway, gin.H{"error": "payment gateway error"})
		return
	}

	// Extract MP payment ID from response
	var mpResp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(mpRaw, &mpResp)

	// Update payment row with PSP data
	h.db.Exec(`
		UPDATE payments SET psp_payment_id=$1, psp_payload=$2, updated_at=now() WHERE id=$3
	`, mpResp.ID, mpRaw, paymentID)

	c.JSON(http.StatusCreated, gin.H{
		"id":         paymentID,
		"method":     req.Method,
		"status":     "pending",
		"psp_payload": mpRaw,
	})
}

// Get handles GET /api/v1/payments/:id
func (h *PaymentHandler) Get(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	var p model.Payment
	err := h.db.QueryRow(`
		SELECT id, order_id, user_id, method, status, amount, currency,
		       psp_payment_id, psp_metadata, psp_payload, confirmed_at, expires_at, created_at, updated_at
		FROM payments WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&p.ID, &p.OrderID, &p.UserID, &p.Method, &p.Status, &p.Amount, &p.Currency,
		&p.PSPPaymentID, &p.PSPMetadata, &p.PSPPayload,
		&p.ConfirmedAt, &p.ExpiresAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	c.JSON(http.StatusOK, p)
}
