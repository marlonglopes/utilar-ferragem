package appmaxv1

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// Gateway.Refund traduz o contrato psp.RefundRequest para a chamada da Appmax
// (POST /v1/orders/refund-request) e normaliza o resultado. Estorno na Appmax é
// ASSÍNCRONO → status "requested" (nunca "refunded" aqui). HTTP mockado.
func TestGatewayRefund(t *testing.T) {
	s := newStub(t)
	var lastBody string
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if r.URL.Path == "/v1/orders/refund-request" {
			lastBody = string(body)
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return false
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}
	ctx := context.Background()

	// TOTAL: type=total, SEM value; status normalizado = requested (assíncrono).
	res, err := g.Refund(ctx, psp.RefundRequest{PSPID: "5", Total: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != psp.RefundRequested {
		t.Fatalf("status = %q, esperado requested", res.Status)
	}
	if !strings.Contains(lastBody, `"type":"total"`) || strings.Contains(lastBody, `"value"`) {
		t.Fatalf("estorno total mal formado (não pode ter value): %s", lastBody)
	}

	// PARCIAL: type=partial + value em CENTAVOS.
	if _, err := g.Refund(ctx, psp.RefundRequest{PSPID: "5", AmountCents: 2500}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastBody, `"type":"partial"`) || !strings.Contains(lastBody, `"value":2500`) {
		t.Fatalf("estorno parcial mal formado: %s", lastBody)
	}

	// pspID não-numérico → ErrInvalidRequest, sem tocar a rede (fail-closed).
	if _, err := g.Refund(ctx, psp.RefundRequest{PSPID: "abc", Total: true}); !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("pspID inválido deveria falhar com ErrInvalidRequest: %v", err)
	}
}
