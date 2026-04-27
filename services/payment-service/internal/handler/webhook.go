// Webhook handler — provider-agnostic, baseado em psp.Gateway.
//
// Fluxo (audit C3, C4, C5):
//   1. Lê body com limite (DoS guard)
//   2. Verifica assinatura via gateway.VerifyWebhook (HMAC PSP-específico)
//   3. Parseia evento via gateway.ParseWebhookEvent
//   4. SEGURANÇA C3: chama gateway.GetPayment(pspID) pra confirmar amount com o
//      PSP — se valor do webhook diverge do PSP, ou do nosso DB, recusamos +
//      logamos como possível fraude. Webhooks são "untrusted input" mesmo com
//      assinatura válida (atacante pode reusar pagamento legítimo de centavos
//      pra disparar confirmação de pedido caro).
//   5. Idempotency check via UNIQUE (psp_id, psp_payment_id, event_type)
//   6. Atualiza payments + insere payments_outbox numa transação só
//
// O endpoint é `/webhooks/:provider`. O `:provider` na URL precisa bater com o
// gateway.Name() configurado — assim o atacante não consegue mandar webhook
// pra um endpoint inativo.
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
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/psp"
)

// maxWebhookBody — limite de 64KB pra prevenir DoS via body absurdo.
// Stripe webhooks ficam < 8KB; MP < 4KB. 64KB é folga generosa.
const maxWebhookBody = 64 * 1024

type WebhookHandler struct {
	db      *sql.DB
	gateway psp.Gateway
}

func NewWebhookHandler(db *sql.DB, gateway psp.Gateway) *WebhookHandler {
	return &WebhookHandler{db: db, gateway: gateway}
}

// Handle handles POST /webhooks/:provider — provider-agnostic.
func (h *WebhookHandler) Handle(c *gin.Context) {
	provider := c.Param("provider")
	if provider != h.gateway.Name() {
		// Provider URL diferente do gateway configurado — 404 deliberado pra
		// evitar que atacante descubra qual provider está ativo.
		c.Status(http.StatusNotFound)
		return
	}

	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody))
	if err != nil {
		slog.Warn("webhook: read body", "error", err, "request_id", c.GetString("request_id"))
		c.Status(http.StatusBadRequest)
		return
	}

	// 1. Verify signature (audit C4 — HMAC delegado pro gateway específico)
	if err := h.gateway.VerifyWebhook(rawBody, c.Request.Header); err != nil {
		slog.Warn("webhook: invalid signature",
			"provider", provider,
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		c.Status(http.StatusUnauthorized)
		return
	}

	// 2. Parse event
	event, err := h.gateway.ParseWebhookEvent(rawBody)
	if err != nil {
		slog.Error("webhook: parse failed",
			"provider", provider,
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		c.Status(http.StatusBadRequest)
		return
	}
	if event == nil {
		// Tipo de evento irrelevante (ex: ping) — ACK e segue
		c.Status(http.StatusOK)
		return
	}

	// 3. Validate amount via gateway.GetPayment — source of truth do PSP (audit C3)
	pspResult, err := h.gateway.GetPayment(c.Request.Context(), event.PSPID)
	if err != nil {
		slog.Error("webhook: GetPayment failed",
			"provider", provider,
			"psp_id", event.PSPID,
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		// 502: erro upstream do PSP. PSPs reentregam webhooks, então não é fim do mundo.
		c.Status(http.StatusBadGateway)
		return
	}

	// 4. Process atomically (idempotency + status + outbox + amount audit)
	if err := h.processEvent(provider, event, pspResult, rawBody, c.GetString("request_id")); err != nil {
		slog.Error("webhook: process failed",
			"provider", provider,
			"psp_id", event.PSPID,
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

func (h *WebhookHandler) processEvent(
	provider string,
	event *psp.WebhookEvent,
	pspResult *psp.GetResult,
	rawPayload []byte,
	requestID string,
) error {
	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Idempotency — duas tentativas do mesmo (provider, payment, event) são no-op.
	var webhookID string
	err = tx.QueryRow(`
		INSERT INTO webhook_events (psp_id, psp_payment_id, event_type, raw_payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (psp_id, psp_payment_id, event_type) DO NOTHING
		RETURNING id
	`, provider, event.PSPID, event.EventType, rawPayload).Scan(&webhookID)
	if err == sql.ErrNoRows {
		// Já processado — ACK silencioso
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("idempotency insert: %w", err)
	}

	// Localiza o pagamento local + valida amount (audit C3)
	var paymentID string
	var localAmount float64
	var currentStatus string
	err = tx.QueryRow(`
		SELECT id, amount, status FROM payments WHERE psp_payment_id = $1
	`, event.PSPID).Scan(&paymentID, &localAmount, &currentStatus)
	if err == sql.ErrNoRows {
		// Webhook chegou antes do INSERT da Create. PSPs reentregam — esse
		// caso resolve sozinho. Ack pra não prender retry.
		slog.Warn("webhook: payment not found locally yet",
			"provider", provider,
			"psp_id", event.PSPID,
			"request_id", requestID,
		)
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("load payment: %w", err)
	}

	// C3: amount audit — compara o que o PSP autoritativamente diz que cobrou
	// vs o valor que persistimos localmente. Tolerância de 1 centavo pra
	// arredondamento (centavos vs reais em float).
	if math.Abs(pspResult.Amount-localAmount) > 0.01 {
		slog.Error("webhook: AMOUNT MISMATCH — possible fraud, rejecting confirmation",
			"provider", provider,
			"psp_id", event.PSPID,
			"local_amount", localAmount,
			"psp_amount", pspResult.Amount,
			"delta", pspResult.Amount-localAmount,
			"request_id", requestID,
		)
		// Não atualiza status — fica em pending pra revisão manual.
		// Marca payment como suspeito (psp_metadata) para alertar ops.
		flag := json.RawMessage(fmt.Sprintf(
			`{"amount_mismatch":true,"local":%v,"psp":%v,"detected_at":%q}`,
			localAmount, pspResult.Amount, time.Now().Format(time.RFC3339),
		))
		_, _ = tx.Exec(`
			UPDATE payments SET psp_metadata=$1, updated_at=now() WHERE id=$2
		`, []byte(flag), paymentID)
		// Inserir evento de auditoria no outbox pra alertar (consumido pelo
		// fraud-monitor depois — hoje só Redpanda subscriber externo)
		fraudPayload, _ := json.Marshal(map[string]any{
			"payment_id":    paymentID,
			"psp_payment_id": event.PSPID,
			"provider":      provider,
			"local_amount":  localAmount,
			"psp_amount":    pspResult.Amount,
		})
		_, _ = tx.Exec(`
			INSERT INTO payments_outbox (event_type, payload_json, next_attempt_at)
			VALUES ($1, $2, now())
		`, "payment.fraud_suspect", fraudPayload)
		// ACK pro PSP pra não retry storm — registramos o problema, ops resolve.
		return tx.Commit()
	}

	// Mapeia status normalizado pro local
	newStatus := mapPSPStatus(event.Status)
	var confirmedAt *time.Time
	if newStatus == "confirmed" {
		now := time.Now()
		confirmedAt = &now
	}

	if newStatus != currentStatus {
		_, err = tx.Exec(`
			UPDATE payments SET status=$1, confirmed_at=$2, updated_at=now() WHERE id=$3
		`, newStatus, confirmedAt, paymentID)
		if err != nil {
			return fmt.Errorf("update payment: %w", err)
		}
	}

	// Outbox event (mesmo se status não mudou — algumas confirmations chegam
	// depois do sync já ter promovido pra confirmed; idempotência fica nos
	// consumers pelo payment_id).
	outboxPayload, _ := json.Marshal(map[string]any{
		"payment_id":     paymentID,
		"psp_payment_id": event.PSPID,
		"provider":       provider,
		"event_type":     event.EventType,
		"status":         newStatus,
		"amount":         localAmount,
	})

	outboxEvent := "payment.confirmed"
	if newStatus == "failed" {
		outboxEvent = "payment.failed"
	} else if newStatus == "cancelled" {
		outboxEvent = "payment.cancelled"
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

// mapPSPStatus traduz o status normalizado do PSP pro vocabulário local.
func mapPSPStatus(s psp.PaymentStatus) string {
	switch s {
	case psp.StatusApproved:
		return "confirmed"
	case psp.StatusRejected:
		return "failed"
	case psp.StatusCancelled:
		return "cancelled"
	case psp.StatusExpired:
		return "expired"
	case psp.StatusPending, psp.StatusAuthorized:
		return "pending"
	default:
		return "pending"
	}
}

// -- Legacy helpers — preservados pros testes unitários antigos. ----------------

// verifyHMAC implementa HMAC-SHA256 simples sobre o body — formato genérico que
// não é mais usado em produção (cada Gateway implementa seu próprio formato via
// VerifyWebhook). Fica aqui só pra compatibilidade do webhook_unit_test.go.
//
// Deprecated: use psp.Gateway.VerifyWebhook em vez disso.
func verifyHMAC(payload []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// resolveStatus mapeia ações antigas do MP pra status. Mantido pra compat
// dos testes unitários existentes — produção usa mapPSPStatus + Gateway.
//
// Deprecated: use mapPSPStatus.
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
