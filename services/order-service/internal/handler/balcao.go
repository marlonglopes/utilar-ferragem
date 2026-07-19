package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/authclient"
	"github.com/utilar/order-service/internal/balcao"
	"github.com/utilar/order-service/internal/model"
)

// ============================================================================
// Venda de balcão — contexto da venda, fila de aprovação e auditoria
// ----------------------------------------------------------------------------
// As REGRAS moram em internal/balcao (funções puras, com testes de regressão).
// Este arquivo é só a cola: extrai o ator do request, busca o teto autoritativo
// no auth-service, chama a regra e traduz o resultado em HTTP.
// ============================================================================

// saleContext é o resultado de resolver "quem está vendendo, para quem, em que
// loja e com quanto de teto" — calculado UMA vez no começo do Create.
type saleContext struct {
	Channel model.OrderChannel
	// OwnerUserID é quem vai para orders.user_id — o dono para efeito de
	// leitura. No balcão é o operador (ou o cliente, se ele tiver conta).
	OwnerUserID          string
	StoreID              *string
	OperatorID           *string
	CustomerID           *string
	CustomerName         *string
	CustomerDocument     *string
	CustomerPhone        *string
	RequestedDiscountPct float64
	DiscountCeilingPct   float64
}

// resolveSaleContext valida canal, autorização de loja e endereço, e resolve o
// teto de desconto autoritativo.
//
// Erros retornados são sentinelas de internal/balcao (traduzidos por
// respondSaleError) ou saleError para as validações de payload.
func (h *OrderHandler) resolveSaleContext(c *gin.Context, req *model.CreateOrderRequest) (*saleContext, error) {
	userID := c.GetString("user_id")

	channel := req.Channel
	if channel == "" {
		channel = model.ChannelWeb
	}

	if channel == model.ChannelWeb {
		// Contrato antigo, intacto: pedido web exige endereço. A validação saiu
		// do `binding:"required"` para cá porque o binding não sabe o canal.
		if req.Address == nil {
			return nil, saleErrorf("address is required for web orders")
		}
		if req.DiscountPct > 0 {
			// Desconto é instrumento de balcão. Aceitar no site significaria
			// deixar o cliente digitar o próprio desconto no JSON.
			return nil, saleErrorf("discount is only available for balcao orders")
		}
		return &saleContext{Channel: model.ChannelWeb, OwnerUserID: userID}, nil
	}

	// -- balcão ---------------------------------------------------------------
	actor := h.actorFromContext(c)

	// REGRA 1: só cria pedido da própria loja.
	storeID, err := balcao.CanCreateBalcaoOrder(actor, req.StoreID)
	if err != nil {
		return nil, err
	}

	// Endereço é recusado, não ignorado: aceitar em silêncio deixaria de pé o
	// endereço falso que o PDV mandava ("Retirada no balcão", CEP 00000-000) e
	// ele acabaria numa etiqueta de entrega.
	if req.Address != nil {
		return nil, saleErrorf("balcao orders are pickup-only and must not carry a shipping address")
	}

	// A Appmax recusa a cobrança sem celular do pagador (403 confirmado em
	// sandbox). Falhar aqui é muito mais barato que falhar na hora de cobrar,
	// com o cliente parado no caixa.
	phone := digitsOnly(req.CustomerPhone)
	if len(phone) < 10 {
		return nil, saleErrorf("telefone do cliente é obrigatório na venda de balcão")
	}
	if req.CustomerName == "" && req.CustomerID == "" {
		return nil, saleErrorf("identifique o cliente (customerId ou customerName)")
	}

	sale := &saleContext{
		Channel: model.ChannelBalcao,
		// O dono do pedido é o OPERADOR: o cliente de balcão normalmente não tem
		// conta, e gravar user_id de alguém que não existe criaria um pedido que
		// ninguém consegue ler. A identidade do comprador vive nos campos
		// customer_* e em store_customers (auth-service).
		OwnerUserID:          userID,
		StoreID:              &storeID,
		OperatorID:           &userID,
		RequestedDiscountPct: req.DiscountPct,
		DiscountCeilingPct:   actor.DiscountCeilingPct,
	}
	if req.CustomerID != "" {
		sale.CustomerID = &req.CustomerID
	}
	if req.CustomerName != "" {
		sale.CustomerName = &req.CustomerName
	}
	if doc := digitsOnly(req.CustomerDocument); doc != "" {
		sale.CustomerDocument = &doc
	}
	sale.CustomerPhone = &phone

	return sale, nil
}

// actorFromContext monta o ator a partir do token + consulta ao auth-service.
//
// O papel e a loja vêm do JWT (escopo, barato). O TETO DE DESCONTO vem do
// auth-service (dinheiro, tem que ser fresco) — ver o comentário de Claims em
// auth-service/internal/auth/jwt.go.
//
// FAIL-CLOSED: auth-service fora do ar, ou não configurado, resulta em teto 0 —
// toda venda com desconto cai na fila do gerente. O contrário (teto infinito
// durante uma indisponibilidade) transformaria um incidente de infra em rombo
// de caixa.
func (h *OrderHandler) actorFromContext(c *gin.Context) balcao.Actor {
	a := balcao.Actor{
		UserID:  c.GetString("user_id"),
		Role:    c.GetString("user_role"),
		StoreID: c.GetString("store_id"),
		Level:   c.GetString("store_level"),
	}

	if a.Role != balcao.RoleStoreOperator {
		return a
	}
	if h.auth == nil {
		slog.Warn("balcao: operator lookup not configured — discount ceiling forced to 0",
			"user_id", a.UserID, "request_id", c.GetString("request_id"))
		return a
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	op, err := h.auth.GetOperator(ctx, a.UserID)
	if err != nil {
		if !errors.Is(err, authclient.ErrNotOperator) {
			slog.Error("balcao: operator lookup failed — fail-closed (ceiling 0)",
				"error", err, "user_id", a.UserID, "request_id", c.GetString("request_id"))
		}
		// Vínculo revogado depois da emissão do token: zera também o escopo de
		// loja, senão o token velho continuaria valendo como passe da filial.
		if errors.Is(err, authclient.ErrNotOperator) {
			a.StoreID = ""
		}
		return a
	}

	// O vínculo do auth-service é a verdade; o token é só hint. Se divergirem
	// (transferência de filial dentro dos 15min do token), vale o vínculo.
	a.StoreID = op.StoreID
	a.Level = op.Level
	a.DiscountCeilingPct = op.DiscountCeilingPct
	a.CanApprove = op.CanApproveDiscount
	return a
}

// orderRefOf reduz o pedido carregado ao subset que autoriza.
func orderRefOf(o *model.Order) balcao.OrderRef {
	ref := balcao.OrderRef{
		OwnerUserID:    o.UserID,
		Channel:        string(o.Channel),
		ApprovalStatus: o.ApprovalStatus,
	}
	if ref.Channel == "" {
		ref.Channel = balcao.ChannelWeb
	}
	if o.StoreID != nil {
		ref.StoreID = *o.StoreID
	}
	if o.OperatorID != nil {
		ref.OperatorID = *o.OperatorID
	}
	return ref
}

// -- fila de aprovação -------------------------------------------------------

// ListPendingApprovals GET /api/v1/balcao/approvals
//
// A fila do gerente. Escopada pela loja do VÍNCULO — não existe query param de
// loja para operador, pelo mesmo motivo de sempre: quem escolhe a loja é o
// servidor.
func (h *OrderHandler) ListPendingApprovals(c *gin.Context) {
	actor := h.actorFromContext(c)

	where := "approval_status = 'pending' AND store_id = $1"
	args := []any{actor.StoreID}
	switch {
	case actor.Role == balcao.RoleAdmin:
		if storeID := c.Query("storeId"); storeID != "" {
			args = []any{storeID}
		} else {
			where = "approval_status = 'pending'"
			args = []any{}
		}
	case actor.Role == balcao.RoleStoreOperator && actor.StoreID != "":
		// ok — escopo da própria loja
	default:
		Forbidden(c, "store operator role required")
		return
	}

	perPage := 50
	if v, err := strconv.Atoi(c.DefaultQuery("per_page", "50")); err == nil && v > 0 && v <= 100 {
		perPage = v
	}

	rows, err := h.db.Query("SELECT id FROM orders WHERE "+where+
		" ORDER BY created_at ASC LIMIT $"+strconv.Itoa(len(args)+1), append(args, perPage)...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	ids := make([]string, 0, perPage)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			DBError(c, err)
			return
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	out := make([]model.Order, 0, len(ids))
	for _, id := range ids {
		o, err := h.loadOrder(id, "")
		if err != nil {
			DBError(c, err)
			return
		}
		out = append(out, *o)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// Approve PATCH /api/v1/balcao/orders/:id/approve
func (h *OrderHandler) Approve(c *gin.Context) { h.decideApproval(c, balcao.ApprovalApproved) }

// Reject PATCH /api/v1/balcao/orders/:id/reject
func (h *OrderHandler) Reject(c *gin.Context) { h.decideApproval(c, balcao.ApprovalRejected) }

// decideApproval é o corpo comum de aprovar/recusar.
//
// Tudo numa transação com SELECT ... FOR UPDATE: sem o lock, dois gerentes
// clicando ao mesmo tempo (ou o mesmo gerente com dois toques no tablet) leriam
// 'pending' os dois, e o pedido acumularia duas linhas de aprovação com
// aprovadores diferentes na auditoria.
func (h *OrderHandler) decideApproval(c *gin.Context, decision string) {
	orderID := c.Param("id")
	actor := h.actorFromContext(c)

	var req model.ApprovalRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			BadRequest(c, err.Error())
			return
		}
	}
	if decision == balcao.ApprovalRejected && (req.Note == nil || *req.Note == "") {
		// Recusa sem motivo obriga o vendedor a voltar ao gerente para saber o
		// que fazer — e não deixa rastro do porquê.
		BadRequest(c, "informe o motivo da recusa")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var ref balcao.OrderRef
	var channel, status string
	var storeID, operatorID sql.NullString
	var oldPct, oldAmount float64
	err = tx.QueryRow(`
		SELECT channel::text, approval_status::text, store_id, operator_id, discount_pct, discount_amount, user_id
		FROM orders WHERE id = $1 FOR UPDATE
	`, orderID).Scan(&channel, &status, &storeID, &operatorID, &oldPct, &oldAmount, &ref.OwnerUserID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	ref.Channel, ref.ApprovalStatus = channel, status
	ref.StoreID, ref.OperatorID = storeID.String, operatorID.String

	// REGRA 3: ninguém aprova o próprio desconto (nem o gerente que vendeu,
	// nem o admin). Testes em internal/balcao/authz_test.go.
	if err := balcao.CanApproveOrder(actor, ref); err != nil {
		respondSaleError(c, err)
		return
	}

	if _, err := tx.Exec(`
		UPDATE orders SET approval_status = $2, approved_by = $3, approved_at = now(), approval_note = $4
		WHERE id = $1
	`, orderID, decision, actor.UserID, req.Note); err != nil {
		DBError(c, err)
		return
	}

	action := "discount.approved"
	if decision == balcao.ApprovalRejected {
		action = "discount.rejected"
	}
	if err := auditTx(tx, c, balcaoEvent{
		OrderID:  &orderID,
		Action:   action,
		StoreID:  &ref.StoreID,
		Amount:   &oldAmount,
		OldValue: map[string]any{"approvalStatus": status},
		NewValue: map[string]any{
			"approvalStatus": decision,
			"discountPct":    oldPct,
			"approvedBy":     actor.UserID,
			"soldBy":         ref.OperatorID,
			"note":           req.Note,
		},
	}); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	order, err := h.loadOrder(orderID, "")
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// -- auditoria ---------------------------------------------------------------

// balcaoEvent é uma linha da trilha: quem, quando, o quê, de→para, quanto, de
// onde. `created_at` e `request_id` completam o quando/correlação.
type balcaoEvent struct {
	OrderID  *string
	Action   string
	StoreID  *string
	Amount   *float64
	OldValue map[string]any
	NewValue map[string]any
}

// auditTx grava o evento DENTRO da transação de quem chamou.
//
// PORQUÊ propaga o erro (diferente da auditoria do auth-service, que falha
// aberto): aqui a linha de auditoria é parte do fato comercial. Um desconto
// aplicado cuja auditoria não gravou é dinheiro que saiu sem rastro até a
// pessoa — exatamente o que esta tabela existe para impedir. Se a auditoria não
// entra, o pedido não entra.
func auditTx(tx *sql.Tx, c *gin.Context, ev balcaoEvent) error {
	_, err := tx.Exec(`
		INSERT INTO balcao_audit_events
			(order_id, action, actor_id, actor_role, store_id, old_value, new_value, amount, ip, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, ev.OrderID, ev.Action, c.GetString("user_id"), c.GetString("user_role"), ev.StoreID,
		jsonOrNil(ev.OldValue), jsonOrNil(ev.NewValue), ev.Amount,
		c.ClientIP(), c.GetString("request_id"))
	if err != nil {
		slog.Error("balcao audit insert failed",
			"action", ev.Action, "error", err, "request_id", c.GetString("request_id"))
	}
	return err
}

// jsonOrNil devolve STRING (não []byte) de propósito: lib/pq codifica []byte
// como bytea em hexadecimal, e o Postgres recusa isso numa coluna jsonb com
// "invalid input syntax for type json". Como a auditoria falha fechada, esse
// engano derrubaria toda venda de balcão com desconto.
func jsonOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

// -- erros -------------------------------------------------------------------

// saleError é uma falha de validação de payload da venda (vira 400).
type saleError struct{ msg string }

func (e saleError) Error() string { return e.msg }

func saleErrorf(msg string) error { return saleError{msg: msg} }

// respondSaleError traduz os sentinelas de autorização em HTTP.
//
// ErrForeignStore/ErrNotOwner → 404 e não 403: confirmar que o recurso existe
// numa loja alheia já é informação (dá para mapear o parque de lojas e o volume
// de pedidos por enumeração). As demais são 403 porque não revelam nada sobre o
// recurso — só sobre o próprio requisitante.
func respondSaleError(c *gin.Context, err error) {
	var se saleError
	switch {
	case errors.As(err, &se):
		BadRequest(c, se.msg)
	case errors.Is(err, balcao.ErrForeignStore), errors.Is(err, balcao.ErrNotOwner):
		slog.Warn("balcao: cross-store access denied",
			"user_id", c.GetString("user_id"), "reason", err.Error(),
			"request_id", c.GetString("request_id"))
		NotFound(c, "order not found")
	case errors.Is(err, balcao.ErrSelfApproval):
		Forbidden(c, "você não pode aprovar o próprio desconto")
	case errors.Is(err, balcao.ErrNotApprover):
		Forbidden(c, "seu cargo não aprova descontos")
	case errors.Is(err, balcao.ErrNoStoreBinding):
		Forbidden(c, "operador sem loja vinculada")
	case errors.Is(err, balcao.ErrNotOperator):
		Forbidden(c, "store operator role required")
	case errors.Is(err, balcao.ErrNothingToApprove):
		Conflict(c, "pedido não está pendente de aprovação")
	default:
		slog.Error("balcao: unexpected sale error", "error", err,
			"request_id", c.GetString("request_id"))
		InternalError(c, "could not process balcao order")
	}
}

// digitsOnly extrai apenas dígitos (documento e telefone chegam mascarados do
// PDV; gravar mascarado quebraria o lookup por igualdade exata).
func digitsOnly(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return string(out)
}
