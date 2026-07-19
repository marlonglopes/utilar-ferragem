package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// DefaultReservationTTL — janela que o cliente tem pra pagar antes do estoque
// voltar pra vitrine. 30min cobre Pix (expira em ~30min) e o tempo de digitar
// cartão; boleto é tratado à parte pelo order-service, que pede TTL maior.
const DefaultReservationTTL = 30 * time.Minute

// maxReservationTTL trava o TTL que o caller pode pedir. Sem isso um bug (ou
// um serviço comprometido) prenderia estoque por tempo indeterminado.
const maxReservationTTL = 72 * time.Hour

// ReservationItem é um par produto/quantidade a reservar.
type ReservationItem struct {
	ProductID string `json:"productId" binding:"required,max=64"`
	Quantity  int    `json:"quantity" binding:"required,gt=0,lte=999"`
}

// ReserveRequest — payload de POST /api/v1/internal/reservations.
// A reserva é all-or-nothing: ou todos os itens têm saldo, ou nada é reservado.
type ReserveRequest struct {
	OrderID    string            `json:"orderId" binding:"required,max=64"`
	Items      []ReservationItem `json:"items" binding:"required,min=1,max=100,dive"`
	TTLMinutes int               `json:"ttlMinutes" binding:"omitempty,gt=0"`
}

// StockShortage descreve exatamente qual item faltou e quanto havia — o
// order-service repassa isso ao cliente pra ele poder corrigir o carrinho em
// vez de tomar um "erro" opaco.
type StockShortage struct {
	ProductID string `json:"productId"`
	Requested int    `json:"requested"`
	Available int    `json:"available"`
}

// ReservationHandler cuida do ciclo de vida das reservas de estoque.
type ReservationHandler struct {
	db *sql.DB
}

func NewReservationHandler(db *sql.DB) *ReservationHandler {
	return &ReservationHandler{db: db}
}

// errShortage sinaliza falta de saldo pra abortar a transação sem confundir
// com erro de banco.
var errShortage = errors.New("insufficient stock")

// Reserve POST /api/v1/internal/reservations
//
// Reserva N unidades de cada produto do pedido, decrementando `products.stock`
// dentro de uma única transação. All-or-nothing.
//
// CONCORRÊNCIA: o coração é
//
//	UPDATE products SET stock = stock - $n WHERE id = $id AND stock >= $n
//
// O Postgres pega row lock no UPDATE e, se a linha mudou desde o snapshot da
// transação, **reavalia o WHERE contra a versão mais nova** (EvalPlanQual). Ou
// seja: dois clientes disputando a última unidade não podem ambos ver stock=1.
// O segundo bloqueia, relê stock=0, o predicado falha, 0 linhas afetadas.
// Não precisa de SELECT ... FOR UPDATE separado — seria uma ida a mais ao banco
// pra chegar na mesma garantia.
//
// IDEMPOTÊNCIA: `idx_stock_reservations_active` (unique em order_id+product_id
// WHERE status='active') faz o segundo INSERT falhar. Tratamos isso como
// "já reservado" e seguimos, sem decrementar de novo — retry de rede ou
// redelivery do consumer não come estoque duas vezes.
func (h *ReservationHandler) Reserve(c *gin.Context) {
	var req ReserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	ttl := DefaultReservationTTL
	if req.TTLMinutes > 0 {
		ttl = time.Duration(req.TTLMinutes) * time.Minute
		if ttl > maxReservationTTL {
			BadRequest(c, "ttlMinutes exceeds maximum of 4320 (72h)")
			return
		}
	}
	expiresAt := time.Now().Add(ttl)

	// Ordenar por productID dá uma ordem global de aquisição de locks. Sem isso,
	// dois pedidos com os mesmos produtos em ordens opostas podem deadlockar
	// (o Postgres detecta e mata um, mas é um erro 500 evitável).
	items := dedupeItems(req.Items)
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // rollback após commit é no-op

	var shortage *StockShortage
	for _, it := range items {
		res, err := reserveOne(tx, req.OrderID, it, expiresAt)
		if errors.Is(err, errShortage) {
			shortage = res
			break
		}
		if errors.Is(err, sql.ErrNoRows) {
			// Produto inexistente/não publicado — 404 é mais honesto que 409.
			_ = tx.Rollback()
			NotFound(c, fmt.Sprintf("product %s not found or not published", it.ProductID))
			return
		}
		if err != nil {
			DBError(c, err)
			return
		}
	}

	if shortage != nil {
		// Rollback explícito antes de responder: nenhuma unidade fica presa.
		_ = tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"error":     fmt.Sprintf("insufficient stock for product %s: requested %d, available %d", shortage.ProductID, shortage.Requested, shortage.Available),
			"code":      "insufficient_stock",
			"requestId": c.GetString("request_id"),
			"details":   shortage,
		})
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("stock reserved",
		"order_id", req.OrderID,
		"items", len(items),
		"expires_at", expiresAt,
		"request_id", c.GetString("request_id"))

	c.JSON(http.StatusCreated, gin.H{
		"orderId":   req.OrderID,
		"items":     items,
		"expiresAt": expiresAt,
		"status":    "active",
	})
}

// reserveOne executa a reserva de um único item dentro da transação.
// Retorna (*StockShortage, errShortage) quando falta saldo.
func reserveOne(tx *sql.Tx, orderID string, it ReservationItem, expiresAt time.Time) (*StockShortage, error) {
	// Reserva ativa pré-existente pro mesmo (pedido, produto)? Então já
	// decrementamos numa chamada anterior — no-op idempotente.
	var existing int
	err := tx.QueryRow(`
		SELECT quantity FROM stock_reservations
		WHERE order_id = $1 AND product_id = $2 AND status = 'active'
	`, orderID, it.ProductID).Scan(&existing)
	if err == nil {
		return nil, nil // já reservado
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Decremento condicional — atômico, ver comentário em Reserve.
	var remaining int
	err = tx.QueryRow(`
		UPDATE products
		SET stock = stock - $2
		WHERE id = $1 AND status = 'published' AND stock >= $2
		RETURNING stock
	`, it.ProductID, it.Quantity).Scan(&remaining)

	if errors.Is(err, sql.ErrNoRows) {
		// Ou o produto não existe/não está publicado, ou faltou saldo.
		// Distinguir importa: 404 vs 409 mudam a ação do cliente.
		var available int
		qErr := tx.QueryRow(
			`SELECT stock FROM products WHERE id = $1 AND status = 'published'`,
			it.ProductID,
		).Scan(&available)
		if errors.Is(qErr, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		if qErr != nil {
			return nil, qErr
		}
		return &StockShortage{
			ProductID: it.ProductID,
			Requested: it.Quantity,
			Available: available,
		}, errShortage
	}
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO stock_reservations (order_id, product_id, quantity, status, expires_at)
		VALUES ($1, $2, $3, 'active', $4)
	`, orderID, it.ProductID, it.Quantity, expiresAt)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// Commit POST /api/v1/internal/reservations/:orderId/commit
//
// Chamado quando o pedido é pago. O estoque JÁ foi decrementado na reserva —
// aqui só marcamos como baixa definitiva, pra que o sweeper de expiração não
// devolva as unidades pra vitrine.
func (h *ReservationHandler) Commit(c *gin.Context) {
	h.settle(c, "committed")
}

// Release POST /api/v1/internal/reservations/:orderId/release
//
// Cancelamento de pedido (ou pagamento falho): devolve as unidades ao estoque.
func (h *ReservationHandler) Release(c *gin.Context) {
	h.settle(c, "released")
}

// settle move as reservas ativas de um pedido pro estado final, devolvendo
// estoque quando for release. Idempotente: chamar duas vezes não devolve
// unidades duas vezes, porque o UPDATE filtra por status='active' e a segunda
// chamada não encontra nada.
func (h *ReservationHandler) settle(c *gin.Context, newStatus string) {
	orderID := c.Param("orderId")
	if orderID == "" {
		BadRequest(c, "orderId is required")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	// Marca e captura o que foi marcado numa tacada. O CTE garante que a
	// devolução ao estoque usa exatamente as linhas que ESTA transação virou —
	// um concorrente que rode em paralelo não vê as mesmas linhas.
	rows, err := tx.Query(`
		UPDATE stock_reservations
		SET status = $2, updated_at = now()
		WHERE order_id = $1 AND status = 'active'
		RETURNING product_id, quantity
	`, orderID, newStatus)
	if err != nil {
		DBError(c, err)
		return
	}

	type freed struct {
		productID string
		qty       int
	}
	var settled []freed
	for rows.Next() {
		var f freed
		if err := rows.Scan(&f.productID, &f.qty); err != nil {
			rows.Close()
			DBError(c, err)
			return
		}
		settled = append(settled, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		DBError(c, err)
		return
	}
	rows.Close()

	if newStatus == "released" {
		for _, f := range settled {
			if _, err := tx.Exec(
				`UPDATE products SET stock = stock + $2 WHERE id = $1`,
				f.productID, f.qty,
			); err != nil {
				DBError(c, err)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("reservation settled",
		"order_id", orderID,
		"status", newStatus,
		"items", len(settled),
		"request_id", c.GetString("request_id"))

	c.JSON(http.StatusOK, gin.H{
		"orderId": orderID,
		"status":  newStatus,
		"settled": len(settled),
	})
}

// dedupeItems soma quantidades do mesmo produto. O carrinho do frontend não
// deveria mandar duplicatas, mas se mandar, dois INSERTs pro mesmo par
// (order_id, product_id) bateriam no unique index e o segundo viraria no-op —
// reservando MENOS do que o pedido precisa. Somar antes evita o buraco.
func dedupeItems(items []ReservationItem) []ReservationItem {
	byID := make(map[string]int, len(items))
	order := make([]string, 0, len(items))
	for _, it := range items {
		if _, seen := byID[it.ProductID]; !seen {
			order = append(order, it.ProductID)
		}
		byID[it.ProductID] += it.Quantity
	}
	out := make([]ReservationItem, 0, len(order))
	for _, id := range order {
		out = append(out, ReservationItem{ProductID: id, Quantity: byID[id]})
	}
	return out
}
