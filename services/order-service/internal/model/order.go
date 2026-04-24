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

type OrderItem struct {
	ProductID  string  `json:"productId"`
	Name       string  `json:"name"`
	Icon       string  `json:"icon"`
	SellerID   string  `json:"sellerId"`
	SellerName string  `json:"sellerName"`
	Quantity   int     `json:"quantity"`
	UnitPrice  float64 `json:"unitPrice"`
}

type OrderAddress struct {
	Street       string  `json:"street"`
	Number       string  `json:"number"`
	Complement   *string `json:"complement,omitempty"`
	Neighborhood string  `json:"neighborhood"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	CEP          string  `json:"cep"`
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

// Payload de criação (POST /api/v1/orders)
type CreateOrderRequest struct {
	PaymentMethod PaymentMethod `json:"paymentMethod" binding:"required,oneof=pix boleto card"`
	Items         []OrderItem   `json:"items" binding:"required,min=1,dive"`
	ShippingCost  float64       `json:"shippingCost" binding:"gte=0"`
	Address       OrderAddress  `json:"address" binding:"required"`
}
