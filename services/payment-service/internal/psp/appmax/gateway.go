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

// Gateway é a implementação Appmax (v3) do psp.Gateway.
type Gateway struct {
	client *Client
	// webhookSecret é opcional. A Appmax NÃO assina postbacks (sem HMAC), então a
	// segurança primária vem da re-consulta via GetPayment (o handler compara o
	// amount autoritativo do PSP com o nosso DB). Se um secret for configurado,
	// exigimos que o header X-Appmax-Token bata — camada extra.
	webhookSecret string
}

// New cria um Gateway Appmax v3. O base URL vem de APPMAX_BASE_URL (env) —
// aponte pro sandbox (homolog.sandboxappmax.com.br/api/v3) em dev.
func New(accessToken, webhookSecret string) *Gateway {
	return &Gateway{
		client:        NewClient(accessToken),
		webhookSecret: cleanEnv(webhookSecret),
	}
}

func (g *Gateway) Name() string { return "appmax" }

// CreatePayment orquestra o fluxo v3: customer → order → payment. Devolve o
// display cru (QR Pix / PDF boleto) em ClientData e usa o ID do PEDIDO Appmax
// como PSPID — é ele que GetPayment e o webhook usam pra reconciliar.
func (g *Gateway) CreatePayment(ctx context.Context, req psp.CreateRequest) (*psp.CreateResult, error) {
	// 1. Customer (CPF vai no pagamento, não aqui — convenção v3).
	first, last := splitName(req.PayerName)
	customerID, _, err := g.client.CreateCustomer(ctx, CustomerInput{
		FirstName: first,
		LastName:  last,
		Email:     req.PayerEmail,
		Telephone: digitsOnly(req.PayerPhone), // celular obrigatório (validado ao vivo)
		// TODO(appmax): endereço ainda pendente — a Appmax exige p/ boleto e
		// recomenda p/ antifraude. Ver docs/appmax-integration.md.
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create customer: %v", psp.ErrUpstream, err)
	}

	// 2. Order — um único line item sintético a partir do amount autoritativo
	// (o handler já derivou req.Amount do order-service, audit C1). Reais.
	orderID, _, err := g.client.CreateOrder(ctx, OrderInput{
		Total:      req.Amount,
		CustomerID: customerID,
		Products: []OrderProduct{{
			SKU:   "UTILAR-" + shortRef(req.OrderID),
			Name:  "Pedido UtiLar Ferragem",
			Qty:   1,
			Price: req.Amount,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create order: %v", psp.ErrUpstream, err)
	}

	// 3. Payment — roteia pelo método.
	var od *OrderData
	cpf := digitsOnly(req.PayerCPF)
	switch req.Method {
	case psp.MethodPix:
		od, err = g.client.PayPix(ctx, orderID, customerID, cpf)
	case psp.MethodBoleto:
		if cpf == "" || req.PayerName == "" {
			return nil, fmt.Errorf("%w: boleto requires payer_cpf and payer_name", psp.ErrInvalidRequest)
		}
		od, err = g.client.PayBoleto(ctx, orderID, customerID, cpf)
	case psp.MethodCard:
		// O cartão é tokenizado no browser (Appmax JS) — nunca trafegamos PAN pelo
		// backend (PCI SAQ-A). O token chega via CardToken.
		if req.CardToken == "" {
			return nil, fmt.Errorf("%w: card via appmax requires a tokenized card (CardToken); frontend tokenization pending", psp.ErrInvalidRequest)
		}
		od, err = g.client.PayCard(ctx, orderID, customerID, CardInput{
			Token:          req.CardToken,
			DocumentNumber: cpf,
			Installments:   1,
			SoftDescriptor: "UTILAR",
		})
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	status := psp.StatusPending
	if od.Status != "" {
		status = normalizeStatus(od.Status)
	}

	return &psp.CreateResult{
		PSPID:      strconv.FormatInt(orderID, 10),
		Status:     status,
		ClientData: clientData(od),
		RawPayload: od.Raw,
	}, nil
}

// clientData normaliza o display de pagamento pro frontend (QR Pix / boleto).
func clientData(od *OrderData) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"provider":       "appmax",
		"pix_qrcode":     od.PixQrCode,
		"pix_emv":        od.PixEmv,
		"pix_expires_at": od.PixExpiresAt,
		"boleto_url":     od.BoletoURL,
		"boleto_line":    od.BoletoLine,
	})
	return b
}

// GetPayment consulta o pedido Appmax (pspID == id do pedido) e devolve status +
// total autoritativos. Pilar da validação anti-fraude do webhook (audit C3).
func (g *Gateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	od, err := g.client.GetOrder(ctx, pspID)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, psp.ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}
	return &psp.GetResult{
		PSPID:      pspID,
		Status:     normalizeStatus(od.Status),
		Amount:     od.Total,
		Currency:   "BRL",
		RawPayload: od.Raw,
	}, nil
}

// VerifyWebhook — a Appmax não assina postbacks. A integridade real vem da
// re-consulta via GetPayment no handler (audit C3). Se um secret compartilhado
// estiver configurado, exigimos o header X-Appmax-Token (defesa em profundidade).
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

// ParseWebhookEvent extrai o evento normalizado do postback Appmax v3.
// Payload: {"environment":"...","event":"OrderApproved","data":{...}}.
// Tolera os DOIS formatos: DefaultResponse (data.id/data.status) e TwoLevel
// (data.order_id/data.order_status), além de data.order.* aninhado. Eventos em
// PascalCase (OrderPaid) ou snake_case (order_paid).
func (g *Gateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID          json.Number `json:"id"`
			OrderID     json.Number `json:"order_id"`
			Status      string      `json:"status"`
			OrderStatus string      `json:"order_status"`
			Total       float64     `json:"total"`
			Order       *struct {
				ID     json.Number `json:"id"`
				Status string      `json:"status"`
				Total  float64     `json:"total"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrInvalidRequest, err)
	}

	orderID := firstNonEmpty(payload.Data.ID.String(), payload.Data.OrderID.String())
	if orderID == "" && payload.Data.Order != nil {
		orderID = payload.Data.Order.ID.String()
	}
	if orderID == "" || orderID == "0" {
		return nil, nil // evento informativo (customer_*, sem order id) — Ack 200.
	}

	orderStatus := firstNonEmpty(payload.Data.Status, payload.Data.OrderStatus)
	if orderStatus == "" && payload.Data.Order != nil {
		orderStatus = payload.Data.Order.Status
	}

	event := normEvent(payload.Event)
	status := statusFromEvent(event)
	if status == psp.StatusPending && orderStatus != "" {
		// evento não conclusivo → cai pro status do pedido.
		status = normalizeStatus(orderStatus)
	}

	return &psp.WebhookEvent{
		EventType: event,
		PSPID:     orderID,
		Status:    status,
		RawBody:   body,
	}, nil
}

// -- helpers ----------------------------------------------------------------

// normalizeStatus mapeia o status de PEDIDO da Appmax v3 → vocabulário normalizado.
// Ref: docs.appmax.com.br/status + gifthy appmax-integration.md.
func normalizeStatus(s string) psp.PaymentStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "aprovado", "integrado", "pendente_integracao", "pago", "paid":
		return psp.StatusApproved
	case "autorizado", "authorized":
		return psp.StatusAuthorized
	case "cancelado", "estornado", "reembolsado", "chargeback_perdido":
		return psp.StatusCancelled
	case "recusado_por_risco", "recusado", "rejeitado":
		return psp.StatusRejected
	case "expirado", "vencido":
		return psp.StatusExpired
	default:
		// pendente, análise antifraude, pendente_integracao_em_analise, disputas.
		return psp.StatusPending
	}
}

// normEvent normaliza o nome do evento (case/separador-insensitive):
// "OrderPaid", "order_paid", "ORDER-PAID" → "orderpaid".
func normEvent(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// statusFromEvent mapeia o evento Appmax (já normalizado por normEvent) → status.
func statusFromEvent(event string) psp.PaymentStatus {
	switch event {
	case "orderapproved", "orderpaid", "orderpaidbypix", "orderintegrated":
		return psp.StatusApproved
	case "orderauthorized", "orderauthorizedwithdelay":
		return psp.StatusAuthorized
	case "orderrefund":
		return psp.StatusCancelled
	case "orderpixexpired", "orderbilletoverdue", "paymentnotauthorized", "paymentnotauthorizedwithdelay":
		return psp.StatusRejected
	default:
		// orderpixcreated, orderbilletcreated, customercreated, etc. → aguardando.
		return psp.StatusPending
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" && v != "0" {
			return v
		}
	}
	return ""
}

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
