package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/fulfillment"
	"github.com/utilar/order-service/internal/model"
)

// ============================================================================
// Endpoints de operação (separar / despachar / entregar)
// ----------------------------------------------------------------------------
// Protegidos por RequireRole("admin", "operator"): quem embala e despacha não
// precisa (nem deve ter) poder de admin sobre o resto do sistema.
//
// Todos passam pelo mesmo fulfillment.Advance, então herdam de graça o lock da
// linha, a validação da máquina de estados, o timestamp certo e o tracking
// event. Transição inválida vira 409 com a mensagem "de X pra Y não pode" — o
// operador entende o que fez de errado sem abrir log.
// ============================================================================

// MarkPicking PATCH /api/v1/admin/orders/:id/picking
// Pedido separado no estoque (paid → picking).
func (h *OrderHandler) MarkPicking(c *gin.Context) {
	h.advance(c, model.StatusPicking, false)
}

// MarkShipped PATCH /api/v1/admin/orders/:id/shipped
// Pedido despachado (picking → shipped). Exige código de rastreio: um pedido
// "enviado" sem rastreio é uma reclamação de suporte garantida.
func (h *OrderHandler) MarkShipped(c *gin.Context) {
	h.advance(c, model.StatusShipped, true)
}

// MarkDelivered PATCH /api/v1/admin/orders/:id/delivered
// Entrega confirmada (shipped → delivered).
func (h *OrderHandler) MarkDelivered(c *gin.Context) {
	h.advance(c, model.StatusDelivered, false)
}

// advance é o corpo comum dos três endpoints.
func (h *OrderHandler) advance(c *gin.Context, to model.OrderStatus, requireTracking bool) {
	orderID := c.Param("id")

	var req model.FulfillmentRequest
	// Body é opcional para picking/delivered — um PATCH sem corpo é legítimo.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			BadRequest(c, err.Error())
			return
		}
	}

	if requireTracking && (req.TrackingCode == nil || *req.TrackingCode == "") {
		BadRequest(c, "trackingCode is required to mark an order as shipped")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	desc := ""
	if req.Note != nil {
		desc = *req.Note
	}

	from, err := fulfillment.Advance(tx, orderID, to, fulfillment.Options{
		Description:  desc,
		Location:     req.Location,
		TrackingCode: req.TrackingCode,
	})
	switch {
	case errors.Is(err, fulfillment.ErrOrderNotFound):
		NotFound(c, "order not found")
		return
	case err != nil:
		var invalid model.ErrInvalidTransition
		if errors.As(err, &invalid) {
			Conflict(c, err.Error())
			return
		}
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	slog.Info("order advanced",
		"order_id", orderID,
		"from", from,
		"to", to,
		"operator", c.GetString("user_id"),
		"request_id", c.GetString("request_id"))

	order, err := h.loadOrder(orderID, "")
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// AdminCancel PATCH /api/v1/admin/orders/:id/cancel
// Cancelamento pelo operador (ex.: item quebrado no estoque). Diferente do
// cancelamento do cliente só no escopo: não filtra por dono e devolve o
// estoque igualmente.
func (h *OrderHandler) AdminCancel(c *gin.Context) {
	orderID := c.Param("id")

	var req model.FulfillmentRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			BadRequest(c, err.Error())
			return
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var hadReservation bool
	_ = tx.QueryRow(`SELECT stock_reserved FROM orders WHERE id=$1`, orderID).Scan(&hadReservation)

	desc := "Pedido cancelado pela operação."
	if req.Note != nil && *req.Note != "" {
		desc = *req.Note
	}

	if _, err := fulfillment.Advance(tx, orderID, model.StatusCancelled, fulfillment.Options{
		Description: desc,
	}); err != nil {
		switch {
		case errors.Is(err, fulfillment.ErrOrderNotFound):
			NotFound(c, "order not found")
		default:
			var invalid model.ErrInvalidTransition
			if errors.As(err, &invalid) {
				Conflict(c, err.Error())
				return
			}
			DBError(c, err)
		}
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	if hadReservation && h.stock != nil {
		relCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.stock.Release(relCtx, orderID); err != nil {
			slog.Error("admin cancel: stock release failed",
				"error", err, "order_id", orderID)
		}
	}

	order, err := h.loadOrder(orderID, "")
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}
