package appmax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// newGateway aponta o Gateway para um servidor de teste.
func newGateway(baseURL, webhookSecret string) *Gateway {
	return &Gateway{
		client:        NewWithBaseURL("test-token", baseURL),
		webhookSecret: webhookSecret,
	}
}

// mockAppmax responde ao fluxo customer → order → payment.
func mockAppmax(t *testing.T, method psp.PaymentMethod) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Todo request deve carregar o access-token no corpo (convenção Appmax).
		if body["access-token"] != "test-token" {
			t.Errorf("missing access-token in body for %s", r.URL.Path)
		}

		switch r.URL.Path {
		case "/customers":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"customer": map[string]any{"id": 4242}},
			})
		case "/orders":
			if body["products_value"] == nil {
				t.Error("order missing products_value")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"order": map[string]any{"id": 9001, "status": "pendente"}},
			})
		case "/payments/pix":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"pix_qrcode": "data:image/png;base64,AAAA",
					"pix_emv":    "00020126...br.gov.bcb.pix",
				},
			})
		case "/payments/boleto":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"pdf":            "https://appmax.com.br/boleto/9001.pdf",
					"digitable_line": "34191.79001 01043.510047 91020.150008 1 90000000010000",
				},
			})
		case "/payments/credit-card":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "aprovado"},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
		_ = method
	}))
}

func TestCreatePayment_Pix(t *testing.T) {
	srv := mockAppmax(t, psp.MethodPix)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	res, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID:    "550e8400-e29b-41d4-a716-446655440000",
		Amount:     149.90,
		Currency:   "BRL",
		Method:     psp.MethodPix,
		PayerEmail: "cliente@utilar.com.br",
		PayerName:  "Maria Silva",
		PayerCPF:   "123.456.789-09",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.PSPID != "9001" {
		t.Errorf("expected PSPID=9001 (appmax order id), got %q", res.PSPID)
	}
	if res.Status != psp.StatusPending {
		t.Errorf("expected pending, got %q", res.Status)
	}
	// O QR Pix cru deve ser repassado ao SPA.
	if !strings.Contains(string(res.ClientData), "pix_emv") {
		t.Errorf("expected pix_emv in ClientData, got %s", res.ClientData)
	}
}

func TestCreatePayment_BoletoRequiresCPFAndName(t *testing.T) {
	srv := mockAppmax(t, psp.MethodBoleto)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "550e8400-e29b-41d4-a716-446655440000",
		Amount:  50,
		Method:  psp.MethodBoleto,
		// sem CPF/Name
	})
	if err == nil {
		t.Fatal("expected ErrInvalidRequest for boleto without cpf/name")
	}
}

func TestCreatePayment_CardRequiresToken(t *testing.T) {
	srv := mockAppmax(t, psp.MethodCard)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID:    "550e8400-e29b-41d4-a716-446655440000",
		Amount:     50,
		Method:     psp.MethodCard,
		PayerName:  "João",
		PayerEmail: "j@x.com",
		// sem CardToken
	})
	if err == nil {
		t.Fatal("expected ErrInvalidRequest for card without token")
	}
}

func TestGetPayment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/orders/9001") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("access-token") != "test-token" {
			t.Error("expected access-token query param on GET")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"order": map[string]any{
				"id": 9001, "status": "aprovado", "total": 149.90,
			}},
		})
	}))
	defer srv.Close()
	g := newGateway(srv.URL, "")

	res, err := g.GetPayment(context.Background(), "9001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != psp.StatusApproved {
		t.Errorf("expected approved, got %q", res.Status)
	}
	if res.Amount != 149.90 {
		t.Errorf("expected amount 149.90, got %v", res.Amount)
	}
}

func TestVerifyWebhook(t *testing.T) {
	// Sem secret → aceita (segurança via re-consulta GetPayment).
	g := newGateway("http://x", "")
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err != nil {
		t.Errorf("expected nil without secret, got %v", err)
	}

	// Com secret → exige header X-Appmax-Token correto.
	g = newGateway("http://x", "s3cr3t")
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err == nil {
		t.Error("expected error with secret but no header")
	}
	h := http.Header{}
	h.Set("X-Appmax-Token", "wrong")
	if err := g.VerifyWebhook([]byte(`{}`), h); err == nil {
		t.Error("expected error with wrong token")
	}
	h.Set("X-Appmax-Token", "s3cr3t")
	if err := g.VerifyWebhook([]byte(`{}`), h); err != nil {
		t.Errorf("expected nil with correct token, got %v", err)
	}
}

func TestParseWebhookEvent(t *testing.T) {
	g := newGateway("http://x", "")

	// snake_case order_paid → approved
	ev, err := g.ParseWebhookEvent([]byte(`{"event":"order_paid","data":{"id":9001,"status":"aprovado"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil || ev.PSPID != "9001" || ev.Status != psp.StatusApproved {
		t.Errorf("unexpected event: %+v", ev)
	}

	// CamelCase OrderPaidByPix → approved
	ev, _ = g.ParseWebhookEvent([]byte(`{"event":"OrderPaidByPix","data":{"id":"9002"}}`))
	if ev == nil || ev.EventType != "order_paid_by_pix" || ev.Status != psp.StatusApproved {
		t.Errorf("unexpected camelcase event: %+v", ev)
	}

	// order_pix_expired → expired
	ev, _ = g.ParseWebhookEvent([]byte(`{"event":"order_pix_expired","data":{"id":9003}}`))
	if ev == nil || ev.Status != psp.StatusExpired {
		t.Errorf("expected expired, got %+v", ev)
	}

	// sem id → irrelevante
	ev, err = g.ParseWebhookEvent([]byte(`{"event":"ping","data":{}}`))
	if err != nil || ev != nil {
		t.Errorf("expected (nil,nil) for event without id, got ev=%+v err=%v", ev, err)
	}
}

func TestHelpers(t *testing.T) {
	if f, l := splitName("Maria Aparecida Silva"); f != "Maria" || l != "Aparecida Silva" {
		t.Errorf("splitName: got %q / %q", f, l)
	}
	if f, l := splitName("Cher"); f != "Cher" || l != "" {
		t.Errorf("splitName single: got %q / %q", f, l)
	}
	if got := digitsOnly("123.456.789-09"); got != "12345678909" {
		t.Errorf("digitsOnly: got %q", got)
	}
	if got := normalizeEventName("OrderBilletCreated"); got != "order_billet_created" {
		t.Errorf("normalizeEventName: got %q", got)
	}
	if normalizeStatus("estornado") != psp.StatusCancelled {
		t.Error("estornado should map to cancelled")
	}
}
