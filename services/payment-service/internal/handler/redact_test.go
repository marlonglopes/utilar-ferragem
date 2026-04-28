package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactPSPPayload_StripeCardSensitiveFields(t *testing.T) {
	// Stripe raramente retorna PAN raw (sempre tokenizado), mas se vier em
	// algum webhook custom, esses campos têm que ser mascarados.
	in := json.RawMessage(`{
		"id": "pi_123",
		"amount": 1000,
		"payment_method": {
			"card": {
				"brand": "visa",
				"last4": "4242",
				"exp_month": 12,
				"exp_year": 2030,
				"card_number": "4242424242424242",
				"cvc": "123"
			}
		}
	}`)
	out := redactPSPPayload(in)
	s := string(out)
	for _, leaked := range []string{"4242424242424242", `"123"`, `12,`, `2030`} {
		if strings.Contains(s, leaked) {
			t.Errorf("redaction não removeu %q: %s", leaked, s)
		}
	}
	// Last4 deve ficar (não é PCI sensitive)
	if !strings.Contains(s, `"4242"`) {
		t.Error("last4 sumiu — deveria permanecer")
	}
	if !strings.Contains(s, "REDACTED") {
		t.Error("placeholder REDACTED ausente")
	}
}

// Boleto (Stripe): linha digitável fica em next_action.boleto_display_details.number.
// É um valor PÚBLICO usado pra pagar — NÃO deve ser mascarado.
func TestRedactPSPPayload_BoletoNumberPreserved(t *testing.T) {
	in := json.RawMessage(`{
		"id": "pi_boleto",
		"status": "requires_action",
		"next_action": {
			"type": "boleto_display_details",
			"boleto_display_details": {
				"number": "00190.00009 02000.000000 00000.000000 1 12345678901234",
				"pdf": "https://stripe.com/boleto/abc.pdf",
				"hosted_voucher_url": "https://stripe.com/boleto/abc"
			}
		}
	}`)
	out := redactPSPPayload(in)
	s := string(out)
	if !strings.Contains(s, "00190.00009") {
		t.Errorf("boleto number foi mascarado erroneamente: %s", s)
	}
	if !strings.Contains(s, "https://stripe.com/boleto/abc.pdf") {
		t.Error("PDF URL sumiu")
	}
}

func TestRedactPSPPayload_MercadoPagoIdentification(t *testing.T) {
	in := json.RawMessage(`{
		"id": "mp_456",
		"payer": {
			"email": "alice@example.com",
			"identification": {"type": "CPF", "number": "12345678900"}
		},
		"card": {
			"first_six_digits": "424242",
			"last_four_digits": "4242"
		}
	}`)
	out := redactPSPPayload(in)
	s := string(out)
	for _, leaked := range []string{"12345678900", "424242"} {
		if strings.Contains(s, leaked) {
			t.Errorf("redaction vazou %q: %s", leaked, s)
		}
	}
	// Email NÃO está na lista (decisão consciente — pode ser usado em fraud
	// monitoring sem ser PCI). Last4 também fica.
	if !strings.Contains(s, "alice@example.com") {
		t.Error("email não deveria sumir nesta versão da redaction")
	}
	if !strings.Contains(s, `"4242"`) {
		t.Error("last4 sumiu")
	}
}

func TestRedactPSPPayload_PreservaIdsETopLevelMeta(t *testing.T) {
	in := json.RawMessage(`{"id":"pi_x","status":"succeeded","amount":1000,"currency":"brl"}`)
	out := redactPSPPayload(in)
	if string(out) == `null` {
		t.Fatalf("redaction zerou payload limpo: %s", out)
	}
	for _, must := range []string{`"pi_x"`, `"succeeded"`, `1000`, `"brl"`} {
		if !strings.Contains(string(out), must) {
			t.Errorf("redação removeu campo não-PII: faltando %q em %s", must, out)
		}
	}
}

func TestRedactPSPPayload_VazioOuInvalido(t *testing.T) {
	if got := redactPSPPayload(nil); got != nil && len(got) != 0 {
		t.Errorf("vazio não preservado: %s", got)
	}
	bad := json.RawMessage(`{not valid json`)
	if got := redactPSPPayload(bad); string(got) != string(bad) {
		t.Error("payload inválido não foi passado adiante intacto")
	}
}
