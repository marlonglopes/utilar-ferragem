// Integration tests do webhook handler. Requerem PAYMENT_DB_URL apontando pro
// test DB. Rodar com: make test-integration
//
// Cobre:
//   - Idempotência (audit foundation)
//   - Assinatura inválida → 401 (audit C4 — delegado pro Gateway, mas verifica fluxo)
//   - Provider mismatch → 404 (audit defensivo)
//   - Amount mismatch entre PSP e DB → não confirma + flag fraud_suspect (audit C3)
package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/psp"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	return db
}

// mockGateway é uma implementação stubbada de psp.Gateway para os webhook tests.
// Permite controlar o que VerifyWebhook/ParseWebhookEvent/GetPayment retornam.
type mockGateway struct {
	name           string
	verifyErr      error
	parsedEvent    *psp.WebhookEvent
	parseErr       error
	getResult      *psp.GetResult
	getErr         error
}

func (m *mockGateway) Name() string                                                    { return m.name }
func (m *mockGateway) CreatePayment(ctx context.Context, r psp.CreateRequest) (*psp.CreateResult, error) {
	return nil, errors.New("not implemented")
}
func (m *mockGateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	return m.getResult, m.getErr
}
func (m *mockGateway) VerifyWebhook(body []byte, headers http.Header) error {
	return m.verifyErr
}
func (m *mockGateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	return m.parsedEvent, m.parseErr
}

func TestWebhookIdempotency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert um payment local com amount conhecido, pra match com o GetPayment do mock
	const pspID = "test-idempotency-001"
	_, err := db.Exec(`DELETE FROM webhook_events WHERE psp_payment_id = $1`, pspID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DELETE FROM payments WHERE psp_payment_id = $1`, pspID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id)
		VALUES ('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000099', 'pix', 'pending', 99.90, $1)
	`, pspID)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}

	gw := &mockGateway{
		name:        "mercadopago",
		parsedEvent: &psp.WebhookEvent{EventType: "payment.updated", PSPID: pspID, Status: psp.StatusApproved, Amount: 99.90},
		getResult:   &psp.GetResult{PSPID: pspID, Status: psp.StatusApproved, Amount: 99.90},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewWebhookHandler(db, gw)
	r.POST("/webhooks/:provider", h.Handle)

	body := []byte(`{"type":"payment","action":"payment.updated","data":{"id":"test-idempotency-001"}}`)

	// First call — sucesso (200)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/webhooks/mercadopago", bytes.NewReader(body)))
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: want 200, got %d (body=%s)", w1.Code, w1.Body.String())
	}

	// Second call — idempotente (200)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/webhooks/mercadopago", bytes.NewReader(body)))
	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate call: want 200, got %d", w2.Code)
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gw := &mockGateway{
		name:      "mercadopago",
		verifyErr: psp.ErrInvalidSignature,
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewWebhookHandler(db, gw)
	r.POST("/webhooks/:provider", h.Handle)

	body := []byte(`{"type":"payment"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/mercadopago", bytes.NewReader(body)))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestWebhookProviderMismatch(t *testing.T) {
	// Gateway é "mercadopago" mas request vai pra /webhooks/stripe → 404
	gw := &mockGateway{name: "mercadopago"}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewWebhookHandler(nil, gw) // db não usado neste path
	r.POST("/webhooks/:provider", h.Handle)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader([]byte(`{}`))))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for provider mismatch, got %d", w.Code)
	}
}

// AMOUNT MISMATCH (audit C3): atacante reenvia webhook de pagamento real de
// centavos pra disparar confirmação de pedido caro. Validação compara o que o
// PSP autoritativamente diz contra o que persistimos local. Se diverge, NÃO
// confirma + marca flag fraud_suspect + emite outbox event.
func TestWebhookAmountMismatchRejectsConfirmation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	const pspID = "test-amount-mismatch-001"
	_, err := db.Exec(`DELETE FROM webhook_events WHERE psp_payment_id = $1`, pspID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DELETE FROM payments WHERE psp_payment_id = $1`, pspID)
	if err != nil {
		t.Fatal(err)
	}
	// Amount LOCAL: R$ 5000 (pedido caro). PSP volta com R$ 0.01 (atacante).
	_, err = db.Exec(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id)
		VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000099', 'pix', 'pending', 5000.00, $1)
	`, pspID)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}

	gw := &mockGateway{
		name:        "mercadopago",
		parsedEvent: &psp.WebhookEvent{EventType: "payment.updated", PSPID: pspID, Status: psp.StatusApproved, Amount: 0.01},
		getResult:   &psp.GetResult{PSPID: pspID, Status: psp.StatusApproved, Amount: 0.01},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewWebhookHandler(db, gw)
	r.POST("/webhooks/:provider", h.Handle)

	body := []byte(`{"data":{"id":"test-amount-mismatch-001"}}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/webhooks/mercadopago", bytes.NewReader(body)))

	// Webhook deve retornar 200 (ack pra evitar retry storm)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	// Status NÃO deve ter sido promovido pra confirmed
	var status string
	var metadata []byte
	err = db.QueryRow(`SELECT status, psp_metadata FROM payments WHERE psp_payment_id = $1`, pspID).Scan(&status, &metadata)
	if err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("status was promoted despite mismatch: got %q, want pending", status)
	}

	// Flag fraud_suspect deve ter sido escrita em psp_metadata
	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err != nil {
		t.Fatalf("metadata not JSON: %v", err)
	}
	if mismatch, ok := meta["amount_mismatch"].(bool); !ok || !mismatch {
		t.Errorf("psp_metadata missing amount_mismatch flag: %v", meta)
	}

	// Evento de fraud_suspect deve ter sido enfileirado no outbox
	var fraudEventCount int
	err = db.QueryRow(`SELECT count(*) FROM payments_outbox WHERE event_type = 'payment.fraud_suspect' AND payload_json::text LIKE $1`, "%"+pspID+"%").Scan(&fraudEventCount)
	if err != nil {
		t.Fatal(err)
	}
	if fraudEventCount == 0 {
		t.Error("expected payment.fraud_suspect outbox event for mismatch")
	}
}
