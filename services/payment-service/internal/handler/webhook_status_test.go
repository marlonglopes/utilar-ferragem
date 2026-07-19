package handler_test

// REGRESSÃO AV1-H1 — a mais grave da auditoria do appmaxv1.
//
// O handler decidia o status a partir do CORPO do webhook (`event.Status`).
// A Appmax NÃO assina webhooks: qualquer um que conheça o endpoint podia POSTar
//
//	{"event":"order_paid","data":{"id":<pedido real>,"total":<valor certo>}}
//
// e promover o pagamento a `confirmed` sem ter pago nada. A validação C3 checava
// só o VALOR — e o valor certo é trivial de acertar: é o preço do produto no
// catálogo público.
//
// Agora o status vem de `GetPayment` (re-consulta autenticada ao PSP) e o corpo
// do webhook é apenas um GATILHO ("vá olhar o pedido X").

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/psp"
)

// seedPaymentForWebhook cria um pagamento pendente e devolve (id, pspID).
func seedPaymentForWebhook(t *testing.T, db *sql.DB, amount float64) (string, string) {
	t.Helper()
	// Sufixo único por chamada: a idempotência do webhook é por
	// (provider, psp_payment_id, event_type) e reexecuções colidiriam.
	pspID := fmt.Sprintf("psp-forja-%d", time.Now().UnixNano())
	var id string
	err := db.QueryRow(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id)
		VALUES (gen_random_uuid(), gen_random_uuid(), 'pix', 'pending', $1, $2)
		RETURNING id`, amount, pspID).Scan(&id)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM webhook_events WHERE psp_payment_id=$1`, pspID)
		_, _ = db.Exec(`DELETE FROM payments WHERE id=$1`, id)
	})
	return id, pspID
}

// O ATAQUE: webhook forjado alegando "approved" num pagamento que o PSP ainda
// considera pendente. O valor bate (é público), a assinatura não existe.
func TestWebhookForjadoNaoConfirmaPagamentoQueOPSPDizPendente(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)

	paymentID, pspID := seedPaymentForWebhook(t, db, 264.00)

	gw := &mockGateway{
		name: "appmax-v1",
		// O corpo do webhook ALEGA aprovado...
		parsedEvent: &psp.WebhookEvent{
			EventType: "orderpaid", PSPID: pspID,
			Status: psp.StatusApproved, Amount: 264.00,
		},
		// ...mas o PSP, consultado, diz que continua PENDENTE.
		getResult: &psp.GetResult{
			PSPID: pspID, Status: psp.StatusPending, Amount: 264.00, Currency: "BRL",
		},
	}

	r := gin.New()
	r.POST("/webhooks/:provider", handler.NewWebhookHandler(db, gw).Handle)

	body, _ := json.Marshal(map[string]any{
		"event": "order_paid",
		"data":  map[string]any{"id": pspID, "total": 26400, "status": "aprovado"},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/appmax-v1", bytes.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status HTTP = %d (o ACK é esperado; o que não pode é confirmar)", w.Code)
	}

	var status string
	var confirmedAt sql.NullTime
	if err := db.QueryRow(`SELECT status, confirmed_at FROM payments WHERE id=$1`, paymentID).
		Scan(&status, &confirmedAt); err != nil {
		t.Fatal(err)
	}
	if status == "confirmed" {
		t.Fatal("WEBHOOK FORJADO CONFIRMOU O PAGAMENTO — o corpo não assinado voltou a ser fonte de verdade")
	}
	if status != "pending" {
		t.Errorf("status = %q, esperado pending (o PSP diz pendente)", status)
	}
	if confirmedAt.Valid {
		t.Error("confirmed_at foi preenchido sem confirmação do PSP")
	}
}

// O caminho legítimo continua funcionando: PSP confirma → nós confirmamos.
func TestWebhookConfirmaQuandoOPSPConfirma(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)

	paymentID, pspID := seedPaymentForWebhook(t, db, 34.90)

	gw := &mockGateway{
		name: "appmax-v1",
		parsedEvent: &psp.WebhookEvent{
			EventType: "orderpaid", PSPID: pspID, Status: psp.StatusApproved, Amount: 34.90,
		},
		getResult: &psp.GetResult{
			PSPID: pspID, Status: psp.StatusApproved, Amount: 34.90, Currency: "BRL",
		},
	}

	r := gin.New()
	r.POST("/webhooks/:provider", handler.NewWebhookHandler(db, gw).Handle)

	body, _ := json.Marshal(map[string]any{"event": "order_paid", "data": map[string]any{"id": pspID}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/appmax-v1", bytes.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status HTTP = %d", w.Code)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM payments WHERE id=$1`, paymentID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "confirmed" {
		t.Fatalf("status = %q, esperado confirmed — o caminho legítimo quebrou", status)
	}
}

// O inverso do ataque: corpo diz "recusado", PSP diz "aprovado". O corpo
// também não pode CANCELAR um pagamento legítimo (negação de serviço no
// checkout de um concorrente, por exemplo).
func TestWebhookForjadoNaoFalhaPagamentoQueOPSPAprovou(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)

	paymentID, pspID := seedPaymentForWebhook(t, db, 99.90)

	gw := &mockGateway{
		name: "appmax-v1",
		parsedEvent: &psp.WebhookEvent{
			EventType: "orderrefusedbyrisk", PSPID: pspID, Status: psp.StatusRejected, Amount: 99.90,
		},
		getResult: &psp.GetResult{
			PSPID: pspID, Status: psp.StatusApproved, Amount: 99.90, Currency: "BRL",
		},
	}

	r := gin.New()
	r.POST("/webhooks/:provider", handler.NewWebhookHandler(db, gw).Handle)

	body, _ := json.Marshal(map[string]any{"event": "order_refused_by_risk", "data": map[string]any{"id": pspID}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/appmax-v1", bytes.NewReader(body)))

	var status string
	if err := db.QueryRow(`SELECT status FROM payments WHERE id=$1`, paymentID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status == "failed" {
		t.Fatal("WEBHOOK FORJADO DERRUBOU UM PAGAMENTO APROVADO PELO PSP")
	}
	if status != "confirmed" {
		t.Errorf("status = %q, esperado confirmed (o PSP aprovou)", status)
	}
}
