package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/model"
	"github.com/utilar/order-service/internal/shipping"
)

// ShippingHandler expõe a cotação de frete pro carrinho.
//
// Endpoint público (não exige pedido): o cliente digita o CEP no carrinho antes
// de existir qualquer pedido. Fica sob RequireUser mesmo assim porque o carrinho
// da Utilar só existe logado — se isso mudar, mover a rota pra fora do grupo
// autenticado é a única alteração necessária.
type ShippingHandler struct {
	rates ShippingRates
}

func NewShippingHandler(rates ShippingRates) *ShippingHandler {
	return &ShippingHandler{rates: rates}
}

// Quote POST /api/v1/shipping/quote
//
// Contrato documentado em docs/shipping-api.md.
func (h *ShippingHandler) Quote(c *gin.Context) {
	var req model.ShippingQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if h.rates == nil {
		InternalError(c, "shipping rates not configured")
		return
	}

	rates, err := h.rates.Rates(c.Request.Context())
	if err != nil {
		DBError(c, err)
		return
	}

	opts, err := shipping.Calculate(rates, shipping.Quote{
		CEP:       req.CEP,
		Subtotal:  req.Subtotal,
		ItemCount: req.ItemCount,
	})
	switch {
	case errors.Is(err, shipping.ErrInvalidCEP):
		BadRequest(c, "CEP inválido: informe 8 dígitos")
		return
	case errors.Is(err, shipping.ErrNoCoverage):
		// 422 e não 404: o recurso (cotação) existe, a entrada é que não é
		// atendível. O frontend mostra "não entregamos nesta região".
		Respond(c, http.StatusUnprocessableEntity, "no_shipping_coverage",
			"não entregamos neste CEP: "+req.CEP)
		return
	case err != nil:
		slog.Error("shipping quote", "error", err, "request_id", c.GetString("request_id"))
		InternalError(c, "could not calculate shipping")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cep":     req.CEP,
		"options": opts,
	})
}
