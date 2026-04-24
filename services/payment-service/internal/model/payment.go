package model

import (
	"encoding/json"
	"time"
)

type PaymentMethod string
type PaymentStatus string

const (
	MethodPix    PaymentMethod = "pix"
	MethodBoleto PaymentMethod = "boleto"
	MethodCard   PaymentMethod = "card"

	StatusPending   PaymentStatus = "pending"
	StatusConfirmed PaymentStatus = "confirmed"
	StatusFailed    PaymentStatus = "failed"
	StatusExpired   PaymentStatus = "expired"
	StatusCancelled PaymentStatus = "cancelled"
)

type Payment struct {
	ID           string          `json:"id" db:"id"`
	OrderID      string          `json:"order_id" db:"order_id"`
	UserID       string          `json:"user_id" db:"user_id"`
	Method       PaymentMethod   `json:"method" db:"method"`
	Status       PaymentStatus   `json:"status" db:"status"`
	Amount       float64         `json:"amount" db:"amount"`
	Currency     string          `json:"currency" db:"currency"`
	PSPPaymentID *string         `json:"psp_payment_id,omitempty" db:"psp_payment_id"`
	PSPMetadata  json.RawMessage `json:"psp_metadata,omitempty" db:"psp_metadata"`
	PSPPayload   json.RawMessage `json:"psp_payload,omitempty" db:"psp_payload"`
	ConfirmedAt  *time.Time      `json:"confirmed_at,omitempty" db:"confirmed_at"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

type CreatePaymentRequest struct {
	OrderID string        `json:"order_id" binding:"required,uuid"`
	Method  PaymentMethod `json:"method" binding:"required,oneof=pix boleto card"`
	Amount  float64       `json:"amount" binding:"required,gt=0"`

	// Boleto requires payer identification (MP rejects without CPF).
	// Pix and card accept these but don't require. Validação no handler.
	// NOTE: audit C1/C2 — essa info continua vindo do cliente em dev;
	// Sprint 8.5 troca por propagação JWT → auth-service para evitar tamper.
	PayerCPF  string `json:"payer_cpf,omitempty"`
	PayerName string `json:"payer_name,omitempty"`
}

// PSPPayload is the response sent back to the SPA per payment method
type PixPayload struct {
	QRCode     string    `json:"qr_code"`
	QRCodeBase64 string  `json:"qr_code_base64"`
	CopyPaste  string    `json:"copy_paste"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type BoletoPayload struct {
	BarCode    string    `json:"bar_code"`
	PDF        string    `json:"pdf_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type CardPayload struct {
	InitPoint string `json:"init_point"` // MP hosted checkout URL
}
