// Package stripe implementa psp.Gateway usando Stripe PaymentIntents API.
//
// Fluxo para CARTÃO (Stripe Elements no frontend):
//  1. Backend CreatePayment → cria PaymentIntent com amount+currency=brl+payment_method_types=[card]
//  2. Retorna client_secret pro frontend
//  3. Frontend usa stripe.confirmPayment(clientSecret) pra coletar PAN/CVV dentro do Elements iframe
//     (PCI scope = SAQ-A — Stripe renderiza os campos sensíveis)
//  4. Stripe processa → PaymentIntent muda pra "succeeded" (ou requires_action em 3DS)
//  5. Webhook payment_intent.succeeded chega no backend → atualizamos DB
//
// Fluxo para PIX:
//  1. Backend CreatePayment → PaymentIntent com payment_method_types=[pix]
//  2. Stripe retorna PaymentIntent com next_action.pix_display_qr_code (QR code + copy_paste)
//  3. Frontend renderiza QR inline
//  4. Usuário paga via app bancário → Stripe confirma em 5-30s
//  5. Webhook payment_intent.succeeded → atualiza DB
//
// Fluxo para BOLETO:
//  1. Backend CreatePayment → PaymentIntent com payment_method_types=[boleto]
//     + payment_method_data.boleto.tax_id (CPF)
//  2. Stripe retorna next_action.boleto_display_details (pdf, barcode, hosted_voucher_url)
//  3. Usuário paga boleto em 1-3 dias úteis
//  4. Webhook payment_intent.succeeded quando compensado
package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/paymentintent"
	"github.com/stripe/stripe-go/v79/webhook"
	"github.com/utilar/payment-service/internal/psp"
)

// Gateway implementa psp.Gateway usando Stripe.
type Gateway struct {
	secretKey     string
	webhookSecret string
}

// New cria um Gateway Stripe.
// secretKey: sk_test_... ou sk_live_...
// webhookSecret: whsec_... gerado pelo `stripe listen` ou pelo dashboard webhook endpoint.
//
// Passar webhookSecret="" em dev desativa validação (não fazer em prod — fail-closed
// entra na Sprint 8.5).
func New(secretKey, webhookSecret string) *Gateway {
	// Configura globalmente o SDK (OK em single-tenant — um process tem 1 só Stripe account)
	stripe.Key = secretKey
	return &Gateway{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
	}
}

func (g *Gateway) Name() string { return "stripe" }

// CreatePayment cria um PaymentIntent apropriado pro método.
func (g *Gateway) CreatePayment(ctx context.Context, req psp.CreateRequest) (*psp.CreateResult, error) {
	// Stripe trabalha com centavos (int64). Converte reais → centavos.
	amountCents := int64(math.Round(req.Amount * 100))
	if amountCents <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", psp.ErrInvalidRequest)
	}

	currency := "brl"
	if req.Currency != "" && req.Currency != "BRL" {
		currency = req.Currency
	}

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountCents),
		Currency: stripe.String(currency),
		Metadata: map[string]string{
			"order_id": req.OrderID,
			"user_id":  req.UserID,
		},
	}

	// IdempotencyKey — Stripe suporta nativo via header (embutido em stripe.Params).
	if req.IdempotencyKey != "" {
		params.Params.IdempotencyKey = stripe.String(req.IdempotencyKey)
	}

	switch req.Method {
	case psp.MethodCard:
		// PaymentElement na SPA vai lidar com o card input. AutomaticPaymentMethods
		// permite Stripe oferecer o que estiver habilitado na conta (card + outros).
		// Em dev com Elements, é o caminho mais flexível.
		params.AutomaticPaymentMethods = &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled:        stripe.Bool(true),
			AllowRedirects: stripe.String("never"),
		}
		if req.PayerEmail != "" {
			params.ReceiptEmail = stripe.String(req.PayerEmail)
		}

	case psp.MethodPix:
		params.PaymentMethodTypes = stripe.StringSlice([]string{"pix"})
		if req.PayerEmail != "" {
			params.ReceiptEmail = stripe.String(req.PayerEmail)
		}
		// Stripe confirma Pix no server via PaymentIntent confirm com método inline
		params.Confirm = stripe.Bool(true)
		params.PaymentMethodData = &stripe.PaymentIntentPaymentMethodDataParams{
			Type: stripe.String("pix"),
		}

	case psp.MethodBoleto:
		if req.PayerCPF == "" || req.PayerName == "" || req.PayerEmail == "" {
			return nil, fmt.Errorf("%w: boleto requires payer_cpf, payer_name, payer_email", psp.ErrInvalidRequest)
		}
		params.PaymentMethodTypes = stripe.StringSlice([]string{"boleto"})
		params.Confirm = stripe.Bool(true)
		params.PaymentMethodData = &stripe.PaymentIntentPaymentMethodDataParams{
			Type: stripe.String("boleto"),
			Boleto: &stripe.PaymentMethodBoletoParams{
				TaxID: stripe.String(req.PayerCPF),
			},
			BillingDetails: &stripe.PaymentIntentPaymentMethodDataBillingDetailsParams{
				Email: stripe.String(req.PayerEmail),
				Name:  stripe.String(req.PayerName),
				Address: &stripe.AddressParams{
					Country:    stripe.String("BR"),
					Line1:      stripe.String("Endereço não informado"), // TODO: auth-service provee
					PostalCode: stripe.String("00000000"),
					City:       stripe.String("São Paulo"),
					State:      stripe.String("SP"),
				},
			},
		}

	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	// #nosec G117 — `pi` inclui client_secret, que é deliberadamente público
	// (expira ao confirmar, escopado a um único PaymentIntent, projetado pra
	// uso em browser via stripe.confirmPayment). RawPayload passa por
	// redactPSPPayload antes do INSERT em psp_payload (M2).
	raw, _ := json.Marshal(pi)

	return &psp.CreateResult{
		PSPID:        pi.ID,
		Status:       normalizeStatus(string(pi.Status)),
		ClientSecret: pi.ClientSecret,
		ClientData:   extractClientData(pi),
		RawPayload:   raw,
	}, nil
}

// GetPayment consulta um PaymentIntent no Stripe.
func (g *Gateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	pi, err := paymentintent.Get(pspID, nil)
	if err != nil {
		var stripeErr *stripe.Error
		if errors.As(err, &stripeErr) && stripeErr.HTTPStatusCode == http.StatusNotFound {
			return nil, psp.ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}
	// #nosec G117 — `pi` (PaymentIntent) inclui client_secret, que é
	// deliberadamente público (vide nota acima). RawPayload passa por
	// redactPSPPayload antes do INSERT em psp_payload (M2).
	raw, _ := json.Marshal(pi)
	return &psp.GetResult{
		PSPID:      pi.ID,
		Status:     normalizeStatus(string(pi.Status)),
		Amount:     float64(pi.Amount) / 100, // centavos → reais
		Currency:   string(pi.Currency),
		RawPayload: raw,
	}, nil
}

// VerifyWebhook valida Stripe-Signature usando o SDK oficial.
// A assinatura Stripe é `t=TIMESTAMP,v1=HEX` sobre `TIMESTAMP + "." + body`
// com HMAC-SHA256. O SDK webhook.ConstructEvent valida timestamp (janela 5min)
// e assinatura em uma call só.
//
// Em dev com webhookSecret="", pulamos validação (ver audit C5).
func (g *Gateway) VerifyWebhook(body []byte, headers http.Header) error {
	if g.webhookSecret == "" {
		return nil
	}
	sig := headers.Get("Stripe-Signature")
	if sig == "" {
		return psp.ErrInvalidSignature
	}
	_, err := webhook.ConstructEvent(body, sig, g.webhookSecret)
	if err != nil {
		return fmt.Errorf("%w: %v", psp.ErrInvalidSignature, err)
	}
	return nil
}

// ParseWebhookEvent extrai o evento Stripe normalizado.
// Eventos relevantes: payment_intent.succeeded, .payment_failed, .canceled.
func (g *Gateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	var event stripe.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrInvalidRequest, err)
	}

	// Só processamos eventos de payment_intent
	if event.Type == "" || len(event.Type) < len("payment_intent.") ||
		event.Type[:len("payment_intent.")] != "payment_intent." {
		return nil, nil // irrelevante — handler responde 200
	}

	// Event.Data.Object contém o PaymentIntent serializado
	var pi stripe.PaymentIntent
	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrInvalidRequest, err)
	}

	return &psp.WebhookEvent{
		EventType: string(event.Type),
		PSPID:     pi.ID,
		Status:    normalizeStatus(string(pi.Status)),
		Amount:    float64(pi.Amount) / 100,
		RawBody:   body,
	}, nil
}

// -- helpers ----------------------------------------------------------------

func normalizeStatus(stripeStatus string) psp.PaymentStatus {
	switch stripeStatus {
	case "succeeded":
		return psp.StatusApproved
	case "processing", "requires_payment_method", "requires_confirmation", "requires_action":
		return psp.StatusPending
	case "requires_capture":
		return psp.StatusAuthorized
	case "canceled":
		return psp.StatusCancelled
	default:
		return psp.StatusPending
	}
}

// extractClientData extrai dados do PaymentIntent que o frontend vai usar.
// Para Pix: next_action.pix_display_qr_code.
// Para Boleto: next_action.boleto_display_details.
// Para Card: só client_secret (que vai em field separado).
func extractClientData(pi *stripe.PaymentIntent) json.RawMessage {
	if pi.NextAction == nil {
		b, _ := json.Marshal(map[string]any{"type": "card", "client_secret": pi.ClientSecret})
		return b
	}

	data := map[string]any{
		"type": string(pi.NextAction.Type),
	}

	// NextAction é oneof — usamos raw dele pra ficar simples
	rawNext, _ := json.Marshal(pi.NextAction)
	data["next_action"] = json.RawMessage(rawNext)
	data["client_secret"] = pi.ClientSecret

	b, _ := json.Marshal(data)
	return b
}

var _ psp.Gateway = (*Gateway)(nil)
