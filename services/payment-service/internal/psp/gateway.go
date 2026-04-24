// Package psp defines the Payment Service Provider abstraction.
//
// Supported providers: Stripe, Mercado Pago. Switched via PSP_PROVIDER env var.
//
// Design goals:
//   - Handlers never import a specific PSP — they use `psp.Gateway`.
//   - Each provider has its own package (internal/psp/stripe, internal/psp/mercadopago)
//     implementing Gateway.
//   - Cross-cutting concerns (PCI handling, idempotency, amount validation) live
//     in the handler layer using Gateway, so they apply to every provider.
//
// Webhook handling is provider-specific (different signature formats, event
// shapes). The Gateway exposes VerifyWebhook + ParseWebhookEvent so the handler
// stays PSP-agnostic.
package psp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// PaymentMethod is the high-level user-visible method.
// PSPs may have their own sub-divisions (e.g. Stripe has "card" with ~80 brands,
// "boleto", "pix") — the Gateway implementation translates.
type PaymentMethod string

const (
	MethodPix    PaymentMethod = "pix"
	MethodBoleto PaymentMethod = "boleto"
	MethodCard   PaymentMethod = "card"
)

// PaymentStatus is the normalized lifecycle across providers.
// Maps:
//   - Stripe PaymentIntent:   requires_payment_method/requires_confirmation/requires_action → pending
//                             processing → pending
//                             succeeded → approved
//                             canceled → cancelled
//                             requires_capture → authorized (raro em BR)
//   - Mercado Pago:           pending/in_process/in_mediation → pending
//                             approved/authorized → approved
//                             rejected/cancelled/refunded/charged_back → failed/cancelled
type PaymentStatus string

const (
	StatusPending    PaymentStatus = "pending"
	StatusApproved   PaymentStatus = "approved"
	StatusRejected   PaymentStatus = "rejected"
	StatusCancelled  PaymentStatus = "cancelled"
	StatusExpired    PaymentStatus = "expired"
	StatusAuthorized PaymentStatus = "authorized"
)

// CreateRequest is what the HTTP handler hands to the Gateway.
// Stripe and MP have different required fields — optional ones are here and
// ignored by PSPs that don't need them.
type CreateRequest struct {
	OrderID    string        // nosso UUID — vai como external_reference/metadata
	UserID     string        // JWT sub — vai como metadata (audit trail)
	Amount     float64       // em reais (convertemos pra centavos no PSP se necessário)
	Currency   string        // "BRL"
	Method     PaymentMethod // pix / boleto / card
	PayerEmail string
	PayerName  string // obrigatório pra boleto no MP
	PayerCPF   string // obrigatório pra boleto (ambos PSPs)

	// CardToken é opcional e só usado com Method=card em certos fluxos.
	// Stripe: client-side `stripe.confirmPayment` costuma ser suficiente
	// (retornamos client_secret e o frontend completa); esse campo existe
	// caso um dia façamos "charges server-side" com token gerado em browser.
	CardToken string

	// IdempotencyKey — passado pro PSP (Stripe suporta nativo; MP via X-Idempotency-Key).
	// Evita double-charge em retry de rede. Recomenda-se UUID.
	IdempotencyKey string
}

// CreateResult é o que a Gateway devolve ao handler após criar o pagamento.
type CreateResult struct {
	PSPID        string          // id do pagamento no PSP (pi_xxx no Stripe, <id> no MP)
	Status       PaymentStatus   // estado inicial normalizado
	ClientSecret string          // Stripe — opaco, front-end usa pra confirm
	ClientData   json.RawMessage // payload shape-specific (QR code, boleto pdf_url, etc)
	RawPayload   json.RawMessage // resposta crua do PSP — persistida em psp_payload pra debug
}

// GetResult é o estado atual de um pagamento consultado via GetPayment.
type GetResult struct {
	PSPID      string
	Status     PaymentStatus
	Amount     float64         // em reais (convertido do centavos do PSP)
	Currency   string
	RawPayload json.RawMessage // resposta crua — útil pra comparar com o que temos
}

// WebhookEvent é o evento extraído de um webhook payload, já normalizado.
// A verificação de assinatura (HMAC/JWK) é responsabilidade da Gateway.
type WebhookEvent struct {
	EventType string          // "payment.approved", "payment.failed", etc — normalizado
	PSPID     string          // id do pagamento referenciado
	Status    PaymentStatus   // estado que o evento indica
	Amount    float64         // algum PSPs mandam (MP manda; Stripe manda no object)
	RawBody   json.RawMessage // corpo cru pra persistir em webhook_events.raw_payload
}

// Errors normalizados que podem ser retornados por qualquer Gateway.
// O handler traduz para HTTP status (bad gateway, not found, etc).
var (
	ErrNotFound       = errors.New("psp: payment not found")
	ErrInvalidRequest = errors.New("psp: invalid request")
	ErrUpstream       = errors.New("psp: upstream error")
	ErrInvalidSignature = errors.New("psp: invalid webhook signature")
)

// Gateway is the contract every PSP implementation must satisfy.
type Gateway interface {
	// Name retorna o identificador canônico (ex: "stripe", "mercadopago").
	// Usado em logs e na tabela webhook_events.psp_id.
	Name() string

	// CreatePayment cria o pagamento no PSP. Não persiste no nosso DB — isso é
	// responsabilidade do handler (que orquestra: insert → CreatePayment → update).
	CreatePayment(ctx context.Context, req CreateRequest) (*CreateResult, error)

	// GetPayment consulta o estado atual no PSP. Útil para sync/polling.
	GetPayment(ctx context.Context, pspID string) (*GetResult, error)

	// VerifyWebhook valida a assinatura do webhook. Implementação é específica
	// por PSP (Stripe: Stripe-Signature header; MP: x-signature `ts=X,v1=Y`).
	// Retorna ErrInvalidSignature se falhar.
	VerifyWebhook(body []byte, headers http.Header) error

	// ParseWebhookEvent extrai o evento normalizado do payload cru, após
	// VerifyWebhook ter passado. Retorna (nil, nil) se o evento não for
	// relevante pra nós (ex: ping, teste) — handler skipa e responde 200.
	ParseWebhookEvent(body []byte) (*WebhookEvent, error)
}
