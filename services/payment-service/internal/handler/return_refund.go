package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/ledger"
)

// ============================================================================
// Estorno de devolução — lançamento contábil
// ----------------------------------------------------------------------------
// A DECISÃO de estornar (quem aprovou, sobre qual pedido, quais itens, se está
// dentro do prazo do CDC) é do ORDER-SERVICE, que é quem tem o pedido, os itens
// com preço snapshot, o prazo legal e a trilha de auditoria da devolução. Este
// endpoint é só o braço contábil: recebe um fato já autorizado e auditado e o
// transforma em partida dobrada.
//
// Mesmo desenho de external_settlement.go, e pelo mesmo motivo: o livro é único
// e vive aqui. Dois serviços escrevendo no mesmo livro é como se perde a
// garantia de que tudo soma zero.
//
// ⚠️ ISTO NÃO ESTORNA NO PSP. Este handler só lança no livro. Devolver o
// dinheiro de verdade ao cartão/Pix do cliente é uma chamada ao gateway e ainda
// não existe — ver docs/devolucao-e-troca.md § "O que falta do PSP real".
// ============================================================================

const maxReturnRefundBody = 8 * 1024

// ReturnRefundRequest é o contrato de POST /internal/v1/ledger/return-refund.
//
// Todo campo de identidade (ApprovedBy) é DADO, não autorização: a autorização
// já aconteceu no order-service. Existe para que o lançamento carregue no livro
// o mesmo rastro que a trilha da devolução carrega.
type ReturnRefundRequest struct {
	// ReturnID é a CHAVE DE IDEMPOTÊNCIA.
	//
	// ⚠️ PORQUÊ não o paymentID: um pedido pode ter VÁRIAS devoluções parciais
	// (comprou 10, devolveu 1 hoje e 1 amanhã). O construtor ledger.Refund
	// chaveia por paymentID + "total"/"parcial", o que faria a segunda
	// devolução parcial ser tratada como duplicata da primeira e o lançamento
	// sumir — despesa subestimada, e o contador sem como saber. Por isso este
	// handler monta o TxInput com source_id = returnID em vez de usar aquele
	// construtor.
	ReturnID  string `json:"returnId" binding:"required,uuid"`
	OrderID   string `json:"orderId" binding:"required,uuid"`
	PaymentID string `json:"paymentId" binding:"omitempty,max=64"`

	AmountBRL   float64 `json:"amount" binding:"required,gt=0"`
	ShippingBRL float64 `json:"shippingAmount" binding:"omitempty,gte=0"`
	Method      string  `json:"method" binding:"omitempty,max=16"`
	Partial     bool    `json:"partial"`

	ApprovedBy string     `json:"approvedBy" binding:"required,max=64"`
	OccurredAt *time.Time `json:"occurredAt"`
}

// ReturnRefundHandler lança estornos de devolução no livro.
type ReturnRefundHandler struct {
	poster *ledger.Poster
}

func NewReturnRefundHandler(p *ledger.Poster) *ReturnRefundHandler {
	return &ReturnRefundHandler{poster: p}
}

// Post POST /internal/v1/ledger/return-refund
//
// Idempotente por DEVOLUÇÃO: a chave (kind=refund, source_type=order,
// source_id=returnID) é UNIQUE no banco. Uma segunda chamada — retry de rede,
// atendente clicando duas vezes, replay do order-service — devolve 200 com
// `duplicate: true` e NÃO gera um segundo lançamento. Responder 409 seria pior:
// o chamador trataria como falha e tentaria para sempre, quando o fato já está
// no livro.
func (h *ReturnRefundHandler) Post(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReturnRefundBody)

	var req ReturnRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	occurred := time.Now().UTC()
	if req.OccurredAt != nil && !req.OccurredAt.IsZero() {
		occurred = req.OccurredAt.UTC()
	}

	// Itens + frete vão num lançamento só, porque saíram do caixa juntos.
	// Postings separados para que o contador veja a composição.
	itens := ledger.Cents(centsOf(req.AmountBRL))
	frete := ledger.Cents(centsOf(req.ShippingBRL))
	total := itens + frete

	tipo := "total"
	if req.Partial {
		tipo = "parcial"
	}

	// D 3.1.8 Estornos / C 1.1.1 Caixa — mesma estrutura de ledger.Refund.
	//
	// Conta REDUTORA de receita, não estorno da 3.1.1: a venda aconteceu de
	// fato e o contador precisa ver bruto e devolução separados (relevante
	// inclusive para apuração de imposto sobre vendas canceladas).
	postings := []ledger.Posting{
		{Account: ledger.AcctEstornos, Side: ledger.Debit, Amount: itens,
			PaymentMethod: req.Method, Memo: "devolução " + tipo + " — mercadoria"},
		{Account: ledger.AcctCaixaPSP, Side: ledger.Credit, Amount: total,
			PaymentMethod: req.Method},
	}
	if frete > 0 {
		// Frete devolvido no arrependimento (CDC art. 49, parágrafo único).
		// Posting próprio para não se misturar ao valor da mercadoria.
		postings = append(postings, ledger.Posting{
			Account: ledger.AcctEstornos, Side: ledger.Debit, Amount: frete,
			PaymentMethod: req.Method, Memo: "devolução " + tipo + " — frete ressarcido",
		})
	}

	tx, err := h.poster.Post(c.Request.Context(), ledger.TxInput{
		OccurredAt: occurred,
		Kind:       ledger.KindRefund,
		// SourceOrder + returnID: único por devolução, e distinto da chave que
		// ledger.Refund usa para estornos vindos de webhook do PSP — os dois
		// caminhos podem coexistir sem colidir.
		SourceType:  ledger.SourceOrder,
		SourceID:    req.ReturnID,
		Description: "Estorno " + tipo + " de devolução do pedido " + req.OrderID,
		RequestID:   c.GetString("request_id"),
		// CreatedBy é quem autorizou. Estorno sem rastro até a PESSOA é
		// exatamente o registro que não pode faltar.
		CreatedBy: req.ApprovedBy,
		Postings:  postings,
	})

	switch {
	case err == nil:
		slog.Info("estorno de devolução lançado no livro",
			"return_id", req.ReturnID, "order_id", req.OrderID, "tx_id", tx.ID,
			"amount_cents", int64(total), "partial", req.Partial,
			"approved_by", req.ApprovedBy, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusCreated, gin.H{
			"transactionId": tx.ID, "period": tx.Period,
			"totalCents": tx.TotalCents, "duplicate": false,
		})

	case errors.Is(err, ledger.ErrDuplicate):
		// Caso NORMAL, não erro: esta devolução já foi lançada.
		slog.Info("estorno de devolução já lançado (idempotência)",
			"return_id", req.ReturnID, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusOK, gin.H{"returnId": req.ReturnID, "duplicate": true})

	case errors.Is(err, ledger.ErrPeriodClosed):
		Respond(c, http.StatusConflict, "period_closed", err.Error())

	case errors.Is(err, ledger.ErrInvalidInput), errors.Is(err, ledger.ErrUnbalanced):
		BadRequest(c, err.Error())

	default:
		slog.Error("estorno de devolução: falha ao lançar no livro",
			"error", err, "return_id", req.ReturnID,
			"request_id", c.GetString("request_id"))
		InternalError(c, "could not post return refund")
	}
}
