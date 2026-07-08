package appmax

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/utilar/payment-service/internal/psp"
)

// Gateway é a implementação Appmax do psp.Gateway.
type Gateway struct {
	client *Client
	// webhookSecret é opcional. A Appmax NÃO assina postbacks com HMAC, então a
	// segurança primária do webhook é a re-consulta via GetPayment (o handler
	// compara o amount autoritativo do PSP com o nosso DB). Se um secret for
	// configurado, exigimos que o header X-Appmax-Token bata — camada extra.
	webhookSecret string
}

// New cria um Gateway Appmax. webhookSecret pode ser vazio (validação por
// re-consulta apenas — ver nota em VerifyWebhook).
func New(accessToken, webhookSecret string) *Gateway {
	return &Gateway{
		client:        NewClient(accessToken),
		webhookSecret: webhookSecret,
	}
}

func (g *Gateway) Name() string { return "appmax" }

// CreatePayment orquestra o fluxo order-centric da Appmax: customer → order →
// payment. Retorna o payload cru da cobrança em ClientData (QR Pix, linha
// digitável do boleto, etc) e usa o ID do PEDIDO Appmax como PSPID — é ele que
// GetPayment e o webhook usam pra reconciliar.
func (g *Gateway) CreatePayment(ctx context.Context, req psp.CreateRequest) (*psp.CreateResult, error) {
	// 1. Customer
	firstName, lastName := splitName(req.PayerName)
	custRaw, err := g.client.CreateCustomer(ctx, map[string]any{
		"first_name":      firstName,
		"last_name":       lastName,
		"email":           req.PayerEmail,
		"document_number": digitsOnly(req.PayerCPF),
		// TODO(appmax): coletar telefone no checkout. A Appmax marca phone como
		// obrigatório; enquanto não coletamos, mandamos vazio e o antifraude pode
		// pedir revisão. Rastreado em docs/appmax-integration.md.
		"phone": "",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create customer: %v", psp.ErrUpstream, err)
	}
	customerID, err := extractNestedID(custRaw, "customer")
	if err != nil {
		return nil, fmt.Errorf("%w: parse customer id: %v", psp.ErrUpstream, err)
	}

	// 2. Order — um único line item sintético a partir do amount autoritativo.
	// (o handler já derivou req.Amount do order-service, audit C1). A Appmax
	// valida que products_value == soma dos itens.
	orderRaw, err := g.client.CreateOrder(ctx, map[string]any{
		"customer_id": customerID,
		"products": []map[string]any{{
			"sku":        "UTILAR-" + shortRef(req.OrderID),
			"name":       "Pedido UtiLar Ferragem",
			"quantity":   1,
			"unit_value": req.Amount,
			"type":       "physical",
		}},
		"products_value":     req.Amount,
		"shipping_value":     0,
		"discount_value":     0,
		"external_reference": req.OrderID, // nosso UUID pra reconciliar
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create order: %v", psp.ErrUpstream, err)
	}
	appmaxOrderID, err := extractNestedID(orderRaw, "order")
	if err != nil {
		return nil, fmt.Errorf("%w: parse order id: %v", psp.ErrUpstream, err)
	}

	// 3. Payment — roteia pelo método.
	var payRaw json.RawMessage
	switch req.Method {
	case psp.MethodPix:
		payRaw, err = g.client.PayPix(ctx, map[string]any{
			"order_id": appmaxOrderID,
			"payment_data": map[string]any{
				"pix": map[string]any{"document_number": digitsOnly(req.PayerCPF)},
			},
		})
	case psp.MethodBoleto:
		if req.PayerCPF == "" || req.PayerName == "" {
			return nil, fmt.Errorf("%w: boleto requires payer_cpf and payer_name", psp.ErrInvalidRequest)
		}
		payRaw, err = g.client.PayBoleto(ctx, map[string]any{
			"order_id": appmaxOrderID,
			"payment_data": map[string]any{
				"boleto": map[string]any{"document_number": digitsOnly(req.PayerCPF)},
			},
		})
	case psp.MethodCard:
		// A Appmax exige o cartão tokenizado (Appmax.js no browser) — nunca
		// trafegamos PAN pelo backend (PCI SAQ-A). O frontend precisa mandar o
		// token via CardToken. Integração de tokenização do SPA pendente.
		if req.CardToken == "" {
			return nil, fmt.Errorf("%w: card via appmax requires a tokenized card (CardToken); frontend tokenization pending", psp.ErrInvalidRequest)
		}
		payRaw, err = g.client.PayCard(ctx, map[string]any{
			"order_id":    appmaxOrderID,
			"customer_id": customerID,
			"payment_data": map[string]any{
				"credit_card": map[string]any{
					"token":           req.CardToken,
					"installments":    1,
					"soft_descriptor": "UTILAR",
				},
			},
		})
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	// Status inicial: a cobrança recém-criada normalmente volta pendente
	// (Pix/boleto aguardam pagamento; cartão pode aprovar na hora). Preferimos o
	// status do pedido, se vier no payload.
	status := psp.StatusPending
	if s := extractOrderStatus(payRaw); s != "" {
		status = normalizeStatus(s)
	}

	return &psp.CreateResult{
		PSPID:      strconv.FormatInt(appmaxOrderID, 10),
		Status:     status,
		ClientData: payRaw, // QR Pix / linha digitável / etc — repassado ao SPA
		RawPayload: payRaw,
	}, nil
}

// GetPayment consulta o pedido Appmax (pspID == id do pedido) e devolve status +
// total autoritativos. É o pilar da validação anti-fraude do webhook (audit C3).
func (g *Gateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	raw, err := g.client.GetOrder(ctx, pspID)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, psp.ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	order, err := unwrapOrder(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	return &psp.GetResult{
		PSPID:      pspID,
		Status:     normalizeStatus(order.Status),
		Amount:     order.total(),
		Currency:   "BRL",
		RawPayload: raw,
	}, nil
}

// VerifyWebhook — a Appmax não assina postbacks (sem HMAC). A garantia real de
// integridade vem da re-consulta via GetPayment no handler, que compara o amount
// autoritativo do PSP contra o nosso DB antes de confirmar qualquer pagamento
// (audit C3). Se um secret compartilhado estiver configurado, exigimos que o
// header X-Appmax-Token bata (defesa em profundidade). Sem secret, aceitamos e
// deixamos a re-consulta ser o gate.
//
// Ver docs/appmax-integration.md para o modelo de confiança completo.
func (g *Gateway) VerifyWebhook(body []byte, headers http.Header) error {
	if g.webhookSecret == "" {
		return nil
	}
	token := headers.Get("X-Appmax-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(g.webhookSecret)) != 1 {
		return fmt.Errorf("%w: x-appmax-token mismatch", psp.ErrInvalidSignature)
	}
	return nil
}

// ParseWebhookEvent extrai o evento normalizado do postback Appmax.
// Payload esperado: {"environment":"...","event":"order_paid","data":{"id":123,"status":"..."}}.
// Aceita event em snake_case (order_paid) ou CamelCase (OrderPaid).
func (g *Gateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID     json.Number `json:"id"`
			Status string      `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrInvalidRequest, err)
	}

	orderID := payload.Data.ID.String()
	if orderID == "" || orderID == "0" {
		return nil, nil // sem order id → irrelevante (ex: ping)
	}

	event := normalizeEventName(payload.Event)
	return &psp.WebhookEvent{
		EventType: event,
		PSPID:     orderID,
		Status:    statusFromEvent(event),
		RawBody:   body,
	}, nil
}

// -- helpers ----------------------------------------------------------------

// normalizeStatus mapeia o status de PEDIDO da Appmax pro vocabulário normalizado.
func normalizeStatus(s string) psp.PaymentStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "aprovado", "pago", "paid", "approved", "integrado", "integrated":
		return psp.StatusApproved
	case "autorizado", "authorized":
		return psp.StatusAuthorized
	case "cancelado", "cancelled", "canceled", "estornado", "reembolsado", "refunded", "chargeback":
		return psp.StatusCancelled
	case "expirado", "vencido", "expired", "overdue":
		return psp.StatusExpired
	case "recusado", "rejeitado", "rejected", "não autorizado", "nao autorizado":
		return psp.StatusRejected
	default:
		return psp.StatusPending
	}
}

// normalizeEventName lowercases e converte CamelCase (OrderPaid) em snake_case
// (order_paid) pra comparação estável.
func normalizeEventName(e string) string {
	e = strings.TrimSpace(e)
	if e == "" {
		return ""
	}
	// Se já tem underscore, só lowercase.
	if strings.Contains(e, "_") {
		return strings.ToLower(e)
	}
	var b strings.Builder
	for i, r := range e {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// statusFromEvent mapeia o nome do evento Appmax pro status normalizado.
func statusFromEvent(event string) psp.PaymentStatus {
	switch event {
	case "order_approved", "order_paid", "order_paid_by_pix", "order_integrated":
		return psp.StatusApproved
	case "order_authorized", "order_authorized_with_delay":
		return psp.StatusAuthorized
	case "order_refund", "order_chargeback_in_treatment":
		return psp.StatusCancelled
	case "order_pix_expired", "order_billet_overdue":
		return psp.StatusExpired
	case "payment_not_authorized":
		return psp.StatusRejected
	default:
		// order_pix_created, order_billet_created, order_pix_updated, etc → aguardando.
		return psp.StatusPending
	}
}

func splitName(full string) (first, last string) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", ""
	}
	parts := strings.Fields(full)
	if len(parts) == 1 {
		return parts[0], ""
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

// shortRef pega os primeiros 8 chars de um UUID pra compor um SKU legível.
func shortRef(orderID string) string {
	orderID = strings.ReplaceAll(orderID, "-", "")
	if len(orderID) > 8 {
		return orderID[:8]
	}
	return orderID
}

// extractNestedID pega data.<key>.id de respostas {data:{customer|order:{id}}}.
// O id da Appmax é numérico.
func extractNestedID(raw json.RawMessage, key string) (int64, error) {
	var env struct {
		Data map[string]struct {
			ID json.Number `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, err
	}
	node, ok := env.Data[key]
	if !ok || node.ID.String() == "" {
		return 0, fmt.Errorf("missing data.%s.id in appmax response", key)
	}
	return node.ID.Int64()
}

// appmaxOrder é a visão que precisamos do objeto order da Appmax.
type appmaxOrder struct {
	ID            json.Number `json:"id"`
	Status        string      `json:"status"`
	Total         json.Number `json:"total"`
	ProductsValue json.Number `json:"products_value"`
}

func (o appmaxOrder) total() float64 {
	if v, err := o.Total.Float64(); err == nil && v > 0 {
		return v
	}
	if v, err := o.ProductsValue.Float64(); err == nil {
		return v
	}
	return 0
}

// unwrapOrder extrai o objeto order de {data:{order:{...}}}.
func unwrapOrder(raw json.RawMessage) (appmaxOrder, error) {
	var env struct {
		Data struct {
			Order appmaxOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return appmaxOrder{}, err
	}
	if env.Data.Order.ID.String() == "" {
		return appmaxOrder{}, fmt.Errorf("missing data.order in appmax response")
	}
	return env.Data.Order, nil
}

// extractOrderStatus tenta achar um status de pedido no payload de cobrança,
// que a Appmax às vezes aninha em data.order.status ou data.status.
func extractOrderStatus(raw json.RawMessage) string {
	var env struct {
		Data struct {
			Status string `json:"status"`
			Order  struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	if env.Data.Order.Status != "" {
		return env.Data.Order.Status
	}
	return env.Data.Status
}

// Compile-time assertion que Gateway implementa psp.Gateway.
var _ psp.Gateway = (*Gateway)(nil)
