package handler

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/model"
	"github.com/utilar/order-service/internal/returns"
)

// ============================================================================
// Acesso a dados da devolução
// ----------------------------------------------------------------------------
// Separado do handler para que o handler fique legível: lá está a POLÍTICA
// (ordem das defesas, o que é fail-closed, o que é best-effort); aqui, o SQL.
// ============================================================================

// returnRow é a linha de order_returns já achatada com o que o handler precisa
// do pedido pai (método de pagamento, id do pagamento).
type returnRow struct {
	ID             string
	OrderID        string
	UserID         string
	Kind           string
	Status         string
	RefundAmount   float64
	RefundShipping float64
	RefundTotal    float64
	PaymentMethod  string
	PaymentID      string
	// FullReturn diz se esta devolução esgota o pedido. Recalculado na leitura
	// (e não gravado) porque depende do que OUTRAS devoluções do mesmo pedido
	// fizeram — um valor congelado ficaria errado assim que a segunda devolução
	// parcial existisse.
	FullReturn bool
}

// loadOrderRefForReturn trava o pedido e devolve o recorte que as regras usam.
//
// FOR UPDATE: sem o lock, dois pedidos de devolução simultâneos do mesmo item
// leriam o mesmo "já devolvido: 0", os dois passariam pela validação de
// quantidade e o cliente devolveria 2 de um item comprado 1 vez — estorno em
// dobro.
func loadOrderRefForReturn(tx *sql.Tx, orderID string) (returns.OrderRef, error) {
	var r returns.OrderRef
	var paidAt, shippedAt, deliveredAt sql.NullTime
	err := tx.QueryRow(`
		SELECT id, user_id, status::text, channel::text,
		       paid_at, shipped_at, delivered_at,
		       COALESCE(payment_split, false), COALESCE(shipping_cost, 0)
		  FROM orders WHERE id = $1 FOR UPDATE
	`, orderID).Scan(&r.ID, &r.UserID, &r.Status, &r.Channel,
		&paidAt, &shippedAt, &deliveredAt, &r.PaymentSplit, &r.ShippingCost)
	if err != nil {
		return r, err
	}
	if paidAt.Valid {
		r.PaidAt = &paidAt.Time
	}
	if shippedAt.Valid {
		r.ShippedAt = &shippedAt.Time
	}
	if deliveredAt.Valid {
		r.DeliveredAt = &deliveredAt.Time
	}
	return r, nil
}

// loadReturnableItems traz os itens do pedido com o quanto de cada um JÁ foi
// devolvido.
//
// O LEFT JOIN desconta apenas devoluções que NÃO foram recusadas nem
// canceladas: uma devolução indeferida não pode continuar consumindo o saldo
// devolvível do cliente — ele ficaria impedido de devolver de novo por causa de
// um pedido que a própria loja negou.
func loadReturnableItems(tx *sql.Tx, orderID string) (map[string]returns.OrderItemRef, error) {
	rows, err := tx.Query(`
		SELECT oi.id, oi.product_id, oi.quantity, oi.unit_price,
		       COALESCE(SUM(ri.quantity) FILTER (
		           WHERE r.status NOT IN ('rejected','cancelled')
		       ), 0) AS ja_devolvido
		  FROM order_items oi
		  LEFT JOIN order_return_items ri ON ri.order_item_id = oi.id
		  LEFT JOIN order_returns      r  ON r.id = ri.return_id
		 WHERE oi.order_id = $1
		 GROUP BY oi.id, oi.product_id, oi.quantity, oi.unit_price
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]returns.OrderItemRef{}
	for rows.Next() {
		var it returns.OrderItemRef
		if err := rows.Scan(&it.ID, &it.ProductID, &it.Quantity, &it.UnitPrice, &it.AlreadyReturned); err != nil {
			return nil, err
		}
		out[it.ID] = it
	}
	// rows.Err() sempre checado (convenção do repo): sem isto, um erro no meio
	// da iteração viraria "o pedido não tem itens" e a devolução seria recusada
	// por um motivo falso.
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// lockReturn trava o registro de devolução e devolve o estado atual.
//
// FOR UPDATE em order_returns (e não em orders): sem o lock, dois cliques no
// botão de estornar leriam os dois `status = 'received'`, os dois passariam
// pela máquina de estados e o dinheiro sairia duas vezes.
func lockReturn(tx *sql.Tx, returnID string) (returnRow, error) {
	var r returnRow
	err := tx.QueryRow(`
		SELECT r.id, r.order_id, r.user_id, r.kind::text, r.status::text,
		       r.refund_amount, r.refund_shipping,
		       o.payment_method::text, COALESCE(o.payment_id::text, '')
		  FROM order_returns r
		  JOIN orders o ON o.id = r.order_id
		 WHERE r.id = $1
		 FOR UPDATE OF r
	`, returnID).Scan(&r.ID, &r.OrderID, &r.UserID, &r.Kind, &r.Status,
		&r.RefundAmount, &r.RefundShipping, &r.PaymentMethod, &r.PaymentID)
	if err != nil {
		return r, err
	}
	r.RefundTotal = round2(r.RefundAmount + r.RefundShipping)

	full, err := isFullReturnOfOrder(tx, r.OrderID)
	if err != nil {
		return r, err
	}
	r.FullReturn = full
	return r, nil
}

// isFullReturnOfOrder diz se TODAS as unidades do pedido estão em devoluções
// vivas. É o que decide `partial` no lançamento contábil e o que a trava de
// split do PSP observa.
func isFullReturnOfOrder(tx *sql.Tx, orderID string) (bool, error) {
	var comprado, devolvido int
	err := tx.QueryRow(`
		SELECT COALESCE(SUM(oi.quantity), 0),
		       COALESCE(SUM(dev.qtd), 0)
		  FROM order_items oi
		  LEFT JOIN LATERAL (
		      SELECT SUM(ri.quantity) AS qtd
		        FROM order_return_items ri
		        JOIN order_returns r ON r.id = ri.return_id
		       WHERE ri.order_item_id = oi.id
		         AND r.status NOT IN ('rejected','cancelled')
		  ) dev ON true
		 WHERE oi.order_id = $1
	`, orderID).Scan(&comprado, &devolvido)
	if err != nil {
		return false, err
	}
	return comprado > 0 && devolvido >= comprado, nil
}

// loadRestockItems monta a lista do que volta ao estoque.
//
// `restrict` vazio = tudo volta. Preenchido = só o que o conferente marcou como
// reaproveitável — mandar de volta à vitrine um produto que voltou destruído é
// vender lixo para o próximo cliente.
//
// Quantidade restrita é limitada pela devolvida: o conferente não pode repor
// mais do que voltou.
func loadRestockItems(db *sql.DB, returnID string, restrict []model.ReturnItemRequest) ([]catalogclient.RestockItem, error) {
	rows, err := db.Query(`
		SELECT order_item_id, product_id, quantity, unit_price, line_amount
		  FROM order_return_items WHERE return_id = $1
	`, returnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// orderItemID vem junto porque `restrict` é indexado por ele — o conferente
	// marca linhas do pedido, não produtos.
	type linha struct {
		orderItemID string
		item        catalogclient.RestockItem
	}
	all := []linha{}
	for rows.Next() {
		var l linha
		var unitPrice, lineAmount float64
		if err := rows.Scan(&l.orderItemID, &l.item.ProductID, &l.item.Quantity,
			&unitPrice, &lineAmount); err != nil {
			return nil, err
		}
		all = append(all, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]catalogclient.RestockItem, 0, len(all))
	if len(restrict) == 0 {
		for _, l := range all {
			out = append(out, l.item)
		}
		return out, nil
	}

	limite := map[string]int{}
	for _, r := range restrict {
		limite[r.OrderItemID] += r.Quantity
	}
	for _, l := range all {
		q, ok := limite[l.orderItemID]
		if !ok || q <= 0 {
			continue
		}
		if q > l.item.Quantity {
			q = l.item.Quantity // nunca repor mais do que voltou
		}
		l.item.Quantity = q
		out = append(out, l.item)
	}
	return out, nil
}

// queryReturns lê devoluções com seus itens. `where` é literal do código
// (nunca entrada de usuário); os valores vão por placeholder.
func (h *ReturnHandler) queryReturns(where string, args ...any) ([]model.Return, error) {
	rows, err := h.db.Query(`
		SELECT r.id, r.order_id, r.user_id, r.kind::text, r.status::text,
		       COALESCE(r.reason_text,''), r.refund_amount, r.refund_shipping,
		       r.deadline_at, r.deadline_basis, r.basis_source,
		       r.decided_by, r.decided_at, r.decision_note,
		       r.requested_at, r.shipped_at, r.received_at, r.refunded_at,
		       r.stock_returned, r.ledger_posted, r.created_at, r.updated_at
		  FROM order_returns r `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Return{}
	ids := []string{}
	for rows.Next() {
		var r model.Return
		var deadlineAt, basis, decidedAt, shippedAt, receivedAt, refundedAt sql.NullTime
		var decidedBy, note sql.NullString
		if err := rows.Scan(&r.ID, &r.OrderID, &r.UserID, &r.Kind, &r.Status,
			&r.Reason, &r.RefundAmount, &r.RefundShipping,
			&deadlineAt, &basis, &r.BasisSource,
			&decidedBy, &decidedAt, &note,
			&r.RequestedAt, &shippedAt, &receivedAt, &refundedAt,
			&r.StockReturned, &r.LedgerPosted, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.RefundTotal = round2(r.RefundAmount + r.RefundShipping)
		r.LegalBasis = model.LegalBasisText(r.Kind)
		assignTime(&r.DeadlineAt, deadlineAt)
		assignTime(&r.DeadlineBasis, basis)
		assignTime(&r.ShippedAt, shippedAt)
		assignTime(&r.ReceivedAt, receivedAt)
		assignTime(&r.RefundedAt, refundedAt)
		assignTime(&r.DecidedAt, decidedAt)
		if decidedBy.Valid {
			v := decidedBy.String
			r.DecidedBy = &v
		}
		if note.Valid {
			v := note.String
			r.DecisionNote = &v
		}
		out = append(out, r)
		ids = append(ids, r.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Itens em UMA query para os N registros — carregar item a item seria N+1
	// na tela de fila do atendimento, que lista até 200.
	byReturn, err := h.loadReturnItems(ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Items = byReturn[out[i].ID]
		if out[i].Items == nil {
			out[i].Items = []model.ReturnItem{}
		}
	}
	return out, nil
}

func (h *ReturnHandler) loadReturnItems(ids []string) (map[string][]model.ReturnItem, error) {
	rows, err := h.db.Query(`
		SELECT return_id, order_item_id, product_id, quantity, unit_price, line_amount
		  FROM order_return_items WHERE return_id = ANY($1)
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]model.ReturnItem{}
	for rows.Next() {
		var rid string
		var it model.ReturnItem
		if err := rows.Scan(&rid, &it.OrderItemID, &it.ProductID, &it.Quantity,
			&it.UnitPrice, &it.LineAmount); err != nil {
			return nil, err
		}
		out[rid] = append(out[rid], it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func assignTime(dst **time.Time, v sql.NullTime) {
	if v.Valid {
		t := v.Time
		*dst = &t
	}
}

// -- trilha de auditoria -----------------------------------------------------

// returnEvent é uma linha da trilha: quem, quando, o quê, de→para, quanto, de
// onde. Espelha balcaoEvent, em tabela própria (ver a migration 007 sobre o
// porquê de não reaproveitar balcao_audit_events).
type returnEvent struct {
	ReturnID *string
	OrderID  *string
	Action   string
	Amount   *float64
	OldValue map[string]any
	NewValue map[string]any
}

// auditReturnTx grava o evento DENTRO da transação de quem chamou e PROPAGA o
// erro.
//
// ⚠️ FAIL-CLOSED, igual ao auditTx do balcão e pelo mesmo motivo: a linha de
// auditoria é parte do fato. Um estorno cuja trilha não gravou é dinheiro que
// saiu sem rastro até a pessoa que autorizou — exatamente o que esta tabela
// existe para impedir. Se a auditoria não entra, o estorno não entra.
func auditReturnTx(tx *sql.Tx, c *gin.Context, ev returnEvent) error {
	_, err := tx.Exec(`
		INSERT INTO return_audit_events
			(return_id, order_id, action, actor_id, actor_role, old_value, new_value, amount, ip, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, ev.ReturnID, ev.OrderID, ev.Action, c.GetString("user_id"), c.GetString("user_role"),
		jsonOrNil(ev.OldValue), jsonOrNil(ev.NewValue), ev.Amount,
		c.ClientIP(), c.GetString("request_id"))
	if err != nil {
		slog.Error("return audit insert failed",
			"action", ev.Action, "error", err, "request_id", c.GetString("request_id"))
	}
	return err
}
