package paymentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/utilar/pkg/servicetoken"
)

// ============================================================================
// Estorno de devolução → livro contábil
// ----------------------------------------------------------------------------
// ⚠️ ESTORNO É DINHEIRO SAINDO. A decisão (quem aprovou, sobre qual pedido,
// quais itens, quanto) é do order-service, que é quem tem o pedido, os itens
// com preço snapshot, o prazo legal e a trilha de auditoria da devolução. O
// LANÇAMENTO cai no payment-service, onde o livro vive — exatamente o mesmo
// desenho da liquidação externa (ver client.go), e pelo mesmo motivo: dois
// serviços escrevendo o próprio livro é como se perde a garantia de que tudo
// soma zero.
//
// ⚠️ ISTO NÃO É O ESTORNO NO PSP. Este endpoint só faz o lançamento CONTÁBIL.
// Devolver o dinheiro de verdade ao cartão/Pix do cliente é uma chamada ao
// gateway, que mora no payment-service e ainda não está implementada — ver
// docs/devolucao-e-troca.md § "O que falta do PSP real".
// ============================================================================

// ReturnRefund é o fato a lançar: dinheiro devolvido ao comprador.
type ReturnRefund struct {
	// ReturnID é a CHAVE DE IDEMPOTÊNCIA do lançamento.
	//
	// PORQUÊ o id da devolução e não o do pagamento: um pedido pode ter várias
	// devoluções parciais (comprou 10, devolveu 1 hoje e 1 amanhã). Chavear
	// pelo pagamento faria a segunda devolução ser tratada como duplicata da
	// primeira e o lançamento sumir — receita superestimada, e o contador não
	// teria como saber.
	ReturnID string `json:"returnId" binding:"required"`
	OrderID  string `json:"orderId"`
	// PaymentID é informativo (aparece na descrição do lançamento). Pode vir
	// vazio: venda de balcão liquidada na maquininha não tem pagamento de PSP.
	PaymentID string `json:"paymentId,omitempty"`

	AmountBRL   float64 `json:"amount"`
	ShippingBRL float64 `json:"shippingAmount,omitempty"`
	// Method é o meio pelo qual o dinheiro entrou (pix/boleto/card/external).
	// Entra como label do posting: o relatório por método precisa mostrar o
	// estorno na mesma linha da venda que ele desfaz.
	Method string `json:"method"`

	// Partial diz se a devolução foi parcial. Vai para a descrição e para o
	// relatório; a Appmax trata split + parcial de forma diferente.
	Partial bool `json:"partial"`

	// ApprovedBy é quem autorizou. Vai para created_by do lançamento: estorno
	// sem rastro até a PESSOA é exatamente o registro que não pode faltar.
	ApprovedBy string    `json:"approvedBy"`
	OccurredAt time.Time `json:"occurredAt"`
}

// PostReturnRefund lança o estorno da devolução no livro contábil.
//
// IDEMPOTENTE do outro lado, pela chave (kind=refund, source_type=order,
// source_id=returnID) com UNIQUE no banco do payment-service. Chamar duas vezes
// com a mesma devolução devolve 200 e nenhum lançamento novo — o que torna
// seguro RETENTAR, e é por isso que o handler pode reprocessar uma devolução
// cujo lançamento falhou na primeira vez.
//
// ⚠️ SEM RETRY AUTOMÁTICO AQUI, e é deliberado. Esta é uma operação de
// DINHEIRO. Mesmo sendo idempotente do outro lado, a idempotência depende de
// uma garantia que vive noutro serviço, noutro banco — e o custo de estar
// errado sobre ela é lançar o mesmo estorno duas vezes no livro. O retry é
// explícito, feito pelo operador chamando o endpoint de novo, com a decisão
// humana no meio. Mesmo princípio de appmaxv1.isFinancialRoute e de
// pkg/retry.NonIdempotent.
func (c *Client) PostReturnRefund(ctx context.Context, in ReturnRefund) error {
	if c.baseURL == "" || c.serviceSecret == "" {
		return ErrNotConfigured
	}

	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/v1/ledger/return-refund", bytes.NewReader(body))
	if err != nil {
		return err
	}
	tok, err := servicetoken.Issue(c.serviceSecret, "order-service")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok && rid != "" {
		req.Header.Set("X-Request-Id", rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	// Corpo lido e descartado com limite: sem isso a conexão não volta ao pool
	// em keep-alive. Mesmo motivo de PostExternalSettlement.
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		// 200 = já existia (idempotência), 201 = lançado agora. Os dois são
		// sucesso: o fato está no livro exatamente uma vez.
		return nil
	case resp.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrPeriodClosed, snippet)
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return fmt.Errorf("%w: status=%d %s", ErrRejected, resp.StatusCode, snippet)
	default:
		return fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	}
}
