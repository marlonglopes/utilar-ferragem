package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/ledger"
)

// ============================================================================
// Liquidação externa — lançamento contábil da venda paga na maquininha da loja
// ----------------------------------------------------------------------------
// A DECISÃO de liquidar (quem pode, em que pedido, com qual NSU) é do
// order-service, que é quem tem o pedido, o vínculo do operador com a loja e a
// trilha de auditoria do balcão. Este endpoint é só o braço contábil: recebe um
// fato já autorizado e auditado e o transforma em partida dobrada.
//
// PORQUÊ o lançamento não mora no order-service: o livro é único e vive aqui.
// Dois serviços escrevendo no mesmo livro é como se perde a garantia de que
// tudo soma zero.
// ============================================================================

// maxExternalSettlementBody — payload é ~500 bytes; 8KB é folga sem abrir DoS.
const maxExternalSettlementBody = 8 * 1024

// ExternalSettlementRequest é o contrato de POST /internal/v1/ledger/external-settlement.
//
// Todo campo de identidade (SettledBy, StoreID, IP) é DADO, não autorização: a
// autorização já aconteceu no order-service. Eles existem para que o lançamento
// carregue no livro o mesmo rastro que a trilha do balcão carrega — quem
// liquidou, de qual loja, de onde.
type ExternalSettlementRequest struct {
	OrderID    string  `json:"orderId" binding:"required,uuid"`
	AmountBRL  float64 `json:"amount" binding:"required,gt=0"`
	NSU        string  `json:"nsu" binding:"required,min=4,max=32"`
	StoreID    string  `json:"storeId" binding:"required,max=64"`
	OperatorID string  `json:"operatorId" binding:"max=64"`
	// SettledBy é o usuário que apertou o botão. Vai para created_by do
	// lançamento: liquidação externa sem rastro até a pessoa é exatamente o
	// registro que não pode faltar.
	SettledBy     string `json:"settledBy" binding:"required,max=64"`
	Brand         string `json:"brand" binding:"max=32"`
	Authorization string `json:"authorizationCode" binding:"max=32"`
	// OccurredAt é a hora do comprovante do adquirente, não a hora em que este
	// serviço processou. Datar pelo processamento jogaria a venda de 23h58 no
	// dia seguinte e faria o fechamento do dia não bater com o extrato.
	OccurredAt *time.Time `json:"occurredAt"`
}

// ExternalSettlementHandler lança vendas liquidadas fora do PSP.
type ExternalSettlementHandler struct {
	poster *ledger.Poster
}

func NewExternalSettlementHandler(p *ledger.Poster) *ExternalSettlementHandler {
	return &ExternalSettlementHandler{poster: p}
}

// Post POST /internal/v1/ledger/external-settlement
//
// Idempotente por pedido: a chave (kind=external_sale, source_type=
// external_settlement, source_id=orderID) é UNIQUE no banco. Uma segunda
// chamada — retry de rede, operador clicando duas vezes, replay do
// order-service — devolve 200 com `duplicate: true` e NÃO gera um segundo
// lançamento. Responder 409 aqui seria pior: o chamador trataria como falha e
// tentaria de novo para sempre, quando na verdade o fato já está no livro.
func (h *ExternalSettlementHandler) Post(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxExternalSettlementBody)

	var req ExternalSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	nsu := strings.TrimSpace(req.NSU)
	if nsu == "" {
		BadRequest(c, "nsu é obrigatório: é o que amarra a venda ao extrato do adquirente")
		return
	}

	occurred := time.Now().UTC()
	if req.OccurredAt != nil && !req.OccurredAt.IsZero() {
		occurred = req.OccurredAt.UTC()
	}

	tx, err := h.poster.Post(c.Request.Context(), ledger.ExternalSale(ledger.ExternalSaleInput{
		OrderID:       req.OrderID,
		NSU:           nsu,
		StoreID:       req.StoreID,
		OperatorID:    req.OperatorID,
		Brand:         req.Brand,
		Authorization: req.Authorization,
		OccurredAt:    occurred,
		GrossCents:    ledger.Cents(centsOf(req.AmountBRL)),
		RequestID:     c.GetString("request_id"),
		SettledBy:     req.SettledBy,
	}))

	switch {
	case err == nil:
		slog.Info("liquidação externa lançada no livro",
			"order_id", req.OrderID, "tx_id", tx.ID, "nsu", nsu,
			"store_id", req.StoreID, "settled_by", req.SettledBy,
			"request_id", c.GetString("request_id"))
		c.JSON(http.StatusCreated, gin.H{
			"transactionId": tx.ID, "period": tx.Period,
			"totalCents": tx.TotalCents, "duplicate": false,
		})

	case errors.Is(err, ledger.ErrDuplicate):
		// Caso NORMAL, não erro: o pedido já foi liquidado e lançado.
		slog.Info("liquidação externa já lançada (idempotência)",
			"order_id", req.OrderID, "nsu", nsu,
			"request_id", c.GetString("request_id"))
		c.JSON(http.StatusOK, gin.H{"orderId": req.OrderID, "duplicate": true})

	case errors.Is(err, ledger.ErrPeriodClosed):
		// Mês fechado: o contador já entregou o balanço. Lançar retroativo
		// furaria o fechamento. Vira caso de ajuste manual com justificativa.
		Respond(c, http.StatusConflict, "period_closed", err.Error())

	case errors.Is(err, ledger.ErrInvalidInput), errors.Is(err, ledger.ErrUnbalanced):
		BadRequest(c, err.Error())

	default:
		slog.Error("liquidação externa: falha ao lançar no livro",
			"error", err, "order_id", req.OrderID,
			"request_id", c.GetString("request_id"))
		InternalError(c, "could not post external settlement")
	}
}
