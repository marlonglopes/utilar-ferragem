// Package appmax implementa psp.Gateway usando a API da Appmax (v1).
//
// A Appmax é um sub-adquirente brasileiro. Diferente de Stripe/MP (amount-centric),
// a Appmax é ORDER-CENTRIC: pra cobrar é preciso, em sequência:
//  1. POST /v1/customers        → cria/atualiza o cliente        → data.customer.id
//  2. POST /v1/orders           → cria o pedido (line items)     → data.order.id
//  3. POST /v1/payments/{pix|boleto|credit-card} → cobra o pedido
//
// Autenticação: a Appmax espera o campo `access-token` DENTRO do corpo JSON de
// cada request (não é header Bearer). Em GET vai como query param.
//
// Base URL: https://api.appmax.com.br/v1 (a Appmax expõe o mesmo host para
// sandbox e produção — o ambiente é definido pela credencial usada).
//
// Refs: https://appmax.readme.io/reference (customer, order, pagamento pix/boleto/cartão).
//
// NOTA: alguns nomes exatos de campos de resposta (pix_emv/pix_qrcode, url do PDF
// do boleto) e o formato do postback precisam ser reconfirmados contra o sandbox
// assim que a conta da Utilar estiver ativa. Por isso guardamos o payload cru
// inteiro em ClientData/RawPayload e repassamos ao frontend sem reformatar.
package appmax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/utilar/pkg/httpclient"
)

const defaultBaseURL = "https://api.appmax.com.br/v1"

// Client é um wrapper HTTP fino sobre a API Appmax.
type Client struct {
	accessToken string
	baseURL     string
	http        *http.Client
}

func NewClient(accessToken string) *Client {
	return NewWithBaseURL(accessToken, defaultBaseURL)
}

func NewWithBaseURL(accessToken, baseURL string) *Client {
	return &Client{
		accessToken: accessToken,
		baseURL:     baseURL,
		http:        httpclient.New(15 * time.Second),
	}
}

// post envia um POST com JSON. O access-token é injetado no corpo automaticamente
// (convenção Appmax), então os callers não precisam repeti-lo.
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
	return c.do(req, http.MethodPost, path)
}

// get envia um GET com o access-token como query param.
func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	u := c.baseURL + path
	if c.accessToken != "" {
		sep := "?"
		if bytes.ContainsRune([]byte(path), '?') {
			sep = "&"
		}
		u += sep + "access-token=" + url.QueryEscape(c.accessToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("appmax %s %s → %d: %s", method, path, resp.StatusCode, raw)
	}
	return raw, nil
}

// --- Endpoints -------------------------------------------------------------

// CreateCustomer cria/atualiza um cliente e devolve o payload cru.
// POST /v1/customers → {data:{customer:{id}}}
func (c *Client) CreateCustomer(ctx context.Context, cust map[string]any) (json.RawMessage, error) {
	return c.post(ctx, "/customers", cust)
}

// CreateOrder cria um pedido e devolve o payload cru.
// POST /v1/orders → {data:{order:{id,status}}}
func (c *Client) CreateOrder(ctx context.Context, order map[string]any) (json.RawMessage, error) {
	return c.post(ctx, "/orders", order)
}

// PayPix cobra um pedido via Pix. POST /v1/payments/pix
func (c *Client) PayPix(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	return c.post(ctx, "/payments/pix", body)
}

// PayBoleto cobra um pedido via boleto. POST /v1/payments/boleto
func (c *Client) PayBoleto(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	return c.post(ctx, "/payments/boleto", body)
}

// PayCard cobra um pedido via cartão de crédito. POST /v1/payments/credit-card
func (c *Client) PayCard(ctx context.Context, body map[string]any) (json.RawMessage, error) {
	return c.post(ctx, "/payments/credit-card", body)
}

// GetOrder consulta os dados de um pedido pelo id Appmax.
// GET /v1/orders/:id → {data:{order:{id,status,total,...}}}
func (c *Client) GetOrder(ctx context.Context, orderID string) (json.RawMessage, error) {
	return c.get(ctx, "/orders/"+url.PathEscape(orderID))
}
