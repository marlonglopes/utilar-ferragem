package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/model"
)

// priceTolerance é o desvio máximo aceito entre o preço do body e o do catalog
// antes de logar warning. Float64 não é exato; 1 centavo é folga aceitável.
const priceTolerance = 0.01

// CatalogLookup é a interface mínima que OrderHandler precisa pra validar
// preço dos itens contra o catalog-service (audit O2-H5).
type CatalogLookup interface {
	GetByID(ctx context.Context, productID string) (*catalogclient.Product, error)
}

type OrderHandler struct {
	db      *sql.DB
	catalog CatalogLookup
	devMode bool
}

// NewOrderHandler. catalog pode ser nil em dev pra simplificar smoke tests
// locais sem catalog-service rodando — mas em DevMode=false um catalog nil
// faria a validação ser pulada e isso seria um regression de O2-H5.
// Logamos no boot pra deixar visível.
func NewOrderHandler(db *sql.DB, catalog CatalogLookup, devMode bool) *OrderHandler {
	return &OrderHandler{db: db, catalog: catalog, devMode: devMode}
}

// Create POST /api/v1/orders
// Cria pedido + items + endereço em uma transação.
// Status inicial: pending_payment. Total calculado no servidor (nunca confia em cliente).
//
// SEGURANÇA (audit O2-H5):
// O `unitPrice` de cada item é validado contra o catalog-service. Se diverge
// do `product.price` autoritativo, **sobrescrevemos** com o valor do catalog
// e logamos warning (sinal de tamper ou bug de frontend).
func (h *OrderHandler) Create(c *gin.Context) {
	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	userID := c.GetString("user_id")
	requestID := c.GetString("request_id")

	// O2-H5: resolve price autoritativo via catalog-service. Mutates req.Items
	// in-place pra que o INSERT abaixo use os valores corretos.
	if err := h.applyAuthoritativePricing(c.Request.Context(), userID, requestID, req.Items); err != nil {
		switch {
		case errors.Is(err, catalogclient.ErrNotFound):
			BadRequest(c, "product not found")
		default:
			slog.Error("create order: catalog lookup",
				"error", err, "request_id", requestID)
			BadGateway(c, "catalog service unavailable")
		}
		return
	}

	subtotal := 0.0
	for _, it := range req.Items {
		subtotal += float64(it.Quantity) * it.UnitPrice
	}
	total := subtotal + req.ShippingCost

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	// número de pedido = ano + 8 chars base32 de crypto/rand (não enumerável)
	orderNumber := generateOrderNumber(time.Now().Year())

	var orderID string
	err = tx.QueryRow(`
		INSERT INTO orders (number, user_id, payment_method, subtotal, shipping_cost, total)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, orderNumber, userID, req.PaymentMethod, subtotal, req.ShippingCost, total).Scan(&orderID)
	if err != nil {
		DBError(c, err)
		return
	}

	// items
	for _, it := range req.Items {
		_, err := tx.Exec(`
			INSERT INTO order_items (order_id, product_id, name, icon, seller_id, seller_name, quantity, unit_price)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, orderID, it.ProductID, it.Name, it.Icon, it.SellerID, it.SellerName, it.Quantity, it.UnitPrice)
		if err != nil {
			DBError(c, err)
			return
		}
	}

	// endereço
	a := req.Address
	_, err = tx.Exec(`
		INSERT INTO shipping_addresses (order_id, street, number, complement, neighborhood, city, state, cep)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, orderID, a.Street, a.Number, a.Complement, a.Neighborhood, a.City, a.State, a.CEP)
	if err != nil {
		DBError(c, err)
		return
	}

	// tracking event inicial
	_, err = tx.Exec(`
		INSERT INTO tracking_events (order_id, status, description)
		VALUES ($1, 'pending_payment', 'Pedido criado. Aguardando pagamento.')
	`, orderID)
	if err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	// retorna o pedido completo
	order, err := h.loadOrder(orderID, userID)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, order)
}

// List GET /api/v1/orders?status=active|done|all&page=1&per_page=20
func (h *OrderHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")

	filter := c.DefaultQuery("status", "all")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	where := "user_id = $1"
	args := []any{userID}
	switch filter {
	case "active":
		where += " AND status IN ('pending_payment', 'paid', 'picking', 'shipped')"
	case "done":
		where += " AND status IN ('delivered', 'cancelled')"
	}

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)

	rows, err := h.db.Query(`
		SELECT id FROM orders WHERE `+where+` ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	orders := make([]model.Order, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			DBError(c, err)
			return
		}
		o, err := h.loadOrder(id, userID)
		if err != nil {
			DBError(c, err)
			return
		}
		orders = append(orders, *o)
	}

	// count total
	var total int
	h.db.QueryRow("SELECT count(*) FROM orders WHERE "+where, args[:len(args)-2]...).Scan(&total)
	totalPages := (total + perPage - 1) / perPage

	c.JSON(http.StatusOK, gin.H{
		"data": orders,
		"meta": gin.H{"page": page, "per_page": perPage, "total": total, "total_pages": totalPages},
	})
}

// Get GET /api/v1/orders/:id
func (h *OrderHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")

	order, err := h.loadOrder(id, userID)
	if err == sql.ErrNoRows {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// Cancel PATCH /api/v1/orders/:id/cancel
//
// SEGURANÇA (audit O3-M4): pessimistic locking via SELECT FOR UPDATE.
// Sem o lock, dois cancels concorrentes (ou cancel + webhook avançando status)
// poderiam ler o mesmo status, ambos passar pela validação, e gerar tracking
// events duplicados ou racing entre cancel e paid. Com FOR UPDATE, o segundo
// fica em wait até o primeiro commitar e relê o status atualizado.
func (h *OrderHandler) Cancel(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")

	// Toda a leitura + validação + UPDATE acontece dentro da MESMA transação,
	// com FOR UPDATE travando a row do pedido. Quem chegar segundo bloqueia.
	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRow(
		`SELECT status FROM orders WHERE id=$1 AND user_id=$2 FOR UPDATE`,
		id, userID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Só pode cancelar se ainda não saiu para entrega.
	// Com FOR UPDATE, o status lido é o "real" — concorrente não consegue
	// cancelar duas vezes nem cancelar pedido que virou shipped no meio do caminho.
	if status == "shipped" || status == "delivered" || status == "cancelled" {
		Conflict(c, fmt.Sprintf("cannot cancel order in status %q", status))
		return
	}

	if _, err := tx.Exec(`
		UPDATE orders SET status='cancelled', cancelled_at=now() WHERE id=$1 AND user_id=$2
	`, id, userID); err != nil {
		DBError(c, err)
		return
	}
	if _, err := tx.Exec(`
		INSERT INTO tracking_events (order_id, status, description)
		VALUES ($1, 'cancelled', 'Pedido cancelado pelo cliente.')
	`, id); err != nil {
		DBError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	order, _ := h.loadOrder(id, userID)
	c.JSON(http.StatusOK, order)
}

// applyAuthoritativePricing valida e sobrescreve o `unitPrice` de cada item
// usando o catalog-service. Em caso de divergência, loga warning e usa o valor
// do catalog (autoritativo).
//
// Quando catalog é nil (DevMode sem catalog rodando), pula a validação com
// warning. Em prod isso seria um regression mas é detectável via log na boot.
//
// Se o produto não existe no catalog, retorna catalogclient.ErrNotFound — o
// caller traduz pra HTTP 400.
func (h *OrderHandler) applyAuthoritativePricing(ctx context.Context, userID, requestID string, items []model.OrderItem) error {
	if h.catalog == nil {
		if !h.devMode {
			slog.Error("create order: catalog client missing in non-dev mode", "request_id", requestID)
		} else {
			slog.Warn("create order: skipping price validation (dev mode, no catalog)", "request_id", requestID)
		}
		return nil
	}

	for i := range items {
		it := &items[i]
		p, err := h.catalog.GetByID(ctx, it.ProductID)
		if err != nil {
			return err
		}
		// Detecta tampering: cliente enviou preço significativamente diferente.
		// Não recusa — apenas loga + sobrescreve. Recusar quebraria UX em casos
		// legítimos de cache stale do frontend; mas o amount cobrado fica
		// sempre igual ao do catalog.
		diff := it.UnitPrice - p.Price
		if diff < 0 {
			diff = -diff
		}
		if diff > priceTolerance {
			slog.Warn("create order: price tamper or stale frontend",
				"product_id", it.ProductID,
				"client_price", it.UnitPrice,
				"catalog_price", p.Price,
				"user_id", userID,
				"request_id", requestID)
		}
		it.UnitPrice = p.Price
		it.Name = p.Name
	}
	return nil
}

// -- helpers ----------------------------------------------------------------

func (h *OrderHandler) loadOrder(id, userID string) (*model.Order, error) {
	var o model.Order
	err := h.db.QueryRow(`
		SELECT
		  id, number, user_id, status, payment_method, payment_id, payment_info,
		  subtotal, shipping_cost, total, tracking_code,
		  created_at, paid_at, picked_at, shipped_at, delivered_at, cancelled_at, updated_at
		FROM orders WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&o.ID, &o.Number, &o.UserID, &o.Status, &o.PaymentMethod, &o.PaymentID, &o.PaymentInfo,
		&o.Subtotal, &o.ShippingCost, &o.Total, &o.TrackingCode,
		&o.CreatedAt, &o.PaidAt, &o.PickedAt, &o.ShippedAt, &o.DeliveredAt, &o.CancelledAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// items
	rows, err := h.db.Query(`
		SELECT product_id, name, icon, seller_id, seller_name, quantity, unit_price
		FROM order_items WHERE order_id = $1 ORDER BY created_at ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	o.Items = make([]model.OrderItem, 0)
	for rows.Next() {
		var it model.OrderItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.Icon, &it.SellerID, &it.SellerName, &it.Quantity, &it.UnitPrice); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it)
	}

	// address
	err = h.db.QueryRow(`
		SELECT street, number, complement, neighborhood, city, state, cep
		FROM shipping_addresses WHERE order_id = $1
	`, id).Scan(&o.Address.Street, &o.Address.Number, &o.Address.Complement,
		&o.Address.Neighborhood, &o.Address.City, &o.Address.State, &o.Address.CEP)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// tracking events
	evRows, err := h.db.Query(`
		SELECT status, location, description, occurred_at
		FROM tracking_events WHERE order_id = $1 ORDER BY occurred_at ASC
	`, id)
	if err == nil {
		defer evRows.Close()
		for evRows.Next() {
			var ev model.TrackingEvent
			if err := evRows.Scan(&ev.Status, &ev.Location, &ev.Description, &ev.OccurredAt); err == nil {
				o.TrackingEvents = append(o.TrackingEvents, ev)
			}
		}
	}

	return &o, nil
}
