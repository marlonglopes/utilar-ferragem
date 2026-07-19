package handler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/balcao"
	"github.com/utilar/order-service/internal/fulfillment"
	"github.com/utilar/order-service/internal/model"
	"github.com/utilar/order-service/internal/paymentclient"
)

// ============================================================================
// Liquidação externa — a venda de balcão paga na MAQUININHA DA LOJA
// ----------------------------------------------------------------------------
// POST /api/v1/balcao/orders/:id/settle-external
//
// PORQUÊ um endpoint no order-service e não `method=external` no fluxo de
// pagamento do payment-service:
//
//  1. Não existe pagamento. O fluxo do payment-service inteiro (criar cobrança
//     no PSP, guardar psp_payment_id, esperar webhook, conciliar contra o
//     extrato da Appmax) pressupõe uma transação de gateway. Aqui não há
//     nenhuma. Um `method=external` naquele caminho criaria uma linha em
//     `payments` sem psp_payment_id, que a conciliação teria que aprender a
//     ignorar — mais um caso especial no lugar mais sensível do sistema.
//
//  2. O POST /payments é uma rota de CLIENTE, escopada por user_id. Aceitar
//     `external` ali significaria que o comprador da loja online consegue
//     chamar o endpoint que declara o próprio pedido pago. Não há como
//     defender isso com validação de payload: a rota inteira está do lado
//     errado da fronteira de confiança.
//
//  3. Tudo que esta operação precisa já mora aqui: o pedido, o vínculo do
//     operador com a loja (internal/balcao), a máquina de estados
//     (fulfillment.Advance), a trilha de auditoria do balcão e a baixa de
//     estoque. Fazer no payment-service exigiria replicar os quatro.
//
// O lançamento contábil continua no payment-service, onde o livro vive — via
// rota /internal com token de SERVIÇO (ver internal/paymentclient).
// ============================================================================

// LedgerPoster é o contrato mínimo pro handler lançar a liquidação no livro.
// Interface pequena para o handler ser testável com stub — e para que um deploy
// sem payment-service configurado degrade de forma explícita, não silenciosa.
type LedgerPoster interface {
	PostExternalSettlement(ctx context.Context, in paymentclient.ExternalSettlement) error
}

// WithLedger liga o lançamento contábil da liquidação externa.
//
// Sem ele a liquidação AINDA acontece (o cliente está no caixa com a mercadoria
// na mão; travar a venda porque o serviço contábil está fora seria pior), mas
// cada liquidação grava uma linha de auditoria `payment.ledger_post_failed`
// para que a reconciliação saiba exatamente o que replicar. Ver settleExternal.
func (h *OrderHandler) WithLedger(l LedgerPoster) *OrderHandler {
	h.ledger = l
	return h
}

// SettleExternal POST /api/v1/balcao/orders/:id/settle-external
//
// Este é o endpoint mais perigoso do PDV: ele declara um pedido PAGO sem que
// dinheiro nenhum tenha entrado no nosso sistema. Não existe webhook, não
// existe assinatura de PSP, não existe nada que possa ser verificado depois a
// não ser o NSU do comprovante e o rastro de quem apertou o botão. As três
// defesas, nesta ordem:
//
//  1. QUEM  — balcao.CanSettleExternal (função pura, testes de regressão que
//     provam que customer e anônimo são recusados).
//  2. RASTRO— auditoria FAIL-CLOSED na mesma transação: se a trilha não grava,
//     o pedido não é liquidado.
//  3. UMA VEZ — idempotência por NSU + UNIQUE no banco + chave de idempotência
//     do lançamento contábil.
func (h *OrderHandler) SettleExternal(c *gin.Context) {
	orderID := c.Param("id")
	requestID := c.GetString("request_id")

	var req model.SettleExternalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// NSU normalizado ANTES de tudo: é ele que entra na comparação de
	// idempotência, e "0044-17" e "004417" são o mesmo comprovante.
	nsu, err := balcao.NormalizeNSU(req.NSU)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	brand, err := balcao.NormalizeBrand(req.Brand)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	// O ator vem do token + consulta ao auth-service. Fail-closed: vínculo
	// revogado zera a loja, e sem loja não há liquidação.
	actor := h.actorFromContext(c)

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	// FOR UPDATE: sem o lock, dois toques no tablet (ou o operador e o gerente
	// ao mesmo tempo) leriam os dois `external_nsu IS NULL`, os dois passariam
	// pela idempotência e o pedido acumularia duas liquidações — duas linhas na
	// trilha e, pior, duas tentativas de lançamento contábil.
	var ref balcao.OrderRef
	var channel, status string
	var storeID, operatorID, existingNSU sql.NullString
	var total float64
	var stockReserved bool
	err = tx.QueryRow(`
		SELECT channel::text, status::text, approval_status::text, store_id, operator_id,
		       user_id, total, stock_reserved, external_nsu
		FROM orders WHERE id = $1 FOR UPDATE
	`, orderID).Scan(&channel, &status, &ref.ApprovalStatus, &storeID, &operatorID,
		&ref.OwnerUserID, &total, &stockReserved, &existingNSU)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	ref.Channel = channel
	ref.StoreID, ref.OperatorID = storeID.String, operatorID.String

	// DEFESA 1 — QUEM. Antes de qualquer escrita e antes de revelar qualquer
	// coisa sobre o pedido.
	if err := balcao.CanSettleExternal(actor, ref); err != nil {
		respondSettleError(c, err)
		return
	}

	// DEFESA 3 — UMA VEZ. Mesmo NSU é retry: devolve o pedido como está, sem
	// segunda linha de auditoria e sem segundo lançamento. NSU diferente no
	// mesmo pedido são dois comprovantes para uma venda só (possível cobrança
	// em duplicidade no cartão do cliente) — recusa e alguém olha.
	settled, err := balcao.CheckSettlementIdempotency(existingNSU.String, nsu)
	if err != nil {
		slog.Error("liquidação externa: NSU divergente no mesmo pedido",
			"order_id", orderID, "gravado", existingNSU.String, "recebido", nsu,
			"user_id", actor.UserID, "request_id", requestID)
		respondSettleError(c, err)
		return
	}
	if settled {
		// Retry idempotente. O lançamento contábil é retentado (ele também é
		// idempotente do outro lado), justamente para dar ao operador um jeito
		// de recuperar a liquidação cujo lançamento falhou na primeira vez.
		_ = tx.Rollback()
		h.postExternalToLedger(c, orderID, nsu, brand, req.AuthorizationCode,
			ref.StoreID, ref.OperatorID, actor.UserID, total)
		h.respondOrder(c, orderID)
		return
	}

	// Estado: pending_payment → paid, com o lock e a validação da máquina de
	// estados que todo o resto do serviço já usa. Um pedido já pago, cancelado
	// ou entregue vira 409 aqui — não existe caminho de liquidação que atropele
	// a máquina de estados.
	info := "Maquininha da loja · NSU " + nsu
	if _, err := fulfillment.Advance(tx, orderID, model.StatusPaid, fulfillment.Options{
		Description: "Pagamento recebido na maquininha da loja (NSU " + nsu + ").",
		PaymentInfo: &info,
	}); err != nil {
		var invalid model.ErrInvalidTransition
		switch {
		case errors.Is(err, fulfillment.ErrOrderNotFound):
			NotFound(c, "order not found")
		case errors.As(err, &invalid):
			Conflict(c, err.Error())
		default:
			DBError(c, err)
		}
		return
	}

	// O método vira `external` AQUI, e não na criação do pedido: até o
	// comprovante existir, a venda não foi paga por maquininha nenhuma. É esta
	// linha que conserta o bug original — a venda deixa de ser gravada como
	// `card` e a conciliação com a Appmax para de acusar divergência.
	settledAt := time.Now().UTC()
	if _, err := tx.Exec(`
		UPDATE orders
		   SET payment_method = 'external', external_nsu = $2, external_brand = $3,
		       external_auth_code = $4, external_settled_by = $5, external_settled_at = $6
		 WHERE id = $1
	`, orderID, nsu, nullIfEmpty(brand), nullIfEmpty(req.AuthorizationCode),
		actor.UserID, settledAt); err != nil {
		// O UNIQUE parcial de external_nsu bate aqui: o mesmo comprovante em
		// outro pedido significa uma venda cobrada uma vez e baixada duas.
		if isUniqueViolation(err) {
			Conflict(c, "este NSU já foi usado para liquidar outro pedido")
			return
		}
		DBError(c, err)
		return
	}

	// DEFESA 2 — RASTRO, na MESMA transação e FAIL-CLOSED (auditTx propaga o
	// erro). Liquidação externa sem rastro até a pessoa é exatamente o registro
	// que não pode faltar: é dinheiro declarado como recebido sem nenhuma prova
	// externa. Se a trilha não grava, a liquidação não acontece.
	if err := auditTx(tx, c, balcaoEvent{
		OrderID:  &orderID,
		Action:   "payment.settled_external",
		StoreID:  &ref.StoreID,
		Amount:   &total,
		OldValue: map[string]any{"status": status, "paymentMethod": "(pendente)"},
		NewValue: map[string]any{
			"status":            string(model.StatusPaid),
			"paymentMethod":     string(model.MethodExternal),
			"nsu":               nsu,
			"brand":             brand,
			"authorizationCode": req.AuthorizationCode,
			"settledBy":         actor.UserID,
			"settledAt":         settledAt,
			"soldBy":            ref.OperatorID,
			"note":              req.Note,
		},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("venda de balcão liquidada externamente",
		"order_id", orderID, "nsu", nsu, "store_id", ref.StoreID,
		"settled_by", actor.UserID, "sold_by", ref.OperatorID,
		"amount", total, "ip", c.ClientIP(), "request_id", requestID)

	// Lançamento contábil e baixa de estoque acontecem FORA da transação: são
	// HTTP para outros serviços. Falhar neles não desfaz a liquidação — o
	// cliente pagou e levou a mercadoria; o pedido é dele.
	h.postExternalToLedger(c, orderID, nsu, brand, req.AuthorizationCode,
		ref.StoreID, ref.OperatorID, actor.UserID, total)
	h.commitExternalStock(orderID, stockReserved, requestID)

	h.respondOrder(c, orderID)
}

// postExternalToLedger lança a venda no livro do payment-service.
//
// PORQUÊ best-effort e não parte da transação: o livro vive noutro banco, em
// outro serviço. Uma transação distribuída aqui significaria segurar o lock do
// pedido durante uma chamada HTTP, com o cliente parado no caixa.
//
// A ordem escolhida é "liquida primeiro, lança depois", e a assimetria é
// deliberada: se o lançamento falhar, temos um pedido pago sem lançamento —
// receita SUBESTIMADA, detectável e replicável (a chamada é idempotente pelo
// pedido). A ordem inversa produziria receita no livro para um pedido que não
// foi liquidado: dinheiro inventado, que é o erro que não se aceita.
//
// A falha grava uma linha de auditoria própria — o lançamento pode ser
// reprocessado depois chamando o mesmo endpoint com o mesmo NSU.
func (h *OrderHandler) postExternalToLedger(c *gin.Context, orderID, nsu, brand, authCode,
	storeID, operatorID, settledBy string, total float64) {

	requestID := c.GetString("request_id")
	if h.ledger == nil {
		slog.Error("liquidação externa NÃO lançada no livro: payment-service não configurado "+
			"(PAYMENT_SERVICE_URL/SERVICE_JWT_SECRET) — receita do período ficará subestimada",
			"order_id", orderID, "nsu", nsu, "request_id", requestID)
		h.auditLedgerFailure(c, orderID, storeID, nsu, total, "payment-service não configurado")
		return
	}

	ctx, cancel := context.WithTimeout(paymentclient.WithRequestID(context.Background(), requestID),
		6*time.Second)
	defer cancel()

	err := h.ledger.PostExternalSettlement(ctx, paymentclient.ExternalSettlement{
		OrderID: orderID, AmountBRL: total, NSU: nsu, StoreID: storeID,
		OperatorID: operatorID, SettledBy: settledBy, Brand: brand,
		AuthorizationCode: authCode, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		slog.Error("liquidação externa: lançamento contábil falhou",
			"error", err, "order_id", orderID, "nsu", nsu, "request_id", requestID)
		h.auditLedgerFailure(c, orderID, storeID, nsu, total, err.Error())
	}
}

// auditLedgerFailure registra na trilha que o livro NÃO recebeu o lançamento.
//
// Fora da transação do pedido (que já foi commitada), então aqui a auditoria
// falha ABERTA — ao contrário da trilha da liquidação em si. A diferença é
// proposital: recusar a resposta agora não desfaria a liquidação já gravada,
// só esconderia do operador que ela aconteceu.
func (h *OrderHandler) auditLedgerFailure(c *gin.Context, orderID, storeID, nsu string,
	total float64, reason string) {

	_, err := h.db.Exec(`
		INSERT INTO balcao_audit_events
			(order_id, action, actor_id, actor_role, store_id, new_value, amount, ip, request_id)
		VALUES ($1,'payment.ledger_post_failed',$2,$3,$4,$5,$6,$7,$8)
	`, orderID, c.GetString("user_id"), c.GetString("user_role"), storeID,
		jsonOrNil(map[string]any{"nsu": nsu, "reason": reason, "amount": total}),
		total, c.ClientIP(), c.GetString("request_id"))
	if err != nil {
		slog.Error("liquidação externa: falha ao auditar a falha do lançamento contábil",
			"error", err, "order_id", orderID, "request_id", c.GetString("request_id"))
	}
}

// commitExternalStock transforma a RESERVA em baixa definitiva.
//
// O fluxo pago normal faz isso no consumer de pagamento (payment.confirmed →
// stock.Commit). A liquidação externa não passa por Kafka nenhum — não há
// evento de PSP —, então a baixa precisa acontecer aqui. Sem isto, a mercadoria
// sairia da loja com a reserva ainda pendurada, e o sweeper de expiração a
// devolveria ao estoque em 30 minutos: o sistema passaria a acreditar que tem
// um item que já foi embora na sacola do cliente.
func (h *OrderHandler) commitExternalStock(orderID string, reserved bool, requestID string) {
	if h.stock == nil || !reserved {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.stock.Commit(ctx, orderID); err != nil {
		slog.Error("liquidação externa: baixa de estoque falhou",
			"error", err, "order_id", orderID, "request_id", requestID)
	}
}

// respondOrder devolve o pedido atualizado. Sem filtro de dono: a autorização
// já aconteceu, e filtrar por user_id devolveria 500 numa venda de balcão cujo
// dono é o operador que vendeu, não quem liquidou.
func (h *OrderHandler) respondOrder(c *gin.Context, orderID string) {
	order, err := h.loadOrder(orderID, "")
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// respondSettleError traduz os sentinelas da liquidação em HTTP.
//
// Reaproveita respondSaleError para os erros de escopo de loja (que viram 404
// por anti-enumeração) e trata os específicos da liquidação.
func respondSettleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, balcao.ErrNotSettler):
		// Mensagem genérica de propósito: não confirma sequer que o pedido
		// existe. Quem tenta liquidar sem ser operador não recebe nenhum dado.
		Forbidden(c, "store operator role required to settle payments externally")
	case errors.Is(err, balcao.ErrNotBalcaoOrder):
		Conflict(c, "somente venda de balcão pode ser liquidada por fora do PSP")
	case errors.Is(err, balcao.ErrApprovalPending):
		Conflict(c, "o desconto deste pedido ainda depende de aprovação do gerente")
	case errors.Is(err, balcao.ErrApprovalRejected):
		Conflict(c, "o desconto deste pedido foi recusado: refaça a venda com o valor aprovado")
	case errors.Is(err, balcao.ErrNSUMismatch):
		Conflict(c, "este pedido já foi liquidado com outro NSU")
	case errors.Is(err, balcao.ErrInvalidNSU):
		BadRequest(c, err.Error())
	default:
		respondSaleError(c, err)
	}
}

// isUniqueViolation detecta o SQLSTATE 23505 sem importar o driver pq aqui —
// mesmo motivo do consumer: o handler não deveria conhecer o dialeto, e os
// testes com stub também não produzem um *pq.Error.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "23505") ||
		strings.Contains(s, "duplicate key value") ||
		strings.Contains(s, "unique constraint")
}

// nullIfEmpty grava NULL em vez de string vazia. Importa para o CHECK de
// completude da liquidação e para o índice único parcial: ” e NULL têm
// significados diferentes no banco, e ” seria "informado como vazio".
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
