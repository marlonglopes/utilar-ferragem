package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

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
		BadRequest(c, err.Error())
		return
	}

	userID := c.GetString("user_id")
	userEmail := c.GetString("user_email")
	if userID == "" {
		Unauthorized(c, "unauthorized")
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
		slog.Error("create payment: db insert", "error", err, "request_id", c.GetString("request_id"))
		InternalError(c, "could not create payment")
		return
	}

	// Call Mercado Pago
	var mpRaw json.RawMessage
	var mpErr error

	switch req.Method {
	case model.MethodPix:
		mpRaw, mpErr = h.mp.CreatePixPayment(req.OrderID, req.Amount, userEmail)
	case model.MethodBoleto:
		if req.PayerCPF == "" || req.PayerName == "" {
			BadRequest(c, "boleto requires payer_cpf and payer_name")
			return
		}
		mpRaw, mpErr = h.mp.CreateBoleto(req.OrderID, req.Amount, userEmail, req.PayerCPF, req.PayerName)
	case model.MethodCard:
		mpRaw, mpErr = h.mp.CreatePreference(req.OrderID, req.Amount, "Pedido UtiLar Ferragem")
	default:
		BadRequest(c, "unsupported method")
		return
	}

	if mpErr != nil {
		slog.Error("create payment: mp call", "method", req.Method, "error", mpErr, "request_id", c.GetString("request_id"))
		h.db.Exec(`UPDATE payments SET status='failed', updated_at=now() WHERE id=$1`, paymentID)
		BadGateway(c, "payment gateway error")
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
		NotFound(c, "payment not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, p)
}

// Sync handles POST /api/v1/payments/:id/sync
// Chama MP.GetPayment e atualiza status local. Workaround do webhook em dev
// (sem ngrok/URL pública). Em produção, o webhook faz esse trabalho.
// Endpoint scopado ao user_id (não pode sincronizar payment alheio).
//
// NOTA DE SEGURANÇA (audit C3): Em dev, usamos este endpoint também como
// verificação cruzada — se MP diz approved mas local diz pending, atualiza.
// A validação de amount contra MP entra na Sprint 8.5 no webhook handler real.
func (h *PaymentHandler) Sync(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	var pspID sql.NullString
	var currentStatus string
	var localAmount float64
	err := h.db.QueryRow(`
		SELECT psp_payment_id, status, amount FROM payments WHERE id=$1 AND user_id=$2
	`, id, userID).Scan(&pspID, &currentStatus, &localAmount)
	if err == sql.ErrNoRows {
		NotFound(c, "payment not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if !pspID.Valid || pspID.String == "" {
		BadRequest(c, "payment has no psp_payment_id yet")
		return
	}

	// Fetch MP
	mpRaw, err := h.mp.GetPayment(pspID.String)
	if err != nil {
		slog.Error("sync: mp get", "error", err, "psp_id", pspID.String, "request_id", c.GetString("request_id"))
		BadGateway(c, "mp gateway error")
		return
	}

	var mpResp struct {
		Status            string  `json:"status"`
		TransactionAmount float64 `json:"transaction_amount"`
		ID                int64   `json:"id"`
	}
	if err := json.Unmarshal(mpRaw, &mpResp); err != nil {
		InternalError(c, "could not parse mp response")
		return
	}

	// Amount validation (foundation de C3 — já usamos aqui porque sync
	// é manual e audita MP contra DB; webhook real vai fazer o mesmo).
	if localAmount > 0 && (mpResp.TransactionAmount-localAmount) > 0.01 || (localAmount-mpResp.TransactionAmount) > 0.01 {
		slog.Warn("sync: amount mismatch",
			"local", localAmount, "mp", mpResp.TransactionAmount,
			"psp_id", pspID.String, "request_id", c.GetString("request_id"))
	}

	newStatus := currentStatus
	var confirmedAt *time.Time
	switch mpResp.Status {
	case "approved":
		newStatus = "confirmed"
		now := time.Now()
		confirmedAt = &now
	case "rejected", "cancelled":
		newStatus = "failed"
	case "pending", "in_process":
		newStatus = "pending"
	}

	if newStatus != currentStatus {
		_, err = h.db.Exec(`
			UPDATE payments SET status=$1, confirmed_at=$2, updated_at=now() WHERE id=$3
		`, newStatus, confirmedAt, id)
		if err != nil {
			DBError(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          id,
		"status":      newStatus,
		"mp_status":   mpResp.Status,
		"mp_amount":   mpResp.TransactionAmount,
		"local_amount": localAmount,
		"changed":     newStatus != currentStatus,
	})
}
