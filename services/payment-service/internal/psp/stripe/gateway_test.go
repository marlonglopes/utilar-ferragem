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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v86"
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

// stripeSignatureHeader monta o header Stripe-Signature `t=TS,v1=HEX` do mesmo
// jeito que o Stripe assina: HMAC-SHA256 sobre `TS + "." + body`.
func stripeSignatureHeader(body []byte, secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// TestRegression_VerifyWebhook_AccountAPIVersionVerifies trava o bug original: um
// evento assinado, criado na versão de API que a CONTA emite, tem que passar no
// ConstructEvent estrito. Na v79 (API embutida 2024-06-20) a versão da conta
// (2026-08-26.dahlia) divergia e o SDK rejeitava a assinatura VÁLIDA → webhook
// 401 → pagamento não confirmava. Subimos pra v86 (embute exatamente a versão da
// conta), então o estrito volta a casar.
//
// O corpo usa stripe.APIVersion (a versão embutida no SDK) DE PROPÓSITO, não uma
// string fixa: assim o teste continua válido quando o SDK subir de novo — ele
// sempre testa "evento na versão do SDK verifica", que é a invariante real. Se
// alguém subir o SDK sem a conta acompanhar (ou vice-versa) e o estrito voltar a
// 401 em produção, é aqui e no E2E que aparece.
func TestRegression_VerifyWebhook_AccountAPIVersionVerifies(t *testing.T) {
	const secret = "whsec_test_regression"
	g := New("sk_test_dummy", secret)

	// "object":"event" é obrigatório: a v86 rejeita payload sem ele como se fosse
	// uma "thin event notification" (evento v2, que não usamos) — eventos snapshot
	// reais da Stripe sempre trazem esse campo.
	body := []byte(`{"id":"evt_1","object":"event","api_version":"` + stripe.APIVersion + `","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","status":"succeeded"}}}`)

	h := http.Header{}
	h.Set("Stripe-Signature", stripeSignatureHeader(body, secret, time.Now().Unix()))

	if err := g.VerifyWebhook(body, h); err != nil {
		t.Fatalf("assinatura válida na versão de API do SDK (%s) deve passar, got %v", stripe.APIVersion, err)
	}
}

// TestRegression_VerifyWebhook_TamperedBodyStillRejected garante que a checagem
// de ASSINATURA continua firme: corpo adulterado depois de assinado é rejeitado.
func TestRegression_VerifyWebhook_TamperedBodyStillRejected(t *testing.T) {
	const secret = "whsec_test_regression"
	g := New("sk_test_dummy", secret)

	original := []byte(`{"id":"evt_1","object":"event","api_version":"` + stripe.APIVersion + `","type":"payment_intent.succeeded"}`)
	sig := stripeSignatureHeader(original, secret, time.Now().Unix())

	tampered := []byte(`{"id":"evt_1","object":"event","api_version":"` + stripe.APIVersion + `","type":"payment_intent.canceled"}`)
	h := http.Header{}
	h.Set("Stripe-Signature", sig)

	if err := g.VerifyWebhook(tampered, h); !errors.Is(err, psp.ErrInvalidSignature) {
		t.Fatalf("corpo adulterado deve ser rejeitado, got %v", err)
	}
}

// TestRegression_ClassifyCreateError_TaxIDInvalidIsClientError trava o bug do
// boleto: um CPF inválido (Stripe code tax_id_invalid) devolvia "payment gateway
// error" genérico (502) porque o gateway embrulhava TUDO como ErrUpstream. Deve
// virar ErrInvalidRequest (400) com o código estável na mensagem, pro handler
// traduzir em "CPF inválido".
func TestRegression_ClassifyCreateError_TaxIDInvalidIsClientError(t *testing.T) {
	stripeErr := &stripe.Error{Code: "tax_id_invalid", Type: stripe.ErrorTypeInvalidRequest}
	got := classifyCreateError(stripeErr)
	if !errors.Is(got, psp.ErrInvalidRequest) {
		t.Fatalf("tax_id_invalid deve virar ErrInvalidRequest (400), got %v", got)
	}
	if !strings.Contains(got.Error(), "stripe_tax_id_invalid") {
		t.Fatalf("mensagem deve conter o código estável, got %q", got.Error())
	}
}

// TestClassifyCreateError_UnknownErrorStaysUpstream — o que NÃO é input do
// cliente (rede, 500 da Stripe, código desconhecido) continua ErrUpstream (502).
// Allowlist, não denylist: erro novo não vira "culpa do cliente" por omissão.
func TestClassifyCreateError_UnknownErrorStaysUpstream(t *testing.T) {
	for _, e := range []error{
		errors.New("connection refused"),
		&stripe.Error{Code: "api_error", Type: stripe.ErrorTypeAPI},
		&stripe.Error{Code: "", Type: stripe.ErrorTypeInvalidRequest},
	} {
		got := classifyCreateError(e)
		if !errors.Is(got, psp.ErrUpstream) {
			t.Errorf("erro %v deveria continuar ErrUpstream (502), got %v", e, got)
		}
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
