// Testa o cross-service amount/ownership do PaymentHandler.Create (audit C1+C2).
//
// C1: amount usado no PSP vem do order-service, não do body. Cliente que envia
//
//	`amount: 0.01` num pedido de R$ 5000 paga 5000.
//
// C2: order_id que não pertence ao user retorna 404 (cliente do order-service
//
//	já filtra por user_id; payment-service confia nessa garantia).
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
	"github.com/utilar/payment-service/internal/authclient"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/orderclient"
	"github.com/utilar/payment-service/internal/psp"
)

// stubUserLookup devolve um User de cadastro fixo (pra testar precedência do CPF
// do formulário sobre o do cadastro no boleto).
type stubUserLookup struct {
	user *authclient.User
	err  error
}

func (s *stubUserLookup) Me(ctx context.Context, jwt string) (*authclient.User, error) {
	return s.user, s.err
}

// --- mocks ----------------------------------------------------------------

type stubGateway struct{}

func (s *stubGateway) Name() string { return "stripe" }
func (s *stubGateway) Refund(context.Context, psp.RefundRequest) (*psp.RefundResult, error) {
	return nil, psp.ErrNotSupported
}

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

func (s *stubGatewayCapture) Refund(context.Context, psp.RefundRequest) (*psp.RefundResult, error) {
	return nil, psp.ErrNotSupported
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

// makeCardReq monta um POST de cartão com token e parcelas.
func makeCardReq(t *testing.T, installments int) *http.Request {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"order_id":     testOrderID,
		"method":       "card",
		"amount":       99.90,
		"card_token":   "tok_browser_x",
		"installments": installments,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake-jwt-for-propagation")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func pendingOrder(total float64) *stubOrderClient {
	return &stubOrderClient{order: &orderclient.Order{
		ID: testOrderID, UserID: testUserID, Status: "pending_payment", Total: total,
	}}
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

// PayerIP vem da CONEXÃO (c.ClientIP()), NUNCA do body — cliente não dita o IP.
func TestCreate_PayerIPFromConnection(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	r, cleanup := setupPaymentRouter(t, gw, pendingOrder(99.90), false)
	defer cleanup()

	req := makePaymentReq(t, 99.90)
	req.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", w.Code, w.Body.String())
	}
	if gw.lastReq.PayerIP != "203.0.113.9" {
		t.Errorf("PayerIP = %q, esperava o IP da conexão", gw.lastReq.PayerIP)
	}
}

// Os itens e o frete do pedido (autoritativos) chegam ao PSP pra itemização.
func TestCreate_ItemsAndShippingReachGateway(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	oc := &stubOrderClient{order: &orderclient.Order{
		ID: testOrderID, UserID: testUserID, Status: "pending_payment",
		Total: 115.00, ShippingCost: 15.00,
		Items: []orderclient.OrderItem{
			{ProductID: "p1", Name: "Parafuso", Quantity: 2, UnitPrice: 40.00},
			{ProductID: "p2", Name: "Furadeira", Quantity: 1, UnitPrice: 20.00},
		},
	}}
	r, cleanup := setupPaymentRouter(t, gw, oc, false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makePaymentReq(t, 115.00))

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", w.Code, w.Body.String())
	}
	if len(gw.lastReq.Items) != 2 {
		t.Fatalf("PSP recebeu %d itens, esperava 2", len(gw.lastReq.Items))
	}
	if gw.lastReq.Items[0].Ref != "p1" || gw.lastReq.Items[0].Quantity != 2 || gw.lastReq.Items[0].UnitPrice != 40.00 {
		t.Errorf("item 0 = %+v", gw.lastReq.Items[0])
	}
	if gw.lastReq.Shipping != 15.00 {
		t.Errorf("Shipping = %v, esperava 15.00", gw.lastReq.Shipping)
	}
}

// Parcelas escolhidas pelo comprador chegam ao PSP (dentro do teto).
func TestCreate_InstallmentsPassThrough(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	r, cleanup := setupPaymentRouter(t, gw, pendingOrder(99.90), false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeCardReq(t, 6))

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (%s)", w.Code, w.Body.String())
	}
	if gw.lastReq.Installments != 6 {
		t.Errorf("Installments = %d, esperava 6", gw.lastReq.Installments)
	}
	if gw.lastReq.CardToken != "tok_browser_x" {
		t.Errorf("CardToken = %q, esperava o token do browser", gw.lastReq.CardToken)
	}
}

// Parcelas acima do teto são recusadas ANTES do PSP (erro limpo).
func TestCreate_InstallmentsAboveCapRejected(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	r, cleanup := setupPaymentRouter(t, gw, pendingOrder(99.90), false)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeCardReq(t, 13)) // teto é 12

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 para parcelas acima do teto, got %d (%s)", w.Code, w.Body.String())
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

// setupBoletoRouter monta o handler COM authClient stub (os outros testes passam
// nil). Necessário pra exercitar a lógica de precedência de CPF do boleto.
func setupBoletoRouter(t *testing.T, gw psp.Gateway, oc handler.OrderLookup, auth handler.UserLookup) (*gin.Engine, func()) {
	t.Helper()
	db := setupTestDB(t)
	_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, testOrderID)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	r.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Set("user_email", "test@utilar.dev")
		c.Next()
	})
	pH := handler.NewPaymentHandler(db, gw, oc, auth, false)
	r.POST("/api/v1/payments", pH.Create)

	return r, func() {
		_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, testOrderID)
		db.Close()
	}
}

func boletoReq(cpf, name string) *http.Request {
	body, _ := json.Marshal(map[string]any{
		"order_id":   testOrderID,
		"method":     "boleto",
		"amount":     100.00,
		"payer_cpf":  cpf,
		"payer_name": name,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake-jwt")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestRegression_BoletoUsaCPFDoFormularioNaoDoCadastro trava o bug pego no demo
// (2026-08-28): o cliente digitava um CPF VÁLIDO no checkout, mas o payment-service
// SOBRESCREVIA com o CPF do cadastro do usuário logado. Como o usuário de teste
// tinha um CPF inválido salvo (12345678901), TODO boleto falhava na Stripe com
// tax_id_invalid ("CPF inválido") mesmo o cliente acertando o número. O CPF do
// FORM tem que ter precedência.
func TestRegression_BoletoUsaCPFDoFormularioNaoDoCadastro(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	badCPF := "12345678901" // o CPF inválido salvo no perfil de teste
	auth := &stubUserLookup{user: &authclient.User{Name: "Cadastro Antigo", CPF: &badCPF}}
	r, cleanup := setupBoletoRouter(t, gw, pendingOrder(100.00), auth)
	defer cleanup()

	const formCPF = "54045797068" // válido e diferente do cadastro
	w := httptest.NewRecorder()
	r.ServeHTTP(w, boletoReq(formCPF, "Ana Silva"))

	if gw.lastReq.PayerCPF != formCPF {
		t.Fatalf("boleto deve usar o CPF do FORM (%s), não o do cadastro (%s); PSP recebeu %q",
			formCPF, badCPF, gw.lastReq.PayerCPF)
	}
	if gw.lastReq.PayerName != "Ana Silva" {
		t.Fatalf("boleto deve usar o NOME do form (Ana Silva); PSP recebeu %q", gw.lastReq.PayerName)
	}
}

// TestBoletoUsaCadastroQuandoFormVazio garante que o FALLBACK do M6 sobrevive: se
// o form não manda CPF, o do cadastro preenche (o PSP rejeita boleto sem CPF).
func TestBoletoUsaCadastroQuandoFormVazio(t *testing.T) {
	gw := &stubGatewayCapture{stubGateway: &stubGateway{}}
	cadCPF := "54045797068"
	auth := &stubUserLookup{user: &authclient.User{Name: "Do Cadastro", CPF: &cadCPF}}
	r, cleanup := setupBoletoRouter(t, gw, pendingOrder(100.00), auth)
	defer cleanup()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, boletoReq("", "")) // form vazio → cai no cadastro

	if gw.lastReq.PayerCPF != cadCPF {
		t.Fatalf("form vazio deve cair no CPF do cadastro (%s); PSP recebeu %q", cadCPF, gw.lastReq.PayerCPF)
	}
	if gw.lastReq.PayerName != "Do Cadastro" {
		t.Fatalf("form vazio deve cair no nome do cadastro; PSP recebeu %q", gw.lastReq.PayerName)
	}
}
