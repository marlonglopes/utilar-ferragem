package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/model"
)

type OrderHandler struct{ db *sql.DB }

func NewOrderHandler(db *sql.DB) *OrderHandler { return &OrderHandler{db: db} }

// Create POST /api/v1/orders
// Cria pedido + items + endereço em uma transação.
// Status inicial: pending_payment. Total calculado no servidor (nunca confia em cliente).
func (h *OrderHandler) Create(c *gin.Context) {
	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	userID := c.GetString("user_id")
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

	// gera número de pedido humano: ANO-SEQ (simples para dev; em prod usar sequence dedicada)
	orderNumber := fmt.Sprintf("%d-%d", time.Now().Year(), time.Now().UnixNano()%100000)

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
func (h *OrderHandler) Cancel(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")

	// Carrega + valida estado
	var status string
	err := h.db.QueryRow("SELECT status FROM orders WHERE id=$1 AND user_id=$2", id, userID).Scan(&status)
	if err == sql.ErrNoRows {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Só pode cancelar se ainda não saiu para entrega
	if status == "shipped" || status == "delivered" || status == "cancelled" {
		Conflict(c, fmt.Sprintf("cannot cancel order in status %q", status))
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

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
