package handler

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// recordStockMovement grava um movimento de estoque. Fail-open (igual a audit e
// price history): perder o movimento não pode derrubar a operação de negócio.
//
// É o que UNIFICA a série do histórico: toda mudança de estoque — o ajuste com
// motivo do almoxarife (Adjust) E a edição do produto pela tela de Produtos —
// vira uma linha aqui. Sem isto, editar o estoque pelo formulário do produto
// sumia do histórico do almoxarife.
func recordStockMovement(db execer, c *gin.Context, productID string, delta, resulting float64, reason string) {
	actorID, actorRole := auditActor(c)
	_, err := db.Exec(`
		INSERT INTO stock_movements
			(product_id, delta, reason, resulting_stock, actor_id, actor_role, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		productID, delta, reason, resulting,
		nullIfEmpty(actorID), nullIfEmpty(actorRole), nullIfEmpty(c.GetString("request_id")))
	if err != nil {
		slog.Error("stock_movement.write_failed",
			"product_id", productID, "reason", reason,
			"request_id", c.GetString("request_id"), "error", err.Error())
	}
}

// StockHandler é a tela do ALMOXARIFE: ver estoque, ajustar com MOTIVO, e o
// histórico de movimento. NÃO devolve custo em lugar nenhum — o almoxarife não
// vê custo (persona). A proteção é a projeção (nunca seleciona `cost`), não um
// filtro depois. Ver docs/backoffice-personas.md.
type StockHandler struct{ db *sql.DB }

func NewStockHandler(db *sql.DB) *StockHandler { return &StockHandler{db: db} }

type stockRow struct {
	ID                string  `json:"id"`
	SKU               *string `json:"sku"`
	Name              string  `json:"name"`
	Stock             float64 `json:"stock"`
	LowStockThreshold float64 `json:"lowStockThreshold"`
	LowStock          bool    `json:"lowStock"`
	Status            string  `json:"status"`
}

// List GET /api/v1/admin/stock?q=&low=1&page=&per_page=
//
// Lista produtos com estoque. `low=1` traz só os no/abaixo do limite. Sem
// custo. Os de estoque baixo sobem para o topo — é o que o almoxarife precisa
// ver primeiro. Projeção EXPLÍCITA (sem SELECT *) para o custo nunca escapar.
func (h *StockHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	where := "1=1"
	var args []any
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		args = append(args, "%"+q+"%")
		p := len(args)
		where += fmt.Sprintf(" AND (name ILIKE $%d OR sku ILIKE $%d)", p, p)
	}
	if c.Query("low") == "1" {
		where += " AND stock <= low_stock_threshold"
	}

	countArgs := append([]any{}, args...)
	offset := (page - 1) * perPage
	limitPH, offsetPH := len(args)+1, len(args)+2
	args = append(args, perPage, offset)

	// #nosec G201 — `where` só tem literais com placeholders posicionais ($N);
	// valores em args. Mesmo padrão do resto do serviço.
	rows, err := h.db.Query(fmt.Sprintf(`
		SELECT id, sku, name, stock, low_stock_threshold, status
		FROM products
		WHERE %s
		ORDER BY (stock <= low_stock_threshold) DESC, name ASC
		LIMIT $%d OFFSET $%d`, where, limitPH, offsetPH), args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]stockRow, 0, perPage)
	for rows.Next() {
		var r stockRow
		if err := rows.Scan(&r.ID, &r.SKU, &r.Name, &r.Stock, &r.LowStockThreshold, &r.Status); err != nil {
			DBError(c, err)
			return
		}
		r.LowStock = r.Stock <= r.LowStockThreshold
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var total int
	if err := h.db.QueryRow("SELECT count(*) FROM products WHERE "+where, countArgs...).Scan(&total); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page": page, "per_page": perPage, "total": total,
			"total_pages": (total + perPage - 1) / perPage,
		},
	})
}

type adjustInput struct {
	Delta  *float64 `json:"delta"`  // + entrada / - saída-ajuste
	Reason string   `json:"reason"` // obrigatório: contagem, recebimento, avaria...
}

// Adjust POST /api/v1/admin/stock/:id/adjust  {delta, reason}
//
// Ajuste RELATIVO (delta), não sobrescrita: o almoxarife conta a prateleira e
// lança a diferença, dá entrada de recebimento (+) ou baixa de avaria (-).
// Motivo é OBRIGATÓRIO — "estoque mudou sem motivo" obriga a caçar o porquê
// depois. Atômico: trava a linha, aplica, grava o movimento e a trilha, tudo na
// mesma transação. Recusa deixar o estoque negativo (respeita o CHECK).
func (h *StockHandler) Adjust(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		BadRequest(c, "id do produto é obrigatório")
		return
	}
	var in adjustInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, "corpo inválido")
		return
	}
	if in.Delta == nil || *in.Delta == 0 {
		BadRequest(c, "delta é obrigatório e diferente de zero")
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		BadRequest(c, "motivo é obrigatório")
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		DBError(c, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var cur float64
	err = tx.QueryRowContext(c.Request.Context(),
		`SELECT stock FROM products WHERE id = $1 FOR UPDATE`, id).Scan(&cur)
	if err == sql.ErrNoRows {
		NotFound(c, "produto não encontrado")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	newStock := cur + *in.Delta
	if newStock < 0 {
		Conflict(c, "estoque não pode ficar negativo")
		return
	}

	if _, err := tx.ExecContext(c.Request.Context(),
		`UPDATE products SET stock = $2 WHERE id = $1`, id, newStock); err != nil {
		DBError(c, err)
		return
	}

	actorID, actorRole := auditActor(c)
	if _, err := tx.ExecContext(c.Request.Context(), `
		INSERT INTO stock_movements
			(product_id, delta, reason, resulting_stock, actor_id, actor_role, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, *in.Delta, reason, newStock,
		nullIfEmpty(actorID), nullIfEmpty(actorRole), nullIfEmpty(c.GetString("request_id"))); err != nil {
		DBError(c, err)
		return
	}

	// Trilha do catálogo: o ajuste vira uma linha "stock.adjust" com o de→para e
	// o motivo — aparece na atividade unificada como qualquer outra edição.
	changes := AuditChanges{}
	changes.changed("stock", cur, newStock)
	changes["reason"] = FieldChange{Old: nil, New: reason}
	audit(tx, c, "stock.adjust", "product", id, changes)

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": id, "stock": newStock, "delta": *in.Delta, "reason": reason,
	})
}

type movementRow struct {
	ID             string  `json:"id"`
	Delta          float64 `json:"delta"`
	Reason         string  `json:"reason"`
	ResultingStock float64 `json:"resultingStock"`
	ActorID        *string `json:"actorId"`
	ActorRole      *string `json:"actorRole"`
	CreatedAt      string  `json:"createdAt"`
}

// Movements GET /api/v1/admin/stock/:id/movements — histórico do produto.
func (h *StockHandler) Movements(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		BadRequest(c, "id do produto é obrigatório")
		return
	}
	rows, err := h.db.Query(`
		SELECT id, delta, reason, resulting_stock, actor_id, actor_role, created_at
		FROM stock_movements
		WHERE product_id = $1
		ORDER BY created_at DESC
		LIMIT 100`, id)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]movementRow, 0, 32)
	for rows.Next() {
		var m movementRow
		var created time.Time
		if err := rows.Scan(&m.ID, &m.Delta, &m.Reason, &m.ResultingStock,
			&m.ActorID, &m.ActorRole, &created); err != nil {
			DBError(c, err)
			return
		}
		m.CreatedAt = created.Format(time.RFC3339)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
