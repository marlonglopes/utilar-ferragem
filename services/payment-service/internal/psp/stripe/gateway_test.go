// Testa a Gateway Stripe focando no que rodamos sem fazer chamada real:
// 1. Mapeamento de status (normalizeStatus)
// 2. Validações no CreatePayment (campos obrigatórios pra boleto, amount > 0)
// 3. Verificação de webhook (com e sem secret)
// 4. ParseWebhookEvent (filtro de tipo + extração de PaymentIntent)
// 5. extractClientData (presença de campos pro frontend)
//
// O que NÃO testamos aqui: chamadas reais à API Stripe (paymentintent.New, .Get).
// Isso fica num teste de integração separado quando rodamos contra `stripe-mock`
// ou conta de teste real (já validado manualmente em 2026-04-26).
package stripe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		stripe string
		want   psp.PaymentStatus
	}{
		{"succeeded", psp.StatusApproved},
		{"processing", psp.StatusPending},
		{"requires_payment_method", psp.StatusPending},
		{"requires_confirmation", psp.StatusPending},
		{"requires_action", psp.StatusPending},
		{"requires_capture", psp.StatusAuthorized},
		{"canceled", psp.StatusCancelled},
		{"unknown", psp.StatusPending},
		{"", psp.StatusPending},
	}

	for _, c := range cases {
		t.Run(c.stripe, func(t *testing.T) {
			got := normalizeStatus(c.stripe)
			if got != c.want {
				t.Errorf("normalizeStatus(%q) = %q, want %q", c.stripe, got, c.want)
			}
		})
	}
}

func TestCreatePaymentValidation(t *testing.T) {
	g := New("sk_test_dummy", "")

	cases := []struct {
		name        string
		req         psp.CreateRequest
		wantErrIs   error
		errContains string
	}{
		{
			name: "amount=0 rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1",
				Amount: 0, Method: psp.MethodCard,
			},
			wantErrIs:   psp.ErrInvalidRequest,
			errContains: "amount",
		},
		{
			name: "negative amount rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1",
				Amount: -10, Method: psp.MethodCard,
			},
			wantErrIs: psp.ErrInvalidRequest,
		},
		{
			name: "boleto sem CPF rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1", Amount: 99.9,
				Method: psp.MethodBoleto, PayerName: "Ana", PayerEmail: "a@b.com",
			},
			wantErrIs:   psp.ErrInvalidRequest,
			errContains: "boleto requires",
		},
		{
			name: "boleto sem nome rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1", Amount: 99.9,
				Method: psp.MethodBoleto, PayerCPF: "12345678901", PayerEmail: "a@b.com",
			},
			wantErrIs: psp.ErrInvalidRequest,
		},
		{
			name: "boleto sem email rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1", Amount: 99.9,
				Method: psp.MethodBoleto, PayerCPF: "12345678901", PayerName: "Ana",
			},
			wantErrIs: psp.ErrInvalidRequest,
		},
		{
			name: "method desconhecido rejected",
			req: psp.CreateRequest{
				OrderID: "uuid", UserID: "u1", Amount: 99.9,
				Method: psp.PaymentMethod("crypto"),
			},
			wantErrIs:   psp.ErrInvalidRequest,
			errContains: "unsupported method",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := g.CreatePayment(context.Background(), c.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if c.wantErrIs != nil && !errors.Is(err, c.wantErrIs) {
				t.Errorf("expected errors.Is(%v) = true, got err=%v", c.wantErrIs, err)
			}
			if c.errContains != "" && !strings.Contains(err.Error(), c.errContains) {
				t.Errorf("expected error containing %q, got %q", c.errContains, err.Error())
			}
		})
	}
}

func TestName(t *testing.T) {
	g := New("sk_test_dummy", "")
	if g.Name() != "stripe" {
		t.Errorf("Name() = %q, want %q", g.Name(), "stripe")
	}
}

func TestVerifyWebhook_NoSecret_AllowsAll(t *testing.T) {
	// Em dev (webhookSecret=""), VerifyWebhook deve passar mesmo sem assinatura.
	// Comportamento por design — fail-closed entra na Sprint 8.5.
	g := New("sk_test_dummy", "")
	err := g.VerifyWebhook([]byte(`{}`), http.Header{})
	if err != nil {
		t.Errorf("expected nil with empty secret, got %v", err)
	}
}

func TestVerifyWebhook_WithSecret_RejectsMissingSignature(t *testing.T) {
	g := New("sk_test_dummy", "whsec_dummy")
	err := g.VerifyWebhook([]byte(`{}`), http.Header{})
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_WithSecret_RejectsBadSignature(t *testing.T) {
	g := New("sk_test_dummy", "whsec_dummy")
	h := http.Header{}
	h.Set("Stripe-Signature", "t=123,v1=deadbeef")
	err := g.VerifyWebhook([]byte(`{}`), h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestParseWebhookEvent_PaymentIntentSucceeded(t *testing.T) {
	g := New("sk_test_dummy", "")

	body := mustJSON(t, map[string]any{
		"id":   "evt_1",
		"type": "payment_intent.succeeded",
		"data": map[string]any{
			"object": map[string]any{
				"id":     "pi_test_123",
				"status": "succeeded",
				"amount": int64(9990),
			},
		},
	})

	ev, err := g.ParseWebhookEvent(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.EventType != "payment_intent.succeeded" {
		t.Errorf("EventType=%q", ev.EventType)
	}
	if ev.PSPID != "pi_test_123" {
		t.Errorf("PSPID=%q", ev.PSPID)
	}
	if ev.Status != psp.StatusApproved {
		t.Errorf("Status=%q", ev.Status)
	}
	if ev.Amount != 99.90 {
		t.Errorf("Amount=%f, want 99.90", ev.Amount)
	}
}

func TestParseWebhookEvent_IrrelevantTypeSkipped(t *testing.T) {
	g := New("sk_test_dummy", "")

	// Tipo que não é payment_intent — deve retornar (nil, nil)
	body := mustJSON(t, map[string]any{
		"id":   "evt_2",
		"type": "customer.subscription.created",
		"data": map[string]any{"object": map[string]any{}},
	})

	ev, err := g.ParseWebhookEvent(body)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if ev != nil {
		t.Errorf("expected nil event for irrelevant type, got %+v", ev)
	}
}

func TestParseWebhookEvent_PaymentIntentFailed(t *testing.T) {
	g := New("sk_test_dummy", "")

	body := mustJSON(t, map[string]any{
		"id":   "evt_3",
		"type": "payment_intent.payment_failed",
		"data": map[string]any{
			"object": map[string]any{
				"id":     "pi_test_fail",
				"status": "requires_payment_method",
				"amount": int64(5000),
			},
		},
	})

	ev, err := g.ParseWebhookEvent(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.Status != psp.StatusPending {
		// requires_payment_method → pending na nossa normalização
		t.Errorf("Status=%q, want StatusPending", ev.Status)
	}
}

func TestParseWebhookEvent_MalformedJSON(t *testing.T) {
	g := New("sk_test_dummy", "")
	_, err := g.ParseWebhookEvent([]byte("{invalid json"))
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

// -- helpers ----------------------------------------------------------------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
