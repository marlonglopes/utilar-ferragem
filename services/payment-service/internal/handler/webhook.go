// Webhook handler — provider-agnostic, baseado em psp.Gateway.
//
// Fluxo (audit C3, C4, C5):
//  1. Lê body com limite (DoS guard)
//  2. Verifica assinatura via gateway.VerifyWebhook (HMAC PSP-específico)
//  3. Parseia evento via gateway.ParseWebhookEvent
//  4. SEGURANÇA C3: chama gateway.GetPayment(pspID) pra confirmar amount com o
//     PSP — se valor do webhook diverge do PSP, ou do nosso DB, recusamos +
//     logamos como possível fraude. Webhooks são "untrusted input" mesmo com
//     assinatura válida (atacante pode reusar pagamento legítimo de centavos
//     pra disparar confirmação de pedido caro).
//  5. Idempotency check via UNIQUE (psp_id, psp_payment_id, event_type)
//  6. Atualiza payments + insere payments_outbox numa transação só
//
// O endpoint é `/webhooks/:provider`. O `:provider` na URL precisa bater com o
// gateway.Name() configurado — assim o atacante não consegue mandar webhook
// pra um endpoint inativo.
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/ledger"
	"github.com/utilar/payment-service/internal/psp"
	"github.com/utilar/pkg/requestid"
)

// maxWebhookBody — limite de 64KB pra prevenir DoS via body absurdo.
// Stripe webhooks ficam < 8KB; MP < 4KB. 64KB é folga generosa.
const maxWebhookBody = 64 * 1024

type WebhookHandler struct {
	db      *sql.DB
	gateway psp.Gateway

	// ledger e metrics são OPCIONAIS (nil = desligado). Opcionais de propósito:
	// o webhook é o caminho crítico do dinheiro entrando, e ele não pode
	// deixar de funcionar porque uma dependência acessória não foi montada.
	ledger  *ledger.Poster
	metrics WebhookMetrics
}

// WebhookMetrics é a fatia de métricas que o webhook usa. Interface local pra
// que internal/handler não dependa de Prometheus.
type WebhookMetrics interface {
	WebhookReceived(provider, outcome string)
	PaymentConfirmed(provider, method string)
	PaymentFailed(provider, method, reason string)
	LedgerPosted(kind string)
	LedgerRejected(kind, reason string)
}

func NewWebhookHandler(db *sql.DB, gateway psp.Gateway) *WebhookHandler {
	return &WebhookHandler{db: db, gateway: gateway}
}

// WithLedger liga o lançamento contábil automático na confirmação.
func (h *WebhookHandler) WithLedger(p *ledger.Poster) *WebhookHandler {
	h.ledger = p
	return h
}

// WithMetrics liga a instrumentação de negócio.
func (h *WebhookHandler) WithMetrics(m WebhookMetrics) *WebhookHandler {
	h.metrics = m
	return h
}

func (h *WebhookHandler) mark(outcome string) {
	if h.metrics != nil {
		h.metrics.WebhookReceived(h.gateway.Name(), outcome)
	}
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
		h.mark("read_error")
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
		h.mark("rejected_signature")
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
		h.mark("parse_error")
		c.Status(http.StatusBadRequest)
		return
	}
	if event == nil {
		// Tipo de evento irrelevante (ex: ping) — ACK e segue
		h.mark("ignored")
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
		h.mark("psp_error")
		c.Status(http.StatusBadGateway)
		return
	}

	// 4. Process atomically (idempotency + status + outbox + amount audit)
	if err := h.processEvent(c.Request.Context(), provider, event, pspResult, rawBody, c.GetString("request_id")); err != nil {
		slog.Error("webhook: process failed",
			"provider", provider,
			"psp_id", event.PSPID,
			"error", err,
			"request_id", c.GetString("request_id"),
		)
		h.mark("process_error")
		c.Status(http.StatusInternalServerError)
		return
	}
	h.mark("accepted")
	c.Status(http.StatusOK)
}

func (h *WebhookHandler) processEvent(
	ctx context.Context,
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
	// M2: armazenamos o webhook payload com PII redacted.
	redactedRaw := redactPSPPayload(rawPayload)
	var webhookID string
	err = tx.QueryRow(`
		INSERT INTO webhook_events (psp_id, psp_payment_id, event_type, raw_payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (psp_id, psp_payment_id, event_type) DO NOTHING
		RETURNING id
	`, provider, event.PSPID, event.EventType, []byte(redactedRaw)).Scan(&webhookID)
	if err == sql.ErrNoRows {
		// Já processado — ACK silencioso
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("idempotency insert: %w", err)
	}

	// Localiza o pagamento local + valida amount (audit C3)
	var paymentID, orderID, method string
	var localAmount float64
	var currentStatus string
	err = tx.QueryRow(`
		SELECT id, order_id::text, method::text, amount, status
		FROM payments WHERE psp_payment_id = $1
	`, event.PSPID).Scan(&paymentID, &orderID, &method, &localAmount, &currentStatus)
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
	// vs o valor que persistimos localmente.
	//
	// COMPARAÇÃO EM CENTAVOS INTEIROS, não em float com tolerância. A versão
	// anterior aceitava `|delta| <= 0.01`, o que abre uma janela real de UM
	// CENTAVO por transação: em volume, é uma sangria silenciosa que nenhum
	// alerta pega. Arredondar os dois lados pra centavo e exigir igualdade
	// exata elimina tanto o ruído de float quanto a brecha. Mesmo raciocínio de
	// internal/psp/appmaxv1/money_test.go e de internal/ledger.
	if centsOf(pspResult.Amount) != centsOf(localAmount) {
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
			"payment_id":     paymentID,
			"psp_payment_id": event.PSPID,
			"provider":       provider,
			"local_amount":   localAmount,
			"psp_amount":     pspResult.Amount,
		})
		_, _ = tx.Exec(`
			INSERT INTO payments_outbox (event_type, payload_json, next_attempt_at)
			VALUES ($1, $2, now())
		`, "payment.fraud_suspect", fraudPayload)
		// ACK pro PSP pra não retry storm — registramos o problema, ops resolve.
		h.mark("rejected_amount")
		return tx.Commit()
	}

	// SEGURANÇA (audit AV1-H1) — O STATUS VEM DO PSP, NÃO DO CORPO DO WEBHOOK.
	//
	// Antes: `mapPSPStatus(event.Status)`, ou seja, o status vinha do BODY. Com a
	// Appmax (que NÃO assina webhook), qualquer um que conheça o endpoint podia
	// POSTar `{"event":"order_paid","data":{"id":<pedido real>,"total":<valor
	// certo>}}` e promover o pagamento a `confirmed`. A validação C3 checava só
	// o VALOR — e o valor certo é trivial de acertar, já que é o preço do
	// produto no catálogo público.
	//
	// Agora o status usado é o de `pspResult`, que veio da re-consulta
	// autenticada GET /v1/orders/{id}. O corpo do webhook passa a ser só um
	// GATILHO ("vai olhar o pedido X"), nunca uma fonte de verdade.
	newStatus := mapPSPStatus(pspResult.Status)

	// Divergência entre o que o webhook alega e o que o PSP confirma é sinal de
	// forja (ou de race de reentrega). Não bloqueia — o PSP mandou — mas fica
	// registrado pra investigação.
	if claimed := mapPSPStatus(event.Status); claimed != newStatus {
		slog.Warn("webhook: status do corpo diverge do status autoritativo do PSP — corpo ignorado",
			"provider", provider,
			"psp_id", event.PSPID,
			"claimed_status", claimed,
			"psp_status", newStatus,
			"request_id", requestID,
		)
	}
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

	if err := tx.Commit(); err != nil {
		return err
	}

	// Lançamento contábil APÓS o commit, de propósito.
	//
	// Fazer dentro da mesma transação seria mais bonito, mas amarraria o
	// caminho crítico do dinheiro ENTRANDO à disponibilidade do livro: um erro
	// no ledger faria o webhook devolver 500, o PSP reentregar, e o pagamento
	// do cliente ficar pendente por um problema contábil. Aqui a ordem de
	// prioridade é explícita — primeiro reconhecer o pagamento, depois
	// escriturar. Se o lançamento falhar, a reconciliação detecta
	// (DiscLedgerMissing) e o alerta dispara; nenhum dinheiro se perde.
	if newStatus == "confirmed" {
		h.postSale(ctx, paymentID, orderID, localAmount, method, requestID)
		if h.metrics != nil {
			h.metrics.PaymentConfirmed(provider, method)
		}
	} else if newStatus == "failed" && h.metrics != nil {
		h.metrics.PaymentFailed(provider, method, "rejected")
	}
	return nil
}

// postSale escritura a venda. Idempotente por (kind, source_type, source_id):
// reentrega do mesmo webhook não dobra a receita.
//
// A taxa do PSP fica em ZERO aqui: a Appmax não informa o MDR efetivo no
// momento da confirmação. Ela entra depois, por ledger.PSPFee, na conciliação
// do extrato. Chutar um percentual seria pior que registrar zero — número
// inventado no livro é indistinguível de número apurado.
func (h *WebhookHandler) postSale(ctx context.Context, paymentID, orderID string, amount float64, method, requestID string) {
	if h.ledger == nil {
		return
	}
	ctx = requestid.NewContext(ctx, requestID)
	_, err := h.ledger.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID:  paymentID,
		OrderID:    orderID,
		OccurredAt: time.Now().UTC(),
		GrossCents: ledger.Cents(centsOf(amount)),
		Method:     method,
		RequestID:  requestID,
	}))
	switch {
	case err == nil:
		if h.metrics != nil {
			h.metrics.LedgerPosted(string(ledger.KindSale))
		}
	case errorsIs(err, ledger.ErrDuplicate):
		// Reentrega — comportamento esperado, não é erro.
		slog.Debug("webhook: venda já lançada no livro", "payment_id", paymentID, "request_id", requestID)
	default:
		// Não derruba o webhook: o pagamento JÁ está confirmado e commitado.
		slog.Error("webhook: FALHA AO LANÇAR VENDA NO LIVRO — receita do período ficará subestimada até a reconciliação",
			"payment_id", paymentID, "order_id", orderID, "error", err, "request_id", requestID)
		if h.metrics != nil {
			h.metrics.LedgerRejected(string(ledger.KindSale), "post_failed")
		}
	}
}

// centsOf converte reais (float64, como vem de NUMERIC(12,2) e de psp.GetResult)
// pra centavos com ARREDONDAMENTO. Truncar perderia um centavo em 19.99, que em
// float64 é 19.989999999999998.
func centsOf(reais float64) int64 { return int64(math.Round(reais * 100)) }

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

// errorsIs é um alias local pra manter o import de `errors` fora do topo deste
// arquivo, que já é longo. Sem mágica: é errors.Is.
func errorsIs(err, target error) bool { return errors.Is(err, target) }
