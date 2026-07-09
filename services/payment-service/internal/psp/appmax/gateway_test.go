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

func newGateway(baseURL, webhookSecret string) *Gateway {
	return &Gateway{
		client:        NewWithBaseURL("test-token", baseURL),
		webhookSecret: webhookSecret,
	}
}

// mockAppmaxV3 responde ao fluxo v3: /customer → /order → /payment/*.
func mockAppmaxV3(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["access-token"] != "test-token" {
				t.Errorf("faltando access-token no corpo em %s", r.URL.Path)
			}
			if r.Header.Get("access-token") != "test-token" {
				t.Errorf("faltando header access-token em %s", r.URL.Path)
			}
		}

		switch {
		case r.URL.Path == "/customer":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "text": "OK", "status": 200,
				"data": map[string]any{"id": 34227},
			})
		case r.URL.Path == "/order":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "status": 200,
				"data": map[string]any{"id": 70263, "status": "pendente", "total": 149.9},
			})
		case r.URL.Path == "/payment/pix":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": "ATIVA", "text": "Transação efetuada com sucesso", "status": 200,
				"data": map[string]any{
					"type": "Pix", "pix_qrcode": "<PNG base64>",
					"pix_emv": "00020101...br.gov.bcb.pix", "pix_expiration_date": "2026-06-10 09:07:05",
				},
			})
		case r.URL.Path == "/payment/boleto":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "status": 200,
				"data": map[string]any{
					"type": "Boleto", "pdf": "https://dev-boletos.appmax.com.br/x.pdf",
					"due_date": "2026-06-13", "digitable_line": "34191.79...",
				},
			})
		case r.URL.Path == "/payment/credit-card":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "text": "Autorização e Captura realizada com sucesso", "status": 200,
				"data": map[string]any{"type": "CreditCard", "pay_reference": "abc"},
			})
		case strings.HasPrefix(r.URL.Path, "/order/"): // GET /order/:id
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "status": 200,
				"data": map[string]any{"id": 70263, "status": "aprovado", "total": 149.9},
			})
		default:
			t.Errorf("path inesperado: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCreatePayment_Pix(t *testing.T) {
	srv := mockAppmaxV3(t)
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
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.PSPID != "70263" {
		t.Errorf("PSPID esperado 70263 (order id), veio %q", res.PSPID)
	}
	if res.Status != psp.StatusPending {
		t.Errorf("esperado pending, veio %q", res.Status)
	}
	// ClientData normalizado deve trazer o EMV do Pix.
	if !strings.Contains(string(res.ClientData), "br.gov.bcb.pix") {
		t.Errorf("esperava pix_emv em ClientData, veio %s", res.ClientData)
	}
}

func TestCreatePayment_Boleto(t *testing.T) {
	srv := mockAppmaxV3(t)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	res, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "550e8400-e29b-41d4-a716-446655440000",
		Amount:  50, Method: psp.MethodBoleto,
		PayerName: "João Souza", PayerCPF: "390.533.447-05",
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !strings.Contains(string(res.ClientData), "dev-boletos.appmax.com.br") {
		t.Errorf("esperava pdf do boleto em ClientData, veio %s", res.ClientData)
	}
}

func TestCreatePayment_BoletoRequiresCPFAndName(t *testing.T) {
	srv := mockAppmaxV3(t)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "550e8400-e29b-41d4-a716-446655440000", Amount: 50, Method: psp.MethodBoleto,
	})
	if err == nil {
		t.Fatal("esperava ErrInvalidRequest para boleto sem cpf/name")
	}
}

func TestCreatePayment_CardRequiresToken(t *testing.T) {
	srv := mockAppmaxV3(t)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "550e8400-e29b-41d4-a716-446655440000", Amount: 50, Method: psp.MethodCard,
		PayerName: "João", PayerEmail: "j@x.com",
	})
	if err == nil {
		t.Fatal("esperava ErrInvalidRequest para cartão sem token")
	}
}

func TestGetPayment(t *testing.T) {
	srv := mockAppmaxV3(t)
	defer srv.Close()
	g := newGateway(srv.URL, "")

	res, err := g.GetPayment(context.Background(), "70263")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res.Status != psp.StatusApproved {
		t.Errorf("esperado approved, veio %q", res.Status)
	}
	if res.Amount != 149.9 {
		t.Errorf("esperado amount 149.9, veio %v", res.Amount)
	}
}

func TestVerifyWebhook(t *testing.T) {
	g := newGateway("http://x", "")
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err != nil {
		t.Errorf("sem secret deveria aceitar, veio %v", err)
	}

	g = newGateway("http://x", "s3cr3t")
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err == nil {
		t.Error("com secret e sem header deveria falhar")
	}
	h := http.Header{}
	h.Set("X-Appmax-Token", "s3cr3t")
	if err := g.VerifyWebhook([]byte(`{}`), h); err != nil {
		t.Errorf("token correto deveria passar, veio %v", err)
	}
}

func TestParseWebhookEvent(t *testing.T) {
	g := newGateway("http://x", "")

	// DefaultResponse: PascalCase OrderApproved, data.id/data.status
	ev, err := g.ParseWebhookEvent([]byte(`{"environment":"production","event":"OrderApproved","data":{"id":3173109,"status":"aprovado"}}`))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ev == nil || ev.PSPID != "3173109" || ev.Status != psp.StatusApproved {
		t.Errorf("evento inesperado: %+v", ev)
	}
	if ev.EventType != "orderapproved" {
		t.Errorf("EventType esperado orderapproved, veio %q", ev.EventType)
	}

	// TwoLevel: data.order_id/data.order_status
	ev, _ = g.ParseWebhookEvent([]byte(`{"event":"OrderPaid","data":{"order_id":3173109,"order_status":"aprovado"}}`))
	if ev == nil || ev.PSPID != "3173109" || ev.Status != psp.StatusApproved {
		t.Errorf("TwoLevel inesperado: %+v", ev)
	}

	// evento sem order id (informativo) → (nil,nil)
	ev, err = g.ParseWebhookEvent([]byte(`{"event":"CustomerCreated","data":{"customer_id":1}}`))
	if err != nil || ev != nil {
		t.Errorf("esperava (nil,nil) para evento sem order id, veio ev=%+v err=%v", ev, err)
	}
}

func TestHelpers(t *testing.T) {
	if f, l := splitName("Maria Aparecida Silva"); f != "Maria" || l != "Aparecida Silva" {
		t.Errorf("splitName: %q / %q", f, l)
	}
	if got := digitsOnly("123.456.789-09"); got != "12345678909" {
		t.Errorf("digitsOnly: %q", got)
	}
	if got := normEvent("OrderPaidByPix"); got != "orderpaidbypix" {
		t.Errorf("normEvent: %q", got)
	}
	if normalizeStatus("integrado") != psp.StatusApproved {
		t.Error("integrado deveria mapear pra approved")
	}
	if normalizeStatus("estornado") != psp.StatusCancelled {
		t.Error("estornado deveria mapear pra cancelled")
	}
}
