package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RestockItem — par produto/quantidade a repor. Quantity segue INT (igual a
// ReservationItem): a reposição espelha a baixa da venda online, que reserva
// unidades inteiras. Ver reservation.go.
type RestockItem struct {
	ProductID string `json:"productId" binding:"required,max=64"`
	Quantity  int    `json:"quantity" binding:"required,gt=0,lte=999"`
}

// RestockRequest — payload de POST /api/v1/internal/restock.
//
// `returnId` é a CHAVE DE IDEMPOTÊNCIA — a devolução, não o pedido (um pedido
// tem várias devoluções parciais). All-or-nothing: ou todos os itens sobem o
// saldo, ou nada sobe.
type RestockRequest struct {
	ReturnID string        `json:"returnId" binding:"required,max=64"`
	Reason   string        `json:"reason" binding:"omitempty,max=64"`
	Items    []RestockItem `json:"items" binding:"required,min=1,max=100,dive"`
}

var errRestockProductMissing = errors.New("restock: product not found")

// Restock POST /api/v1/internal/restock
//
// Devolve ao saldo do catálogo a mercadoria conferida numa devolução RECEBIDA.
// É o ESPELHO INVERTIDO da baixa: enquanto a venda faz `stock = stock - qty`
// (reservation.go), aqui é `stock = stock + qty`. Registra um movimento de
// estoque por item (o almoxarife vê a devolução no histórico) e é IDEMPOTENTE
// por `returnId`.
//
// IDEMPOTÊNCIA: a 1ª chamada insere em `stock_restocks(return_id PK)` e executa
// os incrementos na MESMA transação; um retry colide no PK → 0 linhas → tratamos
// como duplicata e devolvemos 200 sem repor de novo. Repor duas vezes seria
// "vender o que não existe" — por isso o guarda é uma trava de banco, não uma
// checagem em memória.
//
// ALL-OR-NOTHING: se qualquer produto não existir, a transação inteira faz
// rollback (inclusive o guarda de idempotência) — melhor o estoque ficar
// SUBESTIMADO (detectável, o order-service audita a falha) que superestimado.
func (h *ReservationHandler) Restock(c *gin.Context) {
	var req RestockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "customer_return"
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op após Commit

	// Guarda de idempotência: só a 1ª reposição deste returnId insere a linha.
	// ON CONFLICT DO NOTHING → 0 linhas numa 2ª chamada (retry).
	res, err := tx.ExecContext(c.Request.Context(), `
		INSERT INTO stock_restocks (return_id, reason, item_count)
		VALUES ($1, $2, $3)
		ON CONFLICT (return_id) DO NOTHING`,
		req.ReturnID, reason, len(req.Items))
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Já reposto antes: no-op idempotente. Fecha a transação sem tocar saldo.
		if err := tx.Commit(); err != nil {
			DBError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"returnId": req.ReturnID, "duplicate": true})
		return
	}

	actorID, actorRole := auditActor(c)
	for _, it := range req.Items {
		// Incremento atômico + saldo resultante. Sem filtro por status: um
		// produto pode ter sido arquivado desde a venda, mas o estoque devolvido
		// é real e precisa voltar mesmo assim.
		var resulting float64
		err := tx.QueryRowContext(c.Request.Context(),
			`UPDATE products SET stock = stock + $2 WHERE id = $1 RETURNING stock`,
			it.ProductID, it.Quantity,
		).Scan(&resulting)
		if errors.Is(err, sql.ErrNoRows) {
			// Produto inexistente → all-or-nothing: rollback de tudo (inclusive o
			// guarda), para um retry futuro poder repor se o dado for corrigido.
			NotFound(c, errRestockProductMissing.Error())
			return
		}
		if err != nil {
			DBError(c, err)
			return
		}

		// Movimento de estoque (delta positivo) na MESMA transação — o histórico
		// do almoxarife reflete a devolução. actor vem do token de serviço.
		if _, err := tx.ExecContext(c.Request.Context(), `
			INSERT INTO stock_movements
				(product_id, delta, reason, resulting_stock, actor_id, actor_role, request_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			it.ProductID, it.Quantity, reason, resulting,
			nullIfEmpty(actorID), nullIfEmpty(actorRole),
			nullIfEmpty(c.GetString("request_id"))); err != nil {
			DBError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"returnId": req.ReturnID, "restocked": len(req.Items)})
}
