package paymentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/utilar/pkg/servicetoken"
)

// Estas duas NÃO são falhas: sinalizam que não há estorno de PSP a fazer, e o
// order-service segue só para o livro contábil.
var (
	// ErrNoPSPPayment — o pedido não tem pagamento PSP confirmado (pago em
	// dinheiro/maquininha/externo, ou dev sem PSP). Nada a estornar no PSP.
	ErrNoPSPPayment = errors.New("paymentclient: no psp payment to refund")
	// ErrRefundUnsupported — o PSP configurado não implementa estorno.
	ErrRefundUnsupported = errors.New("paymentclient: psp refund not supported")
)

// RefundRequest — pedido de ESTORNO REAL no PSP (POST /internal/v1/refunds).
type RefundRequest struct {
	ReturnID    string `json:"returnId"`
	OrderID     string `json:"orderId"`
	AmountCents int64  `json:"amountCents"`
	Total       bool   `json:"total"`
}

// RefundOutcome — resultado do estorno no PSP.
type RefundOutcome struct {
	Status      string // requested | refunded
	PSPRefundID string
	Duplicate   bool
}

// RequestRefund pede ao payment-service o ESTORNO REAL no PSP.
//
// Semântica dos retornos:
//   - (outcome, nil)            → estorno solicitado/feito (ou duplicata idempotente).
//   - (_, ErrNoPSPPayment)      → não há pagamento PSP; siga só para o livro.
//   - (_, ErrRefundUnsupported) → PSP sem estorno; siga só para o livro.
//   - (_, outro erro)           → falha REAL: NÃO marque refunded (dinheiro não saiu).
//
// IDEMPOTENTE do outro lado por returnID (tabela psp_refunds): o payment reserva
// antes de chamar o PSP, então retry não dispara 2º estorno. SEM RETRY AUTOMÁTICO
// aqui — é dinheiro; o retry é decisão humana (chamar o endpoint de novo). Mesmo
// princípio de PostReturnRefund.
func (c *Client) RequestRefund(ctx context.Context, in RefundRequest) (RefundOutcome, error) {
	if c.baseURL == "" || c.serviceSecret == "" {
		return RefundOutcome{}, ErrNotConfigured
	}

	body, err := json.Marshal(in)
	if err != nil {
		return RefundOutcome{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/v1/refunds", bytes.NewReader(body))
	if err != nil {
		return RefundOutcome{}, err
	}
	tok, err := servicetoken.Issue(c.serviceSecret, "order-service")
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok && rid != "" {
		req.Header.Set("X-Request-Id", rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return RefundOutcome{}, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

	var parsed struct {
		Status      string `json:"status"`
		PSPRefundID string `json:"pspRefundId"`
		Duplicate   bool   `json:"duplicate"`
		Code        string `json:"code"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)

	switch resp.StatusCode {
	case http.StatusOK:
		return RefundOutcome{Status: parsed.Status, PSPRefundID: parsed.PSPRefundID, Duplicate: parsed.Duplicate}, nil
	case http.StatusConflict:
		// 409 com code = caso "não aplicável" (siga pro livro), não falha de dinheiro.
		switch parsed.Code {
		case "no_psp_payment":
			return RefundOutcome{}, ErrNoPSPPayment
		case "psp_refund_unsupported":
			return RefundOutcome{}, ErrRefundUnsupported
		}
		return RefundOutcome{}, fmt.Errorf("%w: status=409 %s", ErrRejected, raw)
	default:
		// 502 psp_refund_failed, 400, 5xx… falha real → aborta o estorno.
		return RefundOutcome{}, fmt.Errorf("%w: status=%d %s", ErrUpstream, resp.StatusCode, raw)
	}
}
