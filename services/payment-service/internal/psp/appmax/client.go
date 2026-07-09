// Package appmax implementa psp.Gateway usando a API da Appmax **v3**
// (docs.appmax.com.br) — espelhando a integração de produção do gifthy
// (api/pkg/payments/appmax), re-mirada da v1 pra v3 e validada contra o sandbox.
//
// Auth: campo `access-token` no corpo E no header (token do gerente de conta).
// Fluxo order-centric: POST /customer → POST /order → POST /payment/{credit-card,
// pix,boleto}. Valores em REAIS (Decimal 10,2), não centavos.
//
// As respostas não são publicadas verbatim na doc, então o parser é TOLERANTE
// (digID acha o id em vários caminhos; parseDisplay extrai pix_emv/pix_qrcode/
// pdf/digitable_line). Envelope validado no sandbox: {success,text,data,status}.
//
// Base URL: produção `https://admin.appmax.com.br/api/v3`; sandbox via env
// APPMAX_BASE_URL=https://homolog.sandboxappmax.com.br/api/v3.
package appmax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/utilar/pkg/httpclient"
)

const defaultBaseURL = "https://admin.appmax.com.br/api/v3"

// Client é o cliente HTTP da Appmax v3.
type Client struct {
	accessToken string
	baseURL     string
	http        *http.Client
}

func NewClient(accessToken string) *Client {
	// Sandbox via APPMAX_BASE_URL=https://homolog.sandboxappmax.com.br/api/v3.
	return NewWithBaseURL(accessToken, cleanEnv(os.Getenv("APPMAX_BASE_URL")))
}

func NewWithBaseURL(accessToken, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		accessToken: cleanEnv(accessToken),
		baseURL:     cleanEnv(baseURL),
		http:        httpclient.New(30 * time.Second),
	}
}

// cleanEnv tira espaço e aspas envolventes — o env_file do Docker às vezes
// entrega o valor como `"X"` literal, o que faria a Appmax devolver 401.
func cleanEnv(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// post faz um POST JSON v3 com o access-token no corpo E no header.
func (c *Client) post(ctx context.Context, path string, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		body = map[string]any{}
	}
	body["access-token"] = c.accessToken
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("access-token", c.accessToken)
	return c.do(req, http.MethodPost, path)
}

// get faz um GET com o access-token no header + query param.
func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	u := c.baseURL + path
	if c.accessToken != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		u += sep + "access-token=" + url.QueryEscape(c.accessToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("access-token", c.accessToken)
	return c.do(req, http.MethodGet, path)
}

func (c *Client) do(req *http.Request, method, path string) (json.RawMessage, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// v3: `success` pode vir "ATIVA" (string) no Pix — decidimos por HTTP status.
	if resp.StatusCode >= 400 {
		return raw, fmt.Errorf("appmax %s %s → %d: %s", method, path, resp.StatusCode, raw)
	}
	return raw, nil
}

// ===================== Tipos de request (v3) =====================

// CustomerInput — POST /customer (campos flat v3). CPF vai no PAGAMENTO, não aqui.
// Endereço é obrigatório p/ boleto e recomendado p/ antifraude.
type CustomerInput struct {
	FirstName               string `json:"firstname"`
	LastName                string `json:"lastname"`
	Email                   string `json:"email"`
	Telephone               string `json:"telephone"`
	Postcode                string `json:"postcode,omitempty"`
	AddressStreet           string `json:"address_street,omitempty"`
	AddressStreetNumber     string `json:"address_street_number,omitempty"`
	AddressStreetComplement string `json:"address_street_complement,omitempty"`
	AddressStreetDistrict   string `json:"address_street_district,omitempty"`
	AddressCity             string `json:"address_city,omitempty"`
	AddressState            string `json:"address_state,omitempty"`
	IP                      string `json:"ip,omitempty"`
}

// OrderInput — POST /order. Valores em REAIS.
type OrderInput struct {
	Total      float64        `json:"total"`
	Products   []OrderProduct `json:"products"`
	CustomerID int64          `json:"customer_id"`
	Shipping   float64        `json:"shipping,omitempty"`
	Discount   float64        `json:"discount,omitempty"`
}

type OrderProduct struct {
	SKU            string  `json:"sku"`
	Name           string  `json:"name"`
	Qty            int     `json:"qty"`
	Price          float64 `json:"price"`
	DigitalProduct bool    `json:"digital_product,omitempty"`
}

// CardInput — payment.CreditCard (v3). Token (tokenizado no browser via Appmax JS).
type CardInput struct {
	Token          string `json:"token,omitempty"`
	Number         string `json:"number,omitempty"`
	CVV            string `json:"cvv,omitempty"`
	Month          int    `json:"month,omitempty"`
	Year           int    `json:"year,omitempty"`
	Name           string `json:"name,omitempty"`
	DocumentNumber string `json:"document_number"`
	Installments   int    `json:"installments"`
	SoftDescriptor string `json:"soft_descriptor,omitempty"`
}

// OrderData é a forma TOLERANTE da resposta (id/status/display de pagamento).
type OrderData struct {
	ID           int64
	Status       string
	Total        float64
	PixQrCode    string
	PixEmv       string
	PixExpiresAt string
	BoletoURL    string
	BoletoLine   string
	Raw          json.RawMessage
}

// ===================== Endpoints (v3) =====================

// CreateCustomer faz upsert (match por phone/email) e devolve o customer_id (data.id).
func (c *Client) CreateCustomer(ctx context.Context, in CustomerInput) (int64, json.RawMessage, error) {
	raw, err := c.post(ctx, "/customer", structToMap(in))
	if err != nil {
		return 0, raw, err
	}
	id := digID(raw, "customer_id", "customer")
	if id == 0 {
		return 0, raw, fmt.Errorf("appmax customer: id não encontrado: %s", raw)
	}
	return id, raw, nil
}

// CreateOrder cria o pedido e devolve o order_id (data.id).
func (c *Client) CreateOrder(ctx context.Context, in OrderInput) (int64, json.RawMessage, error) {
	raw, err := c.post(ctx, "/order", structToMap(in))
	if err != nil {
		return 0, raw, err
	}
	id := digID(raw, "order_id", "order")
	if id == 0 {
		return 0, raw, fmt.Errorf("appmax order: id não encontrado: %s", raw)
	}
	return id, raw, nil
}

// PayPix gera a cobrança PIX. Envelope: payment:{pix:{document_number}} (pix minúsculo).
func (c *Client) PayPix(ctx context.Context, orderID, customerID int64, documentNumber string) (*OrderData, error) {
	payload := map[string]any{
		"cart":     map[string]any{"order_id": orderID},
		"customer": map[string]any{"customer_id": customerID},
		"payment":  map[string]any{"pix": map[string]any{"document_number": documentNumber}},
	}
	return c.pay(ctx, "/payment/pix", payload, orderID)
}

// PayBoleto gera o boleto. Envelope: payment:{Boleto:{document_number}} (Boleto PascalCase).
func (c *Client) PayBoleto(ctx context.Context, orderID, customerID int64, documentNumber string) (*OrderData, error) {
	payload := map[string]any{
		"cart":     map[string]any{"order_id": orderID},
		"customer": map[string]any{"customer_id": customerID},
		"payment":  map[string]any{"Boleto": map[string]any{"document_number": documentNumber}},
	}
	return c.pay(ctx, "/payment/boleto", payload, orderID)
}

// PayCard cobra o cartão (tokenizado). Envelope: payment:{CreditCard:{...}}.
func (c *Client) PayCard(ctx context.Context, orderID, customerID int64, card CardInput) (*OrderData, error) {
	payload := map[string]any{
		"cart":     map[string]any{"order_id": orderID},
		"customer": map[string]any{"customer_id": customerID},
		"payment":  map[string]any{"CreditCard": card},
	}
	return c.pay(ctx, "/payment/credit-card", payload, orderID)
}

func (c *Client) pay(ctx context.Context, path string, payload map[string]any, orderID int64) (*OrderData, error) {
	raw, err := c.post(ctx, path, payload)
	if err != nil {
		return nil, err
	}
	o := &OrderData{ID: orderID, Raw: raw}
	parseDisplay(raw, o)
	return o, nil
}

// GetOrder consulta um pedido pelo id (status + total autoritativos) — usado na
// reconciliação de valor do webhook (audit C3). Endpoint a confirmar no sandbox.
func (c *Client) GetOrder(ctx context.Context, orderID string) (*OrderData, error) {
	raw, err := c.get(ctx, "/order/"+url.PathEscape(orderID))
	if err != nil {
		return nil, err
	}
	o := &OrderData{Raw: raw}
	o.ID = digID(raw, "order_id", "order")
	parseDisplay(raw, o)
	o.Total = digFloat(raw, "total")
	return o, nil
}

// Refund estorna o pedido (total).
func (c *Client) Refund(ctx context.Context, orderID string) error {
	_, err := c.post(ctx, "/refund", map[string]any{"order_id": orderID, "refund_type": "total"})
	return err
}

// ===================== Parse tolerante =====================

func structToMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

// digID procura um id numérico em data.<flat>, data.id, data.<nested>.id, topo.
func digID(raw []byte, flatKey, nestedKey string) int64 {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	data, _ := m["data"].(map[string]any)
	candidates := []any{}
	if data != nil {
		candidates = append(candidates, data[flatKey], data["id"])
		if nested, ok := data[nestedKey].(map[string]any); ok {
			candidates = append(candidates, nested["id"])
		}
	}
	candidates = append(candidates, m[flatKey], m["id"])
	for _, c := range candidates {
		if n := toInt64(c); n != 0 {
			return n
		}
	}
	return 0
}

func digFloat(raw []byte, key string) float64 {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	if data, ok := m["data"].(map[string]any); ok {
		if f, ok := data[key].(float64); ok {
			return f
		}
	}
	if f, ok := m[key].(float64); ok {
		return f
	}
	return 0
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		var i int64
		_, _ = fmt.Sscan(n, &i)
		return i
	}
	return 0
}

// parseDisplay extrai QR/copia-e-cola/boleto/status da resposta de pagamento.
func parseDisplay(raw []byte, o *OrderData) {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	scope := m
	if data, ok := m["data"].(map[string]any); ok {
		scope = data
	}
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := scope[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	o.PixEmv = getStr("pix_emv", "pix_qrcode_text", "qr_code", "emv")
	o.PixQrCode = getStr("pix_qrcode", "qr_code_url", "pix_qrcode_url")
	o.PixExpiresAt = getStr("pix_expiration_date", "expiration_date")
	o.BoletoURL = getStr("pdf", "billet_url", "boleto_url")
	o.BoletoLine = getStr("digitable_line", "billet_digitable_line", "boleto_payment_code", "line")
	if s := getStr("status", "order_status"); s != "" {
		o.Status = s
	}
}
