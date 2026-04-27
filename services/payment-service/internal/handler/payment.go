package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/model"
	"github.com/utilar/payment-service/internal/orderclient"
	"github.com/utilar/payment-service/internal/psp"
)

// maxPaymentRequestBody — limite de body em POST /payments. Payload típico
// é ~1KB; 16KB é folga generosa pra futuros campos sem abrir DoS por body grande.
const maxPaymentRequestBody = 16 * 1024

// extractBearerToken pega o JWT do header Authorization. Vazio se ausente/malformado.
func extractBearerToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// OrderLookup é a interface que o PaymentHandler precisa pra resolver pedidos.
// Definida aqui (ao invés de usar diretamente *orderclient.Client) pra permitir
// mock em testes e pra facilitar swap futuro (gRPC, message bus etc).
type OrderLookup interface {
	Get(ctx context.Context, orderID, jwt string) (*orderclient.Order, error)
}

type PaymentHandler struct {
	db          *sql.DB
	gateway     psp.Gateway
	orderClient OrderLookup
	devMode     bool // permite skip da validação cross-service em tests/dev
}

func NewPaymentHandler(db *sql.DB, gateway psp.Gateway, orderClient OrderLookup, devMode bool) *PaymentHandler {
	return &PaymentHandler{
		db:          db,
		gateway:     gateway,
		orderClient: orderClient,
		devMode:     devMode,
	}
}

// Create handles POST /api/v1/payments
//
// SEGURANÇA (audit C1, C2):
//   - Antes de criar o pagamento, busca o pedido no order-service propagando o
//     JWT do cliente. order-service filtra por user_id, então 404 = "não é seu".
//   - O `amount` é DERIVADO do `order.total` retornado pelo order-service —
//     ignoramos qualquer valor que o cliente envie no body. Logamos warning
//     se diverge (sinal de bug ou tentativa de tamper).
//   - Em DevMode, se `orderClient` for nil, fazemos best-effort com o amount
//     do body (com warning).
func (h *PaymentHandler) Create(c *gin.Context) {
	// H5: cap o body antes de ler. Payments têm payload pequeno (~1KB);
	// 16KB é folga generosa pra futuros campos. Excesso retorna 400 via
	// erro do bind quando o reader corta.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPaymentRequestBody)

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

	// Resolve order no order-service (audit C1+C2).
	// authoritativeAmount é o que vamos cobrar — sempre source-of-truth do server.
	authoritativeAmount := req.Amount
	if h.orderClient != nil {
		jwt := extractBearerToken(c)
		if jwt == "" {
			Unauthorized(c, "missing bearer token")
			return
		}

		order, err := h.orderClient.Get(c.Request.Context(), req.OrderID, jwt)
		if err != nil {
			switch {
			case errors.Is(err, orderclient.ErrNotFound):
				// Pedido não existe OU não pertence ao user — mesma 404 (anti-enum).
				NotFound(c, "order not found")
			case errors.Is(err, orderclient.ErrUnauthorized):
				Unauthorized(c, "invalid token")
			default:
				slog.Error("create payment: order lookup",
					"order_id", req.OrderID,
					"error", err,
					"request_id", c.GetString("request_id"))
				BadGateway(c, "order service unavailable")
			}
			return
		}

		// Sanity check: order.userId precisa bater com o JWT (defesa em profundidade).
		if order.UserID != userID {
			slog.Error("create payment: order user mismatch — possible auth bug",
				"order_id", req.OrderID,
				"order_user", order.UserID,
				"jwt_user", userID,
				"request_id", c.GetString("request_id"))
			NotFound(c, "order not found")
			return
		}

		// Status do pedido tem que permitir pagamento.
		if order.Status != "pending_payment" {
			BadRequest(c, fmt.Sprintf("order status %q does not accept payments", order.Status))
			return
		}

		// AMOUNT AUTHORITATIVE — vem do order-service, não do body.
		// Se diverge do que o cliente mandou, loga (mas usa o do server).
		if math.Abs(req.Amount-order.Total) > 0.01 {
			slog.Warn("create payment: amount mismatch — using server-side total",
				"client_amount", req.Amount,
				"server_amount", order.Total,
				"order_id", req.OrderID,
				"user_id", userID,
				"request_id", c.GetString("request_id"))
		}
		authoritativeAmount = order.Total
	} else if !h.devMode {
		// Sem orderClient e não está em dev → config bug
		slog.Error("create payment: orderClient is nil in non-dev mode",
			"request_id", c.GetString("request_id"))
		InternalError(c, "service misconfigured")
		return
	} else {
		slog.Warn("create payment: skipping order validation (dev mode, no orderClient)",
			"request_id", c.GetString("request_id"))
	}

	// Insert payment row (pending) — usa amount autoritativo
	var paymentID string
	err := h.db.QueryRow(`
		INSERT INTO payments (order_id, user_id, method, amount)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.OrderID, userID, req.Method, authoritativeAmount).Scan(&paymentID)
	if err != nil {
		slog.Error("create payment: db insert", "error", err, "request_id", c.GetString("request_id"))
		InternalError(c, "could not create payment")
		return
	}

	// Call PSP via Gateway abstraction com amount autoritativo
	pspReq := psp.CreateRequest{
		OrderID:        req.OrderID,
		UserID:         userID,
		Amount:         authoritativeAmount,
		Currency:       "BRL",
		Method:         psp.PaymentMethod(req.Method),
		PayerEmail:     userEmail,
		PayerName:      req.PayerName,
		PayerCPF:       req.PayerCPF,
		IdempotencyKey: c.GetString("request_id"),
	}
	result, pspErr := h.gateway.CreatePayment(c.Request.Context(), pspReq)

	if pspErr != nil {
		slog.Error("create payment: psp call",
			"provider", h.gateway.Name(),
			"method", req.Method,
			"error", pspErr,
			"request_id", c.GetString("request_id"))
		h.db.Exec(`UPDATE payments SET status='failed', updated_at=now() WHERE id=$1`, paymentID)

		// Mapeia erro normalizado do PSP para HTTP
		switch {
		case errors.Is(pspErr, psp.ErrInvalidRequest):
			BadRequest(c, pspErr.Error())
		case errors.Is(pspErr, psp.ErrUpstream):
			BadGateway(c, "payment gateway error")
		default:
			BadGateway(c, "payment gateway error")
		}
		return
	}

	// Persiste o retorno do PSP
	h.db.Exec(`
		UPDATE payments SET psp_payment_id=$1, psp_payload=$2, updated_at=now() WHERE id=$3
	`, result.PSPID, result.RawPayload, paymentID)

	c.JSON(http.StatusCreated, gin.H{
		"id":           paymentID,
		"method":       req.Method,
		"status":       string(result.Status),
		"provider":     h.gateway.Name(),
		"psp_id":       result.PSPID,
		"clientSecret": result.ClientSecret, // Stripe: frontend usa pra stripe.confirmPayment
		"psp_payload":  result.ClientData,    // MP: QR/barcode/init_point; Stripe: PaymentIntent
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
// Chama PSP.GetPayment e atualiza status local. Workaround do webhook em dev.
// Em produção, o webhook faz esse trabalho automaticamente.
// Endpoint scopado ao user_id.
//
// NOTA DE SEGURANÇA (audit C3): também audita amount do PSP vs DB (foundation
// do fix que vai no webhook handler na Sprint 8.5).
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

	// Fetch from PSP (qualquer provider)
	pspResult, err := h.gateway.GetPayment(c.Request.Context(), pspID.String)
	if err != nil {
		slog.Error("sync: psp get",
			"provider", h.gateway.Name(),
			"error", err,
			"psp_id", pspID.String,
			"request_id", c.GetString("request_id"))
		BadGateway(c, "psp gateway error")
		return
	}

	// Amount validation (foundation de C3)
	if localAmount > 0 && math.Abs(pspResult.Amount-localAmount) > 0.01 {
		slog.Warn("sync: amount mismatch",
			"local", localAmount,
			"psp", pspResult.Amount,
			"provider", h.gateway.Name(),
			"psp_id", pspID.String,
			"request_id", c.GetString("request_id"))
	}

	// Mapeia status normalizado do PSP pro status local
	newStatus := currentStatus
	var confirmedAt *time.Time
	switch pspResult.Status {
	case psp.StatusApproved:
		newStatus = "confirmed"
		now := time.Now()
		confirmedAt = &now
	case psp.StatusRejected:
		newStatus = "failed"
	case psp.StatusCancelled:
		newStatus = "cancelled"
	case psp.StatusExpired:
		newStatus = "expired"
	case psp.StatusPending, psp.StatusAuthorized:
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
		"id":           id,
		"status":       newStatus,
		"psp_status":   string(pspResult.Status),
		"psp_amount":   pspResult.Amount,
		"local_amount": localAmount,
		"provider":     h.gateway.Name(),
		"changed":      newStatus != currentStatus,
	})
}

// Compile-time guard — silence unused import if ctx not needed in future.
var _ = json.RawMessage(nil)
var _ = context.Background
