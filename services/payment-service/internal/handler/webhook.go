package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	db            *sql.DB
	webhookSecret string
}

func NewWebhookHandler(db *sql.DB, secret string) *WebhookHandler {
	return &WebhookHandler{db: db, webhookSecret: secret}
}

// mpWebhookPayload covers the common shape of MP webhook notifications.
type mpWebhookPayload struct {
	ID       int64  `json:"id"`
	Action   string `json:"action"`
	Type     string `json:"type"`
	Data     struct {
		ID string `json:"id"`
	} `json:"data"`
}

// HandleMercadoPago handles POST /webhooks/mp
// Implements: HMAC verification → idempotency check → update payment → insert outbox
func (h *WebhookHandler) HandleMercadoPago(c *gin.Context) {
	// Read raw body first — needed for HMAC verification
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// HMAC-SHA256 verification (skip if no secret configured — dev mode)
	if h.webhookSecret != "" {
		sig := c.GetHeader("x-signature")
		if !verifyHMAC(rawBody, sig, h.webhookSecret) {
			slog.Warn("webhook: invalid signature")
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var payload mpWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// Only handle payment events
	if payload.Type != "payment" || payload.Data.ID == "" {
		c.Status(http.StatusOK)
		return
	}

	pspPaymentID := payload.Data.ID
	eventType := payload.Action // "payment.created", "payment.updated"

	if err := h.processPaymentEvent(pspPaymentID, eventType, rawBody); err != nil {
		slog.Error("webhook: process event", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

func (h *WebhookHandler) processPaymentEvent(pspPaymentID, eventType string, rawPayload []byte) error {
	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Idempotency check
	var webhookID string
	err = tx.QueryRow(`
		INSERT INTO webhook_events (psp_id, psp_payment_id, event_type, raw_payload)
		VALUES ('mercadopago', $1, $2, $3)
		ON CONFLICT (psp_id, psp_payment_id, event_type) DO NOTHING
		RETURNING id
	`, pspPaymentID, eventType, rawPayload).Scan(&webhookID)

	if err == sql.ErrNoRows {
		// Already processed — idempotent ACK
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("idempotency insert: %w", err)
	}

	// 2. Update payment status
	newStatus, confirmedAt := resolveStatus(eventType)
	var paymentID string
	err = tx.QueryRow(`
		UPDATE payments
		SET status = $1, psp_payment_id = $2, confirmed_at = $3, updated_at = now()
		WHERE psp_payment_id = $2 OR (psp_payment_id IS NULL AND order_id IN (
			SELECT order_id FROM payments WHERE psp_payment_id = $2 LIMIT 1
		))
		RETURNING id
	`, newStatus, pspPaymentID, confirmedAt).Scan(&paymentID)

	if err == sql.ErrNoRows {
		// Payment not yet created in our system (race) — log and ACK
		slog.Warn("webhook: payment not found", "psp_payment_id", pspPaymentID)
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("update payment: %w", err)
	}

	// 3. Insert into outbox (transactional — same tx)
	outboxPayload, _ := json.Marshal(map[string]string{
		"payment_id":     paymentID,
		"psp_payment_id": pspPaymentID,
		"event_type":     eventType,
		"status":         string(newStatus),
	})

	outboxEvent := "payment.confirmed"
	if newStatus == "failed" {
		outboxEvent = "payment.failed"
	}

	_, err = tx.Exec(`
		INSERT INTO payments_outbox (event_type, payload_json, next_attempt_at)
		VALUES ($1, $2, now())
	`, outboxEvent, outboxPayload)
	if err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}

	return tx.Commit()
}

func verifyHMAC(payload []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func resolveStatus(action string) (string, *time.Time) {
	switch action {
	case "payment.updated":
		now := time.Now()
		return "confirmed", &now
	case "payment.created":
		return "pending", nil
	default:
		return "failed", nil
	}
}
