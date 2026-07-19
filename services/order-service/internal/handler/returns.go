package handler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/model"
	"github.com/utilar/order-service/internal/paymentclient"
	"github.com/utilar/order-service/internal/returns"
)

// ============================================================================
// Devolução e troca — endpoints
// ----------------------------------------------------------------------------
// Até aqui não existia fluxo NENHUM: o cliente não tinha como pedir devolução e
// a loja não tinha como registrar. Isso é descumprimento direto do CDC, não um
// item de backlog.
//
// A divisão de responsabilidade segue a mesma do resto do serviço:
//
//	internal/returns  — REGRAS puras (prazo, base legal, autorização,
//	                    quantidade, trava de split). Testáveis sem banco.
//	este arquivo      — persistência, transação, trilha e efeitos externos.
//
// As TRÊS defesas do endpoint que move dinheiro (o de estorno), na ordem:
//
//	1. QUEM   — returns.CanDecide + escopo de dono (returns.Evaluate).
//	2. RASTRO — auditoria FAIL-CLOSED na mesma transação: se a trilha não
//	            grava, a decisão não acontece.
//	3. UMA VEZ— máquina de estados com FOR UPDATE + flags ledger_posted /
//	            stock_returned + idempotência do lançamento pelo returnID.
//
// ⚠️ Este arquivo implementa o caminho do LOJISTA (a Utilar vende o próprio
// produto e responde pela devolução). Ver docs/devolucao-e-troca.md.
// ============================================================================

// RefundPoster é o contrato mínimo pro handler lançar o estorno no livro
// contábil do payment-service. Interface pequena para o handler ser testável
// com stub — e para que um deploy sem payment-service degrade de forma
// explícita, não silenciosa. Mesmo desenho de LedgerPoster.
type RefundPoster interface {
	PostReturnRefund(ctx context.Context, in paymentclient.ReturnRefund) error
}

// StockRestorer devolve mercadoria ao estoque do catalog-service.
//
// ⚠️ NÃO é o Release da reserva. Release cancela uma RESERVA de um pedido que
// ainda não foi pago; aqui a baixa já aconteceu (o produto saiu, foi entregue e
// voltou), então o que se precisa é INCREMENTAR o saldo.
//
// ⚠️ PENDÊNCIA CONHECIDA: a rota correspondente ainda NÃO EXISTE no
// catalog-service (ele expõe apenas /reservations, /commit e /release). Ver
// docs/devolucao-e-troca.md § "Dependência do catalog-service". Enquanto não
// existir, o recebimento é registrado normalmente, `stock_returned` fica false
// e uma linha `return.stock_restore_failed` entra na trilha — a devolução do
// estoque vira tarefa do relatório de pendências, nunca um silêncio.
type StockRestorer interface {
	Restock(ctx context.Context, returnID string, items []catalogclient.RestockItem) error
}

// ReturnHandler serve as rotas de devolução.
type ReturnHandler struct {
	db     *sql.DB
	ledger RefundPoster
	stock  StockRestorer
}

func NewReturnHandler(db *sql.DB) *ReturnHandler {
	return &ReturnHandler{db: db}
}

// WithRefundLedger liga o lançamento contábil do estorno.
func (h *ReturnHandler) WithRefundLedger(l RefundPoster) *ReturnHandler {
	h.ledger = l
	return h
}

// WithRestock liga a devolução de estoque ao catálogo.
func (h *ReturnHandler) WithRestock(s StockRestorer) *ReturnHandler {
	h.stock = s
	return h
}

// ---------------------------------------------------------------------------
// POST /api/v1/orders/:id/returns — o cliente pede a devolução
// ---------------------------------------------------------------------------

// Create abre um pedido de devolução.
//
// A base legal (arrependimento × vício) é DERIVADA da data pelo
// returns.Evaluate, não escolhida pelo cliente nem pela loja. E o
// arrependimento nasce JÁ APROVADO: mandar para uma fila de análise um direito
// que a lei diz ser incondicional só cria o lugar onde ele vai ser negado por
// engano.
func (h *ReturnHandler) Create(c *gin.Context) {
	orderID := c.Param("id")
	requestID := c.GetString("request_id")
	actor := returns.Actor{UserID: c.GetString("user_id"), Role: c.GetString("user_role")}

	var req model.CreateReturnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	// FOR UPDATE no pedido: sem o lock, dois pedidos de devolução simultâneos
	// do mesmo item leriam o mesmo "já devolvido: 0", os dois passariam pela
	// validação de quantidade e o cliente devolveria 2 de um item comprado 1
	// vez — com estorno em dobro.
	ref, err := loadOrderRefForReturn(tx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	items, err := loadReturnableItems(tx, orderID)
	if err != nil {
		DBError(c, err)
		return
	}

	wanted := make([]returns.RequestedItem, 0, len(req.Items))
	for _, it := range req.Items {
		wanted = append(wanted, returns.RequestedItem{
			OrderItemID: it.OrderItemID, Quantity: it.Quantity,
		})
	}

	decision, err := returns.Evaluate(returns.Request{
		Actor: actor, Order: ref, Items: items, Wanted: wanted,
		Reason: req.Reason, Now: time.Now().UTC(),
	})
	if err != nil {
		respondReturnError(c, err)
		return
	}

	returnID, err := newUUIDv4()
	if err != nil {
		InternalError(c, "could not generate return id")
		return
	}

	status := returns.StatusRequested
	var decidedBy *string
	var decidedAt *time.Time
	if decision.AutoApproved {
		// Deferimento automático do art. 49. `decided_by` recebe o sistema, e
		// não o cliente: quem "decidiu" foi a lei, e registrar o próprio
		// comprador como aprovador do próprio estorno confundiria a auditoria
		// com exatamente o padrão que ela existe para detectar.
		status = returns.StatusApproved
		sistema := "system:cdc-art-49"
		agora := time.Now().UTC()
		decidedBy, decidedAt = &sistema, &agora
	}

	var basis, deadline any
	if !decision.Deadline.Basis.IsZero() {
		basis = decision.Deadline.Basis
		deadline = decision.Deadline.RegretAt
	}

	if _, err := tx.Exec(`
		INSERT INTO order_returns
			(id, order_id, user_id, kind, status, reason_code, reason_text,
			 deadline_basis, deadline_at, basis_source,
			 refund_amount, refund_shipping,
			 decided_by, decided_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, returnID, orderID, ref.UserID, string(decision.Kind), string(status),
		nullIfEmpty(reasonCodeOf(decision.Kind, req.Reason)), nullIfEmpty(req.Reason),
		basis, deadline, string(decision.Deadline.Source),
		decision.ItemsAmount, decision.ShippingRefund,
		decidedBy, decidedAt); err != nil {
		DBError(c, err)
		return
	}

	for _, it := range decision.Items {
		if _, err := tx.Exec(`
			INSERT INTO order_return_items
				(return_id, order_item_id, product_id, quantity, unit_price, line_amount)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, returnID, it.OrderItemID, it.ProductID, it.Quantity, it.UnitPrice, it.LineAmount); err != nil {
			DBError(c, err)
			return
		}
	}

	// RASTRO fail-closed na MESMA transação. Uma devolução aberta sem trilha é
	// um estorno futuro sem origem rastreável.
	if err := auditReturnTx(tx, c, returnEvent{
		ReturnID: &returnID, OrderID: &orderID, Action: "return.requested",
		Amount: &decision.TotalRefund,
		NewValue: map[string]any{
			"kind": string(decision.Kind), "status": string(status),
			"autoApproved": decision.AutoApproved,
			"basisSource":  string(decision.Deadline.Source),
			"itemsAmount":  decision.ItemsAmount,
			"shipping":     decision.ShippingRefund,
			"isFullReturn": decision.IsFullReturn,
			"reason":       req.Reason,
		},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("devolução solicitada",
		"return_id", returnID, "order_id", orderID, "kind", string(decision.Kind),
		"auto_approved", decision.AutoApproved, "amount", decision.TotalRefund,
		"basis_source", string(decision.Deadline.Source),
		"user_id", actor.UserID, "request_id", requestID)

	if decision.Deadline.NeedsOperationalReview() {
		// Pedido marcado como entregue SEM data de entrega. A devolução seguiu
		// (a favor do consumidor, ver returns/deadline.go), mas o buraco de
		// dado tem que ser consertado NA ORIGEM.
		slog.Warn("devolução aceita sem data de entrega registrada — "+
			"a loja não tem como provar o início do prazo neste pedido",
			"order_id", orderID, "return_id", returnID, "request_id", requestID)
	}

	h.respondReturn(c, returnID, http.StatusCreated)
}

// ---------------------------------------------------------------------------
// Decisão — aprovar / recusar
// ---------------------------------------------------------------------------

// Approve PATCH /api/v1/admin/returns/:rid/approve
func (h *ReturnHandler) Approve(c *gin.Context) { h.decide(c, returns.StatusApproved) }

// Reject PATCH /api/v1/admin/returns/:rid/reject
//
// ⚠️ Nunca alcança um arrependimento: returns.CanReject recusa, e o CHECK
// `returns_regret_cannot_be_rejected` no banco é a barreira que sobrevive a um
// bug aqui.
func (h *ReturnHandler) Reject(c *gin.Context) { h.decide(c, returns.StatusRejected) }

func (h *ReturnHandler) decide(c *gin.Context, to returns.Status) {
	returnID := c.Param("rid")
	requestID := c.GetString("request_id")
	role := c.GetString("user_role")

	var req model.ReturnDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		BadRequest(c, err.Error())
		return
	}

	// DEFESA 1 — QUEM, antes de qualquer escrita e antes de revelar qualquer
	// coisa sobre a devolução.
	if !returns.CanDecide(role) {
		Forbidden(c, "papel não pode decidir sobre devoluções")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	cur, err := lockReturn(tx, returnID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "return not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	if to == returns.StatusRejected {
		if err := returns.CanReject(returns.Kind(cur.Kind), req.Note); err != nil {
			respondReturnError(c, err)
			return
		}
	}
	if err := returns.CanTransition(returns.Status(cur.Status), to); err != nil {
		Conflict(c, err.Error())
		return
	}

	actorID := c.GetString("user_id")
	if _, err := tx.Exec(`
		UPDATE order_returns
		   SET status = $2::return_status, decided_by = $3, decided_at = now(), decision_note = $4
		 WHERE id = $1
	`, returnID, string(to), actorID, nullIfEmpty(req.Note)); err != nil {
		DBError(c, err)
		return
	}

	if err := auditReturnTx(tx, c, returnEvent{
		ReturnID: &returnID, OrderID: &cur.OrderID,
		Action:   "return." + string(to),
		Amount:   &cur.RefundTotal,
		OldValue: map[string]any{"status": cur.Status},
		NewValue: map[string]any{
			"status": string(to), "decidedBy": actorID, "note": req.Note,
		},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("devolução decidida",
		"return_id", returnID, "order_id", cur.OrderID, "de", cur.Status, "para", string(to),
		"decided_by", actorID, "amount", cur.RefundTotal,
		"ip", c.ClientIP(), "request_id", requestID)

	h.respondReturn(c, returnID, http.StatusOK)
}

// ---------------------------------------------------------------------------
// Receive — a mercadoria chegou e foi CONFERIDA
// ---------------------------------------------------------------------------

// Receive PATCH /api/v1/admin/returns/:rid/receive
//
// ⚠️ É AQUI — e só aqui — que o ESTOQUE VOLTA.
//
// Devolver estoque na solicitação (ou na aprovação) coloca à venda um produto
// que ainda está na casa do cliente, ou que nunca vai ser postado de volta. O
// sistema passaria a vender o que não tem, e quem descobre é a SEGUNDA venda,
// com o cliente já esperando a entrega.
func (h *ReturnHandler) Receive(c *gin.Context) {
	returnID := c.Param("rid")
	requestID := c.GetString("request_id")

	var req model.ReturnReceiveRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		BadRequest(c, err.Error())
		return
	}

	if !returns.CanDecide(c.GetString("user_role")) {
		Forbidden(c, "papel não pode receber devoluções")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	cur, err := lockReturn(tx, returnID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "return not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	if err := returns.CanTransition(returns.Status(cur.Status), returns.StatusReceived); err != nil {
		Conflict(c, err.Error())
		return
	}

	if _, err := tx.Exec(`
		UPDATE order_returns SET status = 'received', received_at = now() WHERE id = $1
	`, returnID); err != nil {
		DBError(c, err)
		return
	}

	if err := auditReturnTx(tx, c, returnEvent{
		ReturnID: &returnID, OrderID: &cur.OrderID, Action: "return.received",
		Amount:   &cur.RefundTotal,
		OldValue: map[string]any{"status": cur.Status},
		NewValue: map[string]any{"status": string(returns.StatusReceived), "note": req.Note},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("devolução recebida e conferida",
		"return_id", returnID, "order_id", cur.OrderID,
		"received_by", c.GetString("user_id"), "request_id", requestID)

	// A devolução de estoque acontece FORA da transação: é HTTP para outro
	// serviço. Segurar o lock do registro durante uma chamada de rede travaria
	// a conferência do balcão inteira.
	h.restock(c, returnID, req.RestockableItems)

	h.respondReturn(c, returnID, http.StatusOK)
}

// restock devolve a mercadoria conferida ao saldo do catálogo.
//
// A ordem é "registra o recebimento, depois repõe", e a assimetria é
// deliberada: se a reposição falhar, temos mercadoria na loja que o sistema
// ainda não oferece — estoque SUBESTIMADO, que custa uma venda perdida e é
// detectável pelo relatório de pendências. A ordem inversa produziria saldo
// disponível para mercadoria que talvez nunca tenha chegado: venda do que não
// existe, que é o erro que não se aceita.
func (h *ReturnHandler) restock(c *gin.Context, returnID string, restrict []model.ReturnItemRequest) {
	requestID := c.GetString("request_id")

	items, err := loadRestockItems(h.db, returnID, restrict)
	if err != nil {
		slog.Error("devolução: falha ao carregar itens para reposição",
			"error", err, "return_id", returnID, "request_id", requestID)
		h.auditReturnFailure(c, returnID, "return.stock_restore_failed", err.Error())
		return
	}
	if len(items) == 0 {
		// Tudo voltou inservível. Não é falha: é decisão do conferente.
		slog.Info("devolução recebida sem itens reaproveitáveis",
			"return_id", returnID, "request_id", requestID)
		return
	}

	if h.stock == nil {
		slog.Error("devolução recebida MAS estoque NÃO reposto: catalog-service não "+
			"configurado para reposição — o saldo ficará subestimado",
			"return_id", returnID, "request_id", requestID)
		h.auditReturnFailure(c, returnID, "return.stock_restore_failed",
			"catalog-service não configurado para reposição")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := h.stock.Restock(ctx, returnID, items); err != nil {
		slog.Error("devolução: reposição de estoque falhou",
			"error", err, "return_id", returnID, "request_id", requestID)
		h.auditReturnFailure(c, returnID, "return.stock_restore_failed", err.Error())
		return
	}

	if _, err := h.db.Exec(`UPDATE order_returns SET stock_returned = true WHERE id = $1`, returnID); err != nil {
		slog.Error("devolução: estoque reposto mas flag não gravada — "+
			"a reposição pode ser tentada de novo (é idempotente por return_id)",
			"error", err, "return_id", returnID, "request_id", requestID)
		return
	}
	slog.Info("devolução: estoque reposto", "return_id", returnID, "request_id", requestID)
}

// ---------------------------------------------------------------------------
// Refund — o dinheiro sai
// ---------------------------------------------------------------------------

// Refund PATCH /api/v1/admin/returns/:rid/refund
//
// ⚠️ ESTE É O ENDPOINT QUE TIRA DINHEIRO DO CAIXA. Só alcançável a partir de
// `received`: não existe aresta approved → refunded, porque estornar antes de a
// mercadoria voltar é entregar produto E dinheiro para a mesma pessoa.
func (h *ReturnHandler) Refund(c *gin.Context) {
	returnID := c.Param("rid")
	requestID := c.GetString("request_id")
	actorID := c.GetString("user_id")

	if !returns.CanDecide(c.GetString("user_role")) {
		Forbidden(c, "papel não pode estornar devoluções")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	cur, err := lockReturn(tx, returnID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "return not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// DEFESA 3 — UMA VEZ. Devolução já estornada é retry: retenta só o
	// lançamento contábil (que é idempotente do outro lado pelo returnID) e
	// devolve o registro como está. Sem segunda linha de auditoria de estorno e
	// sem segunda saída de dinheiro.
	if returns.Status(cur.Status) == returns.StatusRefunded {
		_ = tx.Rollback()
		h.postRefundToLedger(c, cur)
		h.respondReturn(c, returnID, http.StatusOK)
		return
	}

	if err := returns.CanTransition(returns.Status(cur.Status), returns.StatusRefunded); err != nil {
		Conflict(c, err.Error())
		return
	}
	if cur.RefundTotal <= 0 {
		Conflict(c, "devolução sem valor a estornar")
		return
	}

	if _, err := tx.Exec(`
		UPDATE order_returns SET status = 'refunded', refunded_at = now() WHERE id = $1
	`, returnID); err != nil {
		DBError(c, err)
		return
	}

	// DEFESA 2 — RASTRO, na MESMA transação e FAIL-CLOSED. Estorno é dinheiro
	// saindo por decisão humana: quem aprovou, quando e quanto é exatamente o
	// registro que não pode faltar. Se a trilha não grava, o estorno não
	// acontece. Mesmo princípio da liquidação externa.
	if err := auditReturnTx(tx, c, returnEvent{
		ReturnID: &returnID, OrderID: &cur.OrderID, Action: "return.refunded",
		Amount:   &cur.RefundTotal,
		OldValue: map[string]any{"status": cur.Status},
		NewValue: map[string]any{
			"status": string(returns.StatusRefunded),
			"amount": cur.RefundAmount, "shipping": cur.RefundShipping,
			"total": cur.RefundTotal, "kind": cur.Kind,
			"refundedBy": actorID, "partial": !cur.FullReturn,
		},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("ESTORNO de devolução autorizado",
		"return_id", returnID, "order_id", cur.OrderID, "amount", cur.RefundTotal,
		"kind", cur.Kind, "refunded_by", actorID,
		"ip", c.ClientIP(), "request_id", requestID)

	h.postRefundToLedger(c, cur)
	h.respondReturn(c, returnID, http.StatusOK)
}

// postRefundToLedger lança o estorno no livro do payment-service.
//
// Best-effort e FORA da transação, pelo mesmo motivo de postExternalToLedger: o
// livro vive noutro banco, noutro serviço, e uma transação distribuída aqui
// seguraria o lock do registro durante uma chamada HTTP.
//
// A ordem é "estorna primeiro, lança depois". Se o lançamento falhar, temos um
// estorno sem lançamento — despesa SUBESTIMADA, detectável e replicável (a
// chamada é idempotente pelo returnID). A ordem inversa produziria estorno no
// livro para dinheiro que não saiu: despesa inventada.
func (h *ReturnHandler) postRefundToLedger(c *gin.Context, r returnRow) {
	requestID := c.GetString("request_id")

	if h.ledger == nil {
		slog.Error("ESTORNO não lançado no livro: payment-service não configurado "+
			"(PAYMENT_SERVICE_URL/SERVICE_JWT_SECRET) — a despesa do período ficará subestimada",
			"return_id", r.ID, "order_id", r.OrderID, "request_id", requestID)
		h.auditReturnFailure(c, r.ID, "return.ledger_post_failed", "payment-service não configurado")
		return
	}

	ctx, cancel := context.WithTimeout(
		paymentclient.WithRequestID(context.Background(), requestID), 6*time.Second)
	defer cancel()

	err := h.ledger.PostReturnRefund(ctx, paymentclient.ReturnRefund{
		ReturnID: r.ID, OrderID: r.OrderID, PaymentID: r.PaymentID,
		AmountBRL: r.RefundAmount, ShippingBRL: r.RefundShipping,
		Method: r.PaymentMethod, Partial: !r.FullReturn,
		ApprovedBy: c.GetString("user_id"), OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		slog.Error("ESTORNO: lançamento contábil falhou",
			"error", err, "return_id", r.ID, "order_id", r.OrderID, "request_id", requestID)
		h.auditReturnFailure(c, r.ID, "return.ledger_post_failed", err.Error())
		return
	}

	if _, err := h.db.Exec(`UPDATE order_returns SET ledger_posted = true WHERE id = $1`, r.ID); err != nil {
		slog.Error("ESTORNO: lançado no livro mas flag não gravada",
			"error", err, "return_id", r.ID, "request_id", requestID)
	}
}

// auditReturnFailure registra na trilha que um efeito externo NÃO aconteceu.
//
// Fora da transação (que já foi commitada), então aqui a auditoria falha
// ABERTA — ao contrário da trilha da decisão em si. A diferença é proposital:
// recusar a resposta agora não desfaria o estorno já gravado, só esconderia do
// atendente que ele aconteceu.
func (h *ReturnHandler) auditReturnFailure(c *gin.Context, returnID, action, reason string) {
	_, err := h.db.Exec(`
		INSERT INTO return_audit_events
			(return_id, action, actor_id, actor_role, new_value, ip, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, returnID, action, c.GetString("user_id"), c.GetString("user_role"),
		jsonOrNil(map[string]any{"reason": reason}), c.ClientIP(), c.GetString("request_id"))
	if err != nil {
		slog.Error("devolução: falha ao auditar a falha do efeito externo",
			"error", err, "return_id", returnID, "action", action,
			"request_id", c.GetString("request_id"))
	}
}

// ---------------------------------------------------------------------------
// Leitura
// ---------------------------------------------------------------------------

// ListForOrder GET /api/v1/orders/:id/returns — devoluções de um pedido.
//
// Escopado pelo JWT: o cliente vê só as próprias. Zero IDOR, igual ao resto do
// serviço.
func (h *ReturnHandler) ListForOrder(c *gin.Context) {
	orderID := c.Param("id")
	scope := ""
	if !canSeeOthers(c.GetString("user_role")) {
		scope = c.GetString("user_id")
	}

	rows, err := h.queryReturns(`
		WHERE r.order_id = $1 AND ($2 = '' OR r.user_id = $2)
		ORDER BY r.requested_at DESC`, orderID, scope)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"returns": rows})
}

// ListQueue GET /api/v1/admin/returns — a fila do atendimento.
func (h *ReturnHandler) ListQueue(c *gin.Context) {
	if !returns.CanDecide(c.GetString("user_role")) {
		Forbidden(c, "papel não pode ver a fila de devoluções")
		return
	}
	status := c.Query("status")
	rows, err := h.queryReturns(`
		WHERE ($1 = '' AND r.status IN ('requested','approved','in_transit','received')
		       OR r.status::text = $1)
		ORDER BY r.requested_at ASC LIMIT 200`, status)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"returns": rows})
}

// Get GET /api/v1/returns/:rid
func (h *ReturnHandler) Get(c *gin.Context) {
	returnID := c.Param("rid")
	scope := ""
	if !canSeeOthers(c.GetString("user_role")) {
		scope = c.GetString("user_id")
	}

	rows, err := h.queryReturns(`WHERE r.id = $1 AND ($2 = '' OR r.user_id = $2)`, returnID, scope)
	if err != nil {
		DBError(c, err)
		return
	}
	if len(rows) == 0 {
		// 404 e não 403 mesmo quando a devolução existe mas é de outro: 403
		// confirmaria a existência e transformaria a rota num enumerador.
		NotFound(c, "return not found")
		return
	}
	c.JSON(http.StatusOK, rows[0])
}

func (h *ReturnHandler) respondReturn(c *gin.Context, returnID string, status int) {
	rows, err := h.queryReturns(`WHERE r.id = $1`, returnID)
	if err != nil || len(rows) == 0 {
		DBError(c, err)
		return
	}
	c.JSON(status, rows[0])
}

// canSeeOthers — quem enxerga devolução de terceiro. `seller` fora, de
// propósito: lojista do marketplace não é atendente da loja.
func canSeeOthers(role string) bool {
	switch role {
	case returns.RoleAdmin, returns.RoleOperator, returns.RoleStoreOperator, returns.RoleService:
		return true
	}
	return false
}

// reasonCodeOf deriva um código de motivo. No arrependimento o código é fixo
// (não se pergunta o motivo); no vício, hoje, é genérico — a taxonomia fechada
// entra quando o formulário do frontend existir.
func reasonCodeOf(kind returns.Kind, reason string) string {
	if kind == returns.KindRegret {
		return "regret"
	}
	if reason == "" {
		return ""
	}
	return "defect_reported"
}

// respondReturnError traduz os sentinelas de internal/returns em HTTP.
func respondReturnError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, returns.ErrNotOwner):
		// 404 e não 403: anti-enumeração. Mesma decisão do resto do serviço.
		NotFound(c, "order not found")
	case errors.Is(err, returns.ErrOrderNotReturnable):
		Conflict(c, err.Error())
	case errors.Is(err, returns.ErrDefectWindowExpired):
		Respond(c, http.StatusUnprocessableEntity, "return_window_expired",
			"o prazo para solicitar devolução deste pedido já passou")
	case errors.Is(err, returns.ErrReasonRequired):
		BadRequest(c, "descreva o problema do produto: fora dos 7 dias de "+
			"arrependimento a devolução é analisada como vício do produto (CDC art. 26)")
	case errors.Is(err, returns.ErrSplitPartialRefund):
		// Recusado AQUI e não lá no PSP — ver o comentário do erro.
		Respond(c, http.StatusUnprocessableEntity, "split_requires_full_refund",
			"este pedido só aceita devolução total. Selecione todos os itens.")
	case errors.Is(err, returns.ErrQuantityExceeded),
		errors.Is(err, returns.ErrItemNotInOrder),
		errors.Is(err, returns.ErrNoItems):
		BadRequest(c, err.Error())
	case errors.Is(err, returns.ErrRegretCannotBeRejected):
		Respond(c, http.StatusUnprocessableEntity, "regret_is_unconditional", err.Error())
	case errors.Is(err, returns.ErrDecisionNoteRequired):
		BadRequest(c, err.Error())
	case errors.Is(err, returns.ErrNotReviewer):
		Forbidden(c, err.Error())
	default:
		InternalError(c, "could not process return")
	}
}
