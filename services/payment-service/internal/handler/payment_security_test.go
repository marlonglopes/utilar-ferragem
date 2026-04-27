// Testa o cross-service amount/ownership do PaymentHandler.Create (audit C1+C2).
//
// C1: amount usado no PSP vem do order-service, não do body. Cliente que envia
//     `amount: 0.01` num pedido de R$ 5000 paga 5000.
// C2: order_id que não pertence ao user retorna 404 (cliente do order-service
//     já filtra por user_id; payment-service confia nessa garantia).
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/orderclient"
	"github.com/utilar/payment-service/internal/psp"
)

// --- mocks ----------------------------------------------------------------

type stubGateway struct{}

func (s *stubGateway) Name() string { return "stripe" }
func (s *stubGateway) CreatePayment(ctx context.Context, r psp.CreateRequest) (*psp.CreateResult, error) {
	// Captura o amount efetivamente enviado ao PSP via field do struct (não usado
	// aqui — usamos o stubGatewayCapture pra inspecionar). O test default não
	// precisa inspecionar o amount no PSP — ele inspeciona o INSERT no DB.
	return &psp.CreateResult{
		PSPID:        "pi_test_123",
		Status:       psp.StatusPending,
		ClientSecret: "cs_test",
		ClientData:   json.RawMessage(`{"type":"card"}`),
		RawPayload:   json.RawMessage(`{}`),
	}, nil
}
func (s *stubGateway) GetPayment(ctx context.Context, id string) (*psp.GetResult, error) {
	return nil, errors.New("not used")
}
func (s *stubGateway) VerifyWebhook(b []byte, h http.Header) error { return nil }
func (s *stubGateway) ParseWebhookEvent(b []byte) (*psp.WebhookEvent, error) {
	return nil, nil
}

// stubGatewayCapture captura o último CreatePayment recebido para inspeção.
type stubGatewayCapture struct {
	*stubGateway
	lastReq psp.CreateRequest
}

func (s *stubGatewayCapture) CreatePayment(ctx context.Context, r psp.CreateRequest) (*psp.CreateResult, error) {
	s.lastReq = r
	return s.stubGateway.CreatePayment(ctx, r)
}

// stubOrderClient retorna um order pré-definido ou um erro fixo.
type stubOrderClient struct {
	order *orderclient.Order
	err   error
	calls int
}

func (s *stubOrderClient) Get(ctx context.Context, orderID, jwt string) (*orderclient.Order, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.order, nil
}

// --- helpers -----------------------------------------------------------------

const testOrderID = "11111111-1111-1111-1111-111111111111"
const testUserID = "00000000-0000-0000-0000-000000000099"

func setupPaymentRouter(t *testing.T, gw psp.Gateway, oc handler.OrderLookup, devMode bool) (*gin.Engine, func()) {
	t.Helper()
	db := setupTestDB(t)

	// Limpa qualquer payment do test order
	_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, testOrderID)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	// "Auth" stub — seta user_id no contexto
	r.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Set("user_email", "test@utilar.dev")
		c.Next()
	})
	pH := handler.NewPaymentHandler(db, gw, oc, nil, devMode)
	r.POST("/api/v1/payments", pH.Create)

	cleanup := func() {
		_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, testOrderID)
		db.Close()
	}
	return r, cleanup
}

func makePaymentReq(t *testing.T, amount float64) *http.Request {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"order_id": testOrderID,
		"method":   "pix",
		"amount":   amount,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake-jwt-for-propagation")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// --- tests -------------------------------------------------------------------

// C1: amount cobrado vem do order-service, não do body.
func TestCreate_AmountTamperBlocked_UsesOrderTotal(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{
		order: &orderclient.Order{
			ID:     testOrderID,
			UserID: testUserID,
			Status: "pending_payment",
			Total:  5000.00, // pedido caro
		},
	}

	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	// Cliente tenta tampering com amount: 0.01
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 0.01))

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (body=%s)", w.Code, w.Body.String())
	}

	// Amount enviado pro PSP deve ser 5000.00, NÃO 0.01
	if gw.lastReq.Amount != 5000.00 {
		t.Errorf("PSP recebeu amount tampered: got %.2f, want 5000.00", gw.lastReq.Amount)
	}
}

// C2: order que não pertence ao user → 404
func TestCreate_OrderNotFoundOrNotOwned_Returns404(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{err: orderclient.ErrNotFound}

	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 100.0))

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d (body=%s)", w.Code, w.Body.String())
	}
	if oc.calls != 1 {
		t.Errorf("orderClient.Get not called: %d", oc.calls)
	}
}

// C2: order de outro usuário (defesa em profundidade — order-service já filtra,
// mas sanity check no payment caso JWT cross-service esteja errado)
func TestCreate_OrderUserMismatch_Returns404(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{
		order: &orderclient.Order{
			ID:     testOrderID,
			UserID: "different-user-uuid", // ≠ testUserID
			Status: "pending_payment",
			Total:  100.0,
		},
	}

	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 100.0))

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for user mismatch, got %d", w.Code)
	}
}

// Pedido em status diferente de pending_payment → 400
func TestCreate_OrderAlreadyPaid_Returns400(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{
		order: &orderclient.Order{
			ID:     testOrderID,
			UserID: testUserID,
			Status: "paid", // já pago
			Total:  100.0,
		},
	}

	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 100.0))

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for already-paid order, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// Sem JWT no header → 401 (mesmo com user_id setado pelo middleware stub)
func TestCreate_MissingBearerToken_Returns401(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{
		order: &orderclient.Order{ID: testOrderID, UserID: testUserID, Status: "pending_payment", Total: 100.0},
	}

	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	body, _ := json.Marshal(map[string]any{"order_id": testOrderID, "method": "pix", "amount": 100.0})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Sem Authorization

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 without bearer, got %d", w.Code)
	}
}

// DevMode + orderClient nil → permite (sem validação cross-service, com warning).
// Em prod, isso seria 500 (config bug).
func TestCreate_DevModeWithoutOrderClient_AllowsWithBodyAmount(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}

	r, cleanup := setupPaymentRouter(t, gw, nil, true) // devMode=true, oc=nil
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 99.90))

	if w.Code != http.StatusCreated {
		t.Errorf("want 201 in dev mode, got %d (body=%s)", w.Code, w.Body.String())
	}
	// Em dev, amount do body é usado
	if gw.lastReq.Amount != 99.90 {
		t.Errorf("expected body amount 99.90 in dev mode, got %.2f", gw.lastReq.Amount)
	}
}

// Prod + orderClient nil → 500 (config bug — fail-closed).
func TestCreate_ProdWithoutOrderClient_Returns500(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}

	r, cleanup := setupPaymentRouter(t, gw, nil, false) // devMode=false, oc=nil
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 99.90))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 in prod without orderClient, got %d", w.Code)
	}
}
