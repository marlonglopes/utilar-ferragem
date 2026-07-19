package appmaxv1

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/utilar/payment-service/internal/psp"
)

// ProviderName é o identificador canônico deste provider (PSP_PROVIDER=appmax-v1).
// Também é o segmento usado na rota de webhook: POST /webhooks/appmax-v1.
const ProviderName = "appmax-v1"

// defaultCustomerIP é enviado quando o handler não propaga o IP do comprador.
// `ip` é OBRIGATÓRIO em POST /v1/customers — mandar vazio dá 422.
const defaultCustomerIP = "0.0.0.0"

// Gateway é a implementação psp.Gateway da Appmax AppStore API v1 (OAuth2).
type Gateway struct {
	client        *Client
	webhookSecret string
}

// New cria o Gateway a partir da Config (credenciais APPMAX_V1_*).
func New(cfg Config) *Gateway {
	return &Gateway{
		client:        NewClient(cfg),
		webhookSecret: cleanEnv(cfg.WebhookSecret),
	}
}

// Client expõe o client HTTP para os fluxos que não cabem na interface
// psp.Gateway (split, recebedores, saques, rastreio).
func (g *Gateway) Client() *Client { return g.client }

func (g *Gateway) Name() string { return ProviderName }

// CreatePayment orquestra o fluxo v1: customer → order → payment.
//
// Conversão de moeda: psp.CreateRequest.Amount vem em REAIS (float64) e a API v1
// é 100% em CENTAVOS (inteiros) — convertemos com arredondamento (ToCents).
//
// PSPID = id do PEDIDO Appmax. É por ele que GetPayment e o webhook reconciliam.
func (g *Gateway) CreatePayment(ctx context.Context, req psp.CreateRequest) (*psp.CreateResult, error) {
	if req.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount deve ser > 0", psp.ErrInvalidRequest)
	}
	totalCents := ToCents(req.Amount)

	// 0. Validação por método ANTES de qualquer chamada — sem isso criaríamos
	// customer/order órfãos na Appmax só pra falhar no passo do pagamento.
	cpf := digitsOnly(req.PayerCPF)
	switch req.Method {
	case psp.MethodPix:
		if cpf == "" {
			return nil, fmt.Errorf("%w: pix requires payer_cpf", psp.ErrInvalidRequest)
		}
	case psp.MethodBoleto:
		if cpf == "" || req.PayerName == "" {
			return nil, fmt.Errorf("%w: boleto requires payer_cpf and payer_name", psp.ErrInvalidRequest)
		}
	case psp.MethodCard:
		// PAN nunca passa pelo backend: o cartão é tokenizado no browser via
		// Appmax JS (PCI SAQ-A) e chega aqui só como token de uso único.
		if req.CardToken == "" {
			return nil, fmt.Errorf("%w: card via appmax-v1 requires a tokenized card (CardToken)", psp.ErrInvalidRequest)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}

	// 1. Customer — first_name/last_name/email/phone/ip são obrigatórios.
	first, last := splitName(req.PayerName)
	customerID, _, err := g.client.CreateCustomer(ctx, CustomerInput{
		FirstName:      first,
		LastName:       last,
		Email:          req.PayerEmail,
		Phone:          digitsOnly(req.PayerPhone),
		IP:             defaultCustomerIP,
		DocumentNumber: digitsOnly(req.PayerCPF),
	})
	if err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}

	// 2. Order — item sintético a partir do amount autoritativo (o handler já o
	// derivou do order-service, audit C1). A Appmax não calcula juros: mandamos
	// products_value/discount/shipping finais.
	orderID, _, err := g.client.CreateOrder(ctx, OrderInput{
		CustomerID:    customerID,
		ProductsValue: totalCents,
		DiscountValue: 0,
		ShippingValue: 0,
		Products: []OrderProduct{{
			SKU:       "UTILAR-" + shortRef(req.OrderID),
			Name:      "Pedido UtiLar Ferragem",
			Quantity:  1,
			UnitValue: totalCents,
			Type:      ProductTypePhysical,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	// 3. Payment (já validado no passo 0).
	var pr *PaymentResult
	switch req.Method {
	case psp.MethodPix:
		pr, err = g.client.PayPix(ctx, orderID, cpf)
	case psp.MethodBoleto:
		pr, err = g.client.PayBoleto(ctx, orderID, cpf)
	case psp.MethodCard:
		pr, err = g.client.PayCreditCard(ctx, orderID, customerID, CardChargeInput{
			Token:                req.CardToken,
			HolderDocumentNumber: cpf,
			HolderName:           req.PayerName,
			Installments:         1, // psp.CreateRequest ainda não carrega parcelas
			SoftDescriptor:       "UTILAR",
		})
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}
	if err != nil {
		return nil, err
	}

	status := psp.StatusPending
	if pr.Status != "" {
		status = NormalizeStatus(pr.Status)
	}
	installments := pr.Installments
	if installments <= 0 {
		installments = 1
	}

	return &psp.CreateResult{
		PSPID:      strconv.FormatInt(orderID, 10),
		Status:     status,
		ClientData: clientData(pr, installments),
		RawPayload: pr.Raw,
	}, nil
}

// clientData normaliza o display de pagamento pro SPA.
//
// pix_qrcode aqui é PNG em BASE64 sem o prefixo `data:` (é o que a API v1
// devolve). O front deve montar `data:image/png;base64,<valor>`. No WEBHOOK o
// campo homônimo é uma URL — ver Event.PixQRCodeURL.
func clientData(pr *PaymentResult, installments int) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"provider":       ProviderName,
		"pix_qrcode":     pr.PixQRCodeB64,
		"pix_emv":        pr.PixEMV,
		"pix_expires_at": pr.PixExpiresAt,
		"boleto_url":     pr.BoletoURL,
		"boleto_line":    pr.BoletoLine,
		"installments":   installments,
	})
	return b
}

// GetPayment consulta o pedido (pspID == id do pedido Appmax) e devolve status +
// valor autoritativos. É o pilar da validação do webhook não-assinado (audit C3).
func (g *Gateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	ov, err := g.client.GetOrder(ctx, pspID)
	if err != nil {
		if errors.Is(err, psp.ErrNotFound) {
			return nil, psp.ErrNotFound
		}
		return nil, err
	}
	return &psp.GetResult{
		PSPID:      pspID,
		Status:     NormalizeStatus(ov.Status),
		Amount:     FromCents(ov.TotalCents()), // centavos → reais
		Currency:   "BRL",
		RawPayload: ov.Raw,
	}, nil
}

// VerifyWebhook — a Appmax NÃO assina webhooks (sem HMAC, sem token no payload).
// A integridade real vem da re-consulta GET /v1/orders/{id} feita pelo handler
// (audit C3). Se APPMAX_WEBHOOK_SECRET estiver configurado, exigimos o header
// X-Appmax-Token igual (fail-closed opcional, defesa em profundidade).
func (g *Gateway) VerifyWebhook(_ []byte, headers http.Header) error {
	if g.webhookSecret == "" {
		return nil
	}
	token := headers.Get("X-Appmax-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(g.webhookSecret)) != 1 {
		return fmt.Errorf("%w: x-appmax-token mismatch", psp.ErrInvalidSignature)
	}
	return nil
}

// ParseWebhookEvent extrai o evento normalizado.
// Timeout de entrega da Appmax é 5s; retries em 0, +30min, +2h, +4h e depois
// descarta — responder 200 rápido é obrigatório.
func (g *Gateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	ev, err := ParseEvent(body)
	if err != nil {
		return nil, err
	}
	if ev.OrderID == "" || ev.OrderID == "0" {
		// customer_*, subscription_* sem order id → informativo, Ack 200.
		return nil, nil
	}
	status := statusFromEvent(ev.Event)
	if status == psp.StatusPending && ev.Status != "" {
		status = NormalizeStatus(ev.Status)
	}
	return &psp.WebhookEvent{
		EventType: normEvent(ev.Event),
		PSPID:     ev.OrderID,
		Status:    status,
		Amount:    FromCents(ev.TotalCents),
		RawBody:   body,
	}, nil
}

// ===================== Webhook =====================

// Event é o webhook da Appmax v1 já decomposto.
//
// Envelope: {event, event_type, site_id, app_id, client_key, external_key, data,
// partner_merchant}. `event_type` é o domínio: order|customer|payment|subscription.
type Event struct {
	Event       string // ex: "order_paid_by_pix" (snake_case)
	EventType   string // "order" | "customer" | "payment" | "subscription"
	SiteID      string
	AppID       string
	ClientKey   string
	ExternalKey string

	OrderID    string
	Status     string
	TotalCents int64
	FreightVal int64
	MerchTotal int64
	Discount   int64
	Interest   int64
	PaidAt     string

	Products []EventProduct

	// Método de pagamento — payment_info é um objeto de UMA chave só.
	PaymentMethod string // "credit_card" | "pix" | "boleto"

	// Cartão
	CardInstallments  int
	CardBrand         string
	CardNSU           string
	CardAuthorization string
	CardCapturedAt    string

	// Pix. ATENÇÃO: PixQRCodeURL vem do campo `pix_qrcode` do WEBHOOK, que é uma
	// **URL** — diferente do `pix_qrcode` da resposta de POST /v1/payments/pix,
	// que é PNG em **base64**. Mesmo nome, tipos diferentes.
	PixEndToEndID  string
	PixExpiration  string
	PixEMV         string
	PixQRCodeURL   string
	PixPaymentLink string

	// Boleto
	BoletoOverdueDate string
	BoletoURL         string
	BoletoLine        string

	// Cashback é READ-ONLY: só existe no webhook, não há endpoint de cashback na
	// API v1. Exposto aqui (e no RawBody) para o Utilar reconciliar.
	CashbackUsed     int64
	CashbackReserved int64
	CashbackStatus   string

	// PartnerMerchant é repassado cru — a doc não fixa o shape.
	PartnerMerchant json.RawMessage

	Raw json.RawMessage
}

// EventProduct é um item do pedido no payload de webhook.
type EventProduct struct {
	SKU      string
	Name     string
	Price    int64
	Quantity int
}

// ParseEvent decompõe o payload de webhook de forma tolerante.
func ParseEvent(body []byte) (*Event, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("%w: webhook json inválido: %v", psp.ErrInvalidRequest, err)
	}
	ev := &Event{
		Event:       str(m, "event"),
		EventType:   str(m, "event_type"),
		SiteID:      fmt.Sprint(orEmpty(m["site_id"])),
		AppID:       fmt.Sprint(orEmpty(m["app_id"])),
		ClientKey:   str(m, "client_key"),
		ExternalKey: str(m, "external_key"),
		Raw:         body,
	}
	if pm, ok := m["partner_merchant"]; ok {
		if b, err := json.Marshal(pm); err == nil {
			ev.PartnerMerchant = b
		}
	}

	data := mapAt(m, "data")
	if data == nil {
		return ev, nil
	}
	// Alguns eventos aninham o pedido em data.order.
	if order := mapAt(data, "order"); order != nil {
		for k, v := range order {
			if _, exists := data[k]; !exists {
				data[k] = v
			}
		}
	}

	if id := toInt64(firstOf(data, "order_id", "id")); id != 0 {
		ev.OrderID = strconv.FormatInt(id, 10)
	}
	ev.Status = str(data, "status", "order_status")
	ev.TotalCents = toInt64(firstOf(data, "total"))
	ev.FreightVal = toInt64(firstOf(data, "freight_value"))
	ev.MerchTotal = toInt64(firstOf(data, "merchant_total"))
	ev.Discount = toInt64(firstOf(data, "discount"))
	ev.Interest = toInt64(firstOf(data, "interest"))
	ev.PaidAt = str(data, "paid_at")
	ev.CashbackUsed = toInt64(firstOf(data, "cashback_used"))
	ev.CashbackReserved = toInt64(firstOf(data, "cashback_reserved"))
	ev.CashbackStatus = fmt.Sprint(orEmpty(data["cashback_status"]))

	if items, ok := data["products"].([]any); ok {
		for _, it := range items {
			p, ok := it.(map[string]any)
			if !ok {
				continue
			}
			ev.Products = append(ev.Products, EventProduct{
				SKU:      str(p, "sku"),
				Name:     str(p, "name"),
				Price:    toInt64(p["price"]),
				Quantity: toInt(p["quantity"]),
			})
		}
	}

	// payment_info é um objeto de UMA chave: credit_card | pix | boleto.
	if pi := mapAt(data, "payment_info"); pi != nil {
		switch {
		case mapAt(pi, "credit_card") != nil:
			cc := mapAt(pi, "credit_card")
			ev.PaymentMethod = "credit_card"
			ev.CardInstallments = toInt(cc["installments"])
			ev.CardBrand = str(cc, "card_brand")
			ev.CardNSU = fmt.Sprint(orEmpty(cc["nsu"]))
			ev.CardAuthorization = fmt.Sprint(orEmpty(cc["authorization_code"]))
			ev.CardCapturedAt = str(cc, "captured_at")
		case mapAt(pi, "pix") != nil:
			px := mapAt(pi, "pix")
			ev.PaymentMethod = "pix"
			ev.PixEndToEndID = str(px, "end_to_end_id")
			ev.PixExpiration = str(px, "pix_expiration_date")
			ev.PixEMV = str(px, "pix_emv")
			ev.PixQRCodeURL = str(px, "pix_qrcode") // URL no webhook (base64 na API!)
			ev.PixPaymentLink = str(px, "pix_payment_link")
		case mapAt(pi, "boleto") != nil:
			bo := mapAt(pi, "boleto")
			ev.PaymentMethod = "boleto"
			ev.BoletoOverdueDate = str(bo, "boleto_overdue_date", "due_date")
			ev.BoletoURL = str(bo, "boleto_url", "pdf")
			ev.BoletoLine = str(bo, "boleto_digitable_line", "digitable_line")
		}
	}
	return ev, nil
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// ===================== Status =====================

// NormalizeStatus mapeia o status de PEDIDO da Appmax → vocabulário normalizado.
// Os status são os mesmos da v3 (pendente, aprovado, autorizado, cancelado,
// estornado, recusado_por_risco, integrado, pendente_integracao,
// pendente_integracao_em_analise, chargeback_*).
func NormalizeStatus(s string) psp.PaymentStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "aprovado", "integrado", "pendente_integracao", "pago", "paid", "approved":
		return psp.StatusApproved
	case "autorizado", "authorized":
		return psp.StatusAuthorized
	case "cancelado", "estornado", "reembolsado", "chargeback_perdido", "cancelled", "refunded":
		return psp.StatusCancelled
	case "recusado_por_risco", "recusado", "rejeitado", "refused":
		return psp.StatusRejected
	case "expirado", "vencido", "chargeback_vencido":
		return psp.StatusExpired
	default:
		// pendente, pendente_integracao_em_analise, chargeback_em_tratativa,
		// chargeback_em_disputa → seguimos aguardando desfecho.
		return psp.StatusPending
	}
}

// normEvent normaliza o nome do evento (case/separador-insensitive):
// "order_paid", "OrderPaid", "ORDER-PAID" → "orderpaid".
func normEvent(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// statusFromEvent mapeia o evento de webhook → status normalizado.
// Aceita snake_case (formato v1) e PascalCase (formato v3) via normEvent.
func statusFromEvent(rawEvent string) psp.PaymentStatus {
	switch normEvent(rawEvent) {
	case "orderapproved", "orderpaid", "orderpaidbypix", "orderintegrated",
		"orderchargebackgain": // order_charge_back_gain: chargeback ganho → volta a valer
		return psp.StatusApproved
	case "orderauthorized", "paymentauthorizedwithdelay":
		return psp.StatusAuthorized
	case "orderrefund", "orderpartialrefund", "ordercanceled", "ordercancelled":
		return psp.StatusCancelled
	case "orderpixexpired", "orderbilletoverdue":
		return psp.StatusExpired
	case "orderrefusedbyrisk", "paymentnotauthorized", "orderchargebacklost":
		return psp.StatusRejected
	default:
		// order_pix_created, order_billet_created, order_chargeback_in_treatment,
		// split_orders, customer_* → sem desfecho, mantemos pendente e caímos no
		// status do pedido.
		return psp.StatusPending
	}
}

// ===================== helpers =====================

func splitName(full string) (first, last string) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "Cliente", "UtiLar"
	}
	parts := strings.Fields(full)
	if len(parts) == 1 {
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shortRef(orderID string) string {
	orderID = strings.ReplaceAll(orderID, "-", "")
	if len(orderID) > 8 {
		return orderID[:8]
	}
	return orderID
}

// Compile-time assertion que Gateway implementa psp.Gateway.
var _ psp.Gateway = (*Gateway)(nil)
