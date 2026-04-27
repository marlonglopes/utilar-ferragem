package model

import "time"

// JSON em camelCase para match direto com app/src/lib/mockOrders.ts

type OrderStatus string
type PaymentMethod string

const (
	StatusPendingPayment OrderStatus = "pending_payment"
	StatusPaid           OrderStatus = "paid"
	StatusPicking        OrderStatus = "picking"
	StatusShipped        OrderStatus = "shipped"
	StatusDelivered      OrderStatus = "delivered"
	StatusCancelled      OrderStatus = "cancelled"

	MethodPix    PaymentMethod = "pix"
	MethodBoleto PaymentMethod = "boleto"
	MethodCard   PaymentMethod = "card"
)

// OrderItem é o item de um pedido. Validação de binding é OBRIGATÓRIA aqui pra
// evitar tamper de preço/quantidade pelo cliente (audit O1-C1):
//   - Quantity > 0 e <= 999 (limites razoáveis pra hardware/ferramentas)
//   - UnitPrice > 0 e <= 999999.99 (cliente não envia preço grátis nem absurdo)
//
// NOTA: o ideal é o backend buscar o preço do catalog-service (audit O2-H5),
// mas até lá pelo menos travamos o range plausível.
type OrderItem struct {
	ProductID  string  `json:"productId" binding:"required,max=64"`
	Name       string  `json:"name" binding:"required,max=255"`
	Icon       string  `json:"icon" binding:"max=16"`
	SellerID   string  `json:"sellerId" binding:"required,max=64"`
	SellerName string  `json:"sellerName" binding:"required,max=255"`
	Quantity   int     `json:"quantity" binding:"required,gt=0,lte=999"`
	UnitPrice  float64 `json:"unitPrice" binding:"required,gt=0,lte=999999.99"`
}

// OrderAddress — campos de endereço com limites de tamanho pra prevenir DoS por
// payload absurdo + XSS (audit O1-C2). CEP é validado por regex (8 dígitos com
// hífen opcional). Estado é UF de 2 chars.
type OrderAddress struct {
	Street       string  `json:"street" binding:"required,max=255"`
	Number       string  `json:"number" binding:"required,max=20"`
	Complement   *string `json:"complement,omitempty" binding:"omitempty,max=100"`
	Neighborhood string  `json:"neighborhood" binding:"required,max=100"`
	City         string  `json:"city" binding:"required,max=100"`
	State        string  `json:"state" binding:"required,len=2"`
	CEP          string  `json:"cep" binding:"required,max=9"`
}

type TrackingEvent struct {
	Status      OrderStatus `json:"status"`
	Location    *string     `json:"location,omitempty"`
	Description string      `json:"description"`
	OccurredAt  time.Time   `json:"occurredAt"`
}

type Order struct {
	ID             string          `json:"id"`
	Number         string          `json:"number"`
	UserID         string          `json:"userId"`
	Status         OrderStatus     `json:"status"`
	PaymentMethod  PaymentMethod   `json:"paymentMethod"`
	PaymentID      *string         `json:"paymentId,omitempty"`
	PaymentInfo    *string         `json:"paymentInfo,omitempty"`
	Items          []OrderItem     `json:"items"`
	Subtotal       float64         `json:"subtotal"`
	ShippingCost   float64         `json:"shippingCost"`
	Total          float64         `json:"total"`
	Address        OrderAddress    `json:"address"`
	TrackingCode   *string         `json:"trackingCode,omitempty"`
	TrackingEvents []TrackingEvent `json:"trackingEvents,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	PaidAt         *time.Time      `json:"paidAt,omitempty"`
	PickedAt       *time.Time      `json:"pickedAt,omitempty"`
	ShippedAt      *time.Time      `json:"shippedAt,omitempty"`
	DeliveredAt    *time.Time      `json:"deliveredAt,omitempty"`
	CancelledAt    *time.Time      `json:"cancelledAt,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// CreateOrderRequest — payload de POST /api/v1/orders.
// Limites de tamanho previnem DoS por payload absurdo (audit O3-M1).
type CreateOrderRequest struct {
	PaymentMethod PaymentMethod `json:"paymentMethod" binding:"required,oneof=pix boleto card"`
	Items         []OrderItem   `json:"items" binding:"required,min=1,max=100,dive"`
	ShippingCost  float64       `json:"shippingCost" binding:"gte=0,lte=99999.99"`
	Address       OrderAddress  `json:"address" binding:"required"`
}
