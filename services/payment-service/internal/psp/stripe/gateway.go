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

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/paymentintent"
	"github.com/stripe/stripe-go/v86/refund"
	"github.com/stripe/stripe-go/v86/webhook"
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
		// CARTÃO = só cartão. O Utilar TEM o seu próprio seletor de método (as abas
		// Pix/Boleto/Cartão), então o PaymentIntent do fluxo de cartão precisa ser
		// card-only — igual pix→["pix"] e boleto→["boleto"] logo abaixo.
		//
		// Antes usávamos AutomaticPaymentMethods (Stripe oferece TUDO que a conta tem
		// habilitado), e o PaymentElement renderizava o SEU próprio seletor com abas
		// Cartão/Boleto POR CIMA das abas do Utilar — o cliente via "Boleto" em dois
		// lugares e uma tela dentro da outra. Escopar em ["card"] faz o Element
		// mostrar só o formulário de cartão, sem seletor duplicado.
		//
		// Trade-off consciente: carteiras (Apple/Google Pay/Link) exigem
		// AutomaticPaymentMethods e ficam de fora por ora — entram depois, com uma
		// decisão de como encaixam nas abas, não de esgueirar um 2º seletor.
		params.PaymentMethodTypes = stripe.StringSlice([]string{"card"})
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
		return nil, classifyCreateError(err)
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

// clientFixableStripeCodes — códigos de erro da Stripe que são culpa do INPUT do
// cliente (CPF/cartão/valor), não da nossa infra. Viram 400 com mensagem
// acionável em vez de 502 "payment gateway error". O caso que motivou isto: um
// CPF inválido no boleto (tax_id_invalid) devolvia "payment gateway error"
// genérico e o cliente não sabia que era o CPF. Só os CÓDIGOS (enums estáveis)
// entram na mensagem — nunca o texto cru do erro, que pode ecoar PII (ver a nota
// AV1-H5 no handler).
var clientFixableStripeCodes = map[string]bool{
	"tax_id_invalid":       true, // CPF/CNPJ do boleto não passa no dígito verificador
	"card_declined":        true,
	"expired_card":         true,
	"incorrect_cvc":        true,
	"incorrect_number":     true,
	"invalid_number":       true,
	"invalid_expiry_month": true,
	"invalid_expiry_year":  true,
	"incorrect_zip":        true,
	"postal_code_invalid":  true,
	"amount_too_small":     true,
	"amount_too_large":     true,
}

// classifyCreateError separa erro de INPUT do cliente (→ ErrInvalidRequest, 400)
// de falha real do gateway (→ ErrUpstream, 502). Sem isso, tudo virava 502 e o
// comprador via "payment gateway error" mesmo quando o problema era o CPF dele.
func classifyCreateError(err error) error {
	var se *stripe.Error
	if errors.As(err, &se) && clientFixableStripeCodes[string(se.Code)] {
		// Passa só o code (estável, sem PII); clientSafePSPMessage traduz.
		return fmt.Errorf("%w: stripe_%s", psp.ErrInvalidRequest, se.Code)
	}
	return fmt.Errorf("%w: %v", psp.ErrUpstream, err)
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
// Refund estorna um PaymentIntent no Stripe (pspID = pi_...). SÍNCRONO: o Stripe
// devolve o refund já "succeeded" na maioria dos métodos (cartão), então
// normalizamos para "refunded"; se vier "pending" (alguns métodos), é "requested".
// É o caminho testável em DEV (PSP=stripe), enquanto a Appmax não entra.
func (g *Gateway) Refund(ctx context.Context, req psp.RefundRequest) (*psp.RefundResult, error) {
	params := &stripe.RefundParams{PaymentIntent: stripe.String(req.PSPID)}
	params.Context = ctx
	if !req.Total {
		if req.AmountCents <= 0 {
			return nil, fmt.Errorf("%w: estorno parcial exige AmountCents>0", psp.ErrInvalidRequest)
		}
		params.Amount = stripe.Int64(req.AmountCents)
	}
	r, err := refund.New(params)
	if err != nil {
		return nil, fmt.Errorf("%w: stripe refund: %v", psp.ErrUpstream, err)
	}
	status := psp.RefundDone
	if r.Status == stripe.RefundStatusPending {
		status = psp.RefundRequested
	}
	raw, _ := json.Marshal(r)
	return &psp.RefundResult{PSPRefundID: r.ID, Status: status, RawPayload: raw}, nil
}

func (g *Gateway) VerifyWebhook(body []byte, headers http.Header) error {
	if g.webhookSecret == "" {
		return nil
	}
	sig := headers.Get("Stripe-Signature")
	if sig == "" {
		return psp.ErrInvalidSignature
	}
	// ConstructEvent ESTRITO: valida assinatura (HMAC-SHA256, janela de 5min) E a
	// versão de API. Isto voltou a ser seguro depois de subir a stripe-go pra v86,
	// que embute a mesma versão que a conta emite (stripe.APIVersion =
	// "2026-08-26.dahlia"). Antes, na v79 (API 2024-06-20), a versão divergia da
	// conta e o ConstructEvent estrito rejeitava a assinatura VÁLIDA → webhook 401
	// → pagamento não confirmava; por isso usávamos ConstructEventWithOptions com
	// IgnoreAPIVersionMismatch. Se um dia a conta for movida pra uma versão MAIS
	// NOVA que a embutida no SDK, isto volta a dar 401 — e o conserto certo é subir
	// o SDK, não reafrouxar a checagem. Ver TestRegression_VerifyWebhook_*.
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
