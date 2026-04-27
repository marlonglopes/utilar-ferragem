// Package mercadopago implementa psp.Gateway usando a API do Mercado Pago.
//
// Dois endpoints do MP são usados:
//   - POST /v1/payments        (Checkout API — pix, boleto, cartão direto)
//   - POST /checkout/preferences (Checkout Pro — fallback, hosted redirect)
//
// Em test mode (test users via /users/test_user), só /checkout/preferences
// funciona para todos os métodos. /v1/payments direto requer onboarding do
// merchant no dashboard MP — conhecemos a limitação, documentamos em
// docs/security/mp-integration-test-2026-04-24.md.
package mercadopago

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

// Gateway é a implementação MP do psp.Gateway.
type Gateway struct {
	client        *Client
	webhookSecret string
}

// New cria um Gateway Mercado Pago. webhookSecret pode ser vazio em dev
// (HMAC validation skipada — NÃO FAZER EM PROD, ver audit issue C5).
func New(accessToken, webhookSecret string) *Gateway {
	return &Gateway{
		client:        NewClient(accessToken),
		webhookSecret: webhookSecret,
	}
}

func (g *Gateway) Name() string { return "mercadopago" }

// CreatePayment roteia pro método MP correto.
// Implementação atual mantém o comportamento legado (Checkout API direto pra
// pix/boleto, Preference pra cartão). Limitação de sandbox conhecida.
func (g *Gateway) CreatePayment(ctx context.Context, req psp.CreateRequest) (*psp.CreateResult, error) {
	var raw json.RawMessage
	var err error

	switch req.Method {
	case psp.MethodPix:
		raw, err = g.client.CreatePixPayment(req.OrderID, req.Amount, req.PayerEmail)
	case psp.MethodBoleto:
		if req.PayerCPF == "" || req.PayerName == "" {
			return nil, fmt.Errorf("%w: boleto requires payer_cpf and payer_name", psp.ErrInvalidRequest)
		}
		raw, err = g.client.CreateBoleto(req.OrderID, req.Amount, req.PayerEmail, req.PayerCPF, req.PayerName)
	case psp.MethodCard:
		raw, err = g.client.CreatePreference(req.OrderID, req.Amount, "Pedido UtiLar Ferragem")
	default:
		return nil, fmt.Errorf("%w: unsupported method %q", psp.ErrInvalidRequest, req.Method)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	// Extrai pspID + status
	var mpResp struct {
		ID     any    `json:"id"` // MP pode mandar int ou string
		Status string `json:"status"`
	}
	_ = json.Unmarshal(raw, &mpResp)

	pspID := ""
	switch v := mpResp.ID.(type) {
	case string:
		pspID = v
	case float64:
		pspID = strconv.FormatInt(int64(v), 10)
	}

	return &psp.CreateResult{
		PSPID:      pspID,
		Status:     normalizeStatus(mpResp.Status),
		ClientData: raw, // MP devolve o payload todo (QR code, barcode, init_point)
		RawPayload: raw,
	}, nil
}

// GetPayment consulta um pagamento MP pelo pspID.
// Só funciona para pagamentos criados via /v1/payments (ids numéricos).
// Preferences usam /checkout/preferences/:id — não implementado aqui.
func (g *Gateway) GetPayment(ctx context.Context, pspID string) (*psp.GetResult, error) {
	raw, err := g.client.GetPayment(pspID)
	if err != nil {
		// MP retorna 404 como erro genérico — poderíamos parsear mais fino
		if strings.Contains(err.Error(), "404") {
			return nil, psp.ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", psp.ErrUpstream, err)
	}

	var mpResp struct {
		ID                any     `json:"id"`
		Status            string  `json:"status"`
		TransactionAmount float64 `json:"transaction_amount"`
		Currency          string  `json:"currency_id"`
	}
	_ = json.Unmarshal(raw, &mpResp)

	pspIDStr := pspID
	if v, ok := mpResp.ID.(float64); ok {
		pspIDStr = strconv.FormatInt(int64(v), 10)
	}

	return &psp.GetResult{
		PSPID:      pspIDStr,
		Status:     normalizeStatus(mpResp.Status),
		Amount:     mpResp.TransactionAmount,
		Currency:   mpResp.Currency,
		RawPayload: raw,
	}, nil
}

// maxClockSkew — janela de replay aceita pra timestamp do webhook.
// MP recomenda 5min. Webhooks com ts mais antigos são rejeitados como replay.
const maxClockSkew = 5 * time.Minute

// VerifyWebhook valida o header `x-signature` no formato oficial Mercado Pago
// Webhook V2 (audit C4):
//   - Parse `x-signature: ts=TIMESTAMP,v1=HASH`
//   - Manifest: `id:<data.id>;request-id:<x-request-id>;ts:<ts>;`
//   - HMAC-SHA256(manifest, secret), constant-time compare com v1
//   - Replay protection: rejeita ts com delta > maxClockSkew
//
// Em dev com webhookSecret="", pula validação (audit C5 — controle separado
// no config.Load fail-closed).
//
// Refs: https://www.mercadopago.com.br/developers/pt/docs/your-integrations/notifications/webhooks
func (g *Gateway) VerifyWebhook(body []byte, headers http.Header) error {
	if g.webhookSecret == "" {
		return nil
	}

	sig := headers.Get("X-Signature")
	if sig == "" {
		// Tenta lowercase também (Gin canonicaliza, mas defensivo)
		sig = headers.Get("x-signature")
	}
	if sig == "" {
		return fmt.Errorf("%w: missing x-signature header", psp.ErrInvalidSignature)
	}

	requestID := headers.Get("X-Request-Id")
	if requestID == "" {
		return fmt.Errorf("%w: missing x-request-id header", psp.ErrInvalidSignature)
	}

	ts, v1, err := parseMPSignatureHeader(sig)
	if err != nil {
		return fmt.Errorf("%w: %v", psp.ErrInvalidSignature, err)
	}

	// Replay protection
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: ts not numeric", psp.ErrInvalidSignature)
	}
	if delta := time.Since(time.UnixMilli(tsInt)); delta > maxClockSkew || delta < -maxClockSkew {
		return fmt.Errorf("%w: ts out of window (delta=%s)", psp.ErrInvalidSignature, delta)
	}

	dataID, err := extractDataID(body)
	if err != nil {
		return fmt.Errorf("%w: %v", psp.ErrInvalidSignature, err)
	}

	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, ts)
	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write([]byte(manifest))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return fmt.Errorf("%w: hmac mismatch", psp.ErrInvalidSignature)
	}
	return nil
}

// parseMPSignatureHeader parsa o header `ts=X,v1=Y` do Mercado Pago.
// Aceita campos em qualquer ordem e ignora campos desconhecidos.
func parseMPSignatureHeader(h string) (ts, v1 string, err error) {
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		switch key {
		case "ts":
			ts = val
		case "v1":
			v1 = val
		}
	}
	if ts == "" || v1 == "" {
		return "", "", fmt.Errorf("malformed signature header (missing ts/v1)")
	}
	return ts, v1, nil
}

// extractDataID extrai `data.id` do body do webhook MP. Suporta formato V2
// (`{"data":{"id":"..."}}`) e fallback legacy (`{"resource":"...","topic":"payment"}`)
// extraindo o ID do final da URL do resource.
func extractDataID(body []byte) (string, error) {
	var v2 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v2); err == nil && v2.Data.ID != "" {
		return v2.Data.ID, nil
	}

	var legacy struct {
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(body, &legacy); err == nil && legacy.Resource != "" {
		// Resource format: ".../payments/<id>"
		idx := strings.LastIndex(legacy.Resource, "/")
		if idx >= 0 && idx+1 < len(legacy.Resource) {
			return legacy.Resource[idx+1:], nil
		}
	}

	return "", fmt.Errorf("could not extract data.id from body")
}

// ParseWebhookEvent extrai o evento normalizado do payload MP.
func (g *Gateway) ParseWebhookEvent(body []byte) (*psp.WebhookEvent, error) {
	var payload struct {
		ID     int64  `json:"id"`
		Type   string `json:"type"`
		Action string `json:"action"`
		Data   struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", psp.ErrInvalidRequest, err)
	}

	// Só processamos eventos de pagamento
	if payload.Type != "payment" || payload.Data.ID == "" {
		return nil, nil // irrelevante — handler responde 200
	}

	return &psp.WebhookEvent{
		EventType: payload.Action, // "payment.created" / "payment.updated"
		PSPID:     payload.Data.ID,
		Status:    normalizeStatusFromAction(payload.Action),
		RawBody:   body,
	}, nil
}

// -- helpers ----------------------------------------------------------------

func normalizeStatus(mpStatus string) psp.PaymentStatus {
	switch mpStatus {
	case "pending", "in_process", "in_mediation":
		return psp.StatusPending
	case "approved":
		return psp.StatusApproved
	case "authorized":
		return psp.StatusAuthorized
	case "rejected":
		return psp.StatusRejected
	case "cancelled", "refunded", "charged_back":
		return psp.StatusCancelled
	default:
		return psp.StatusPending
	}
}

func normalizeStatusFromAction(action string) psp.PaymentStatus {
	switch action {
	case "payment.updated":
		// MP manda "updated" pra várias transições — precisamos dar GetPayment pra saber o estado.
		// Retornar pending aqui força o handler a chamar GetPayment antes de confirmar.
		return psp.StatusPending
	case "payment.created":
		return psp.StatusPending
	case "payment.cancelled":
		return psp.StatusCancelled
	default:
		return psp.StatusPending
	}
}

// Compile-time assertion que Gateway implementa psp.Gateway.
var _ psp.Gateway = (*Gateway)(nil)
var _ = errors.New // mantém import disponível pra refactors futuros
