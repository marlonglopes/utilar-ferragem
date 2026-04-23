package mercadopago

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.mercadopago.com"

type Client struct {
	accessToken string
	baseURL     string
	http        *http.Client
}

func New(accessToken string) *Client {
	return NewWithBaseURL(accessToken, defaultBaseURL)
}

func NewWithBaseURL(accessToken, baseURL string) *Client {
	return &Client{
		accessToken: accessToken,
		baseURL:     baseURL,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) (json.RawMessage, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("mercadopago %s %s → %d: %s", method, path, resp.StatusCode, raw)
	}
	return raw, nil
}

// CreatePixPayment creates a Pix payment preference and returns the raw MP response.
func (c *Client) CreatePixPayment(orderID string, amount float64, email string) (json.RawMessage, error) {
	body := map[string]any{
		"transaction_amount": amount,
		"description":        "Pedido " + orderID,
		"payment_method_id":  "pix",
		"payer": map[string]any{
			"email": email,
		},
	}
	return c.do("POST", "/v1/payments", body)
}

// CreateBoleto creates a boleto payment.
func (c *Client) CreateBoleto(orderID string, amount float64, email, cpf, name string) (json.RawMessage, error) {
	body := map[string]any{
		"transaction_amount": amount,
		"description":        "Pedido " + orderID,
		"payment_method_id":  "bolbradesco",
		"payer": map[string]any{
			"email":      email,
			"first_name": name,
			"identification": map[string]string{
				"type":   "CPF",
				"number": cpf,
			},
		},
	}
	return c.do("POST", "/v1/payments", body)
}

// GetPayment fetches a payment by MP payment ID.
func (c *Client) GetPayment(mpPaymentID string) (json.RawMessage, error) {
	return c.do("GET", "/v1/payments/"+mpPaymentID, nil)
}

// CreatePreference creates a card checkout preference (hosted checkout).
func (c *Client) CreatePreference(orderID string, amount float64, title string) (json.RawMessage, error) {
	body := map[string]any{
		"items": []map[string]any{
			{
				"title":      title,
				"quantity":   1,
				"unit_price": amount,
				"currency_id": "BRL",
			},
		},
		"external_reference": orderID,
		"back_urls": map[string]string{
			"success": "https://utilarferragem.com.br/pedido/sucesso",
			"failure": "https://utilarferragem.com.br/pedido/falha",
			"pending": "https://utilarferragem.com.br/pedido/pendente",
		},
		"auto_return": "approved",
	}
	return c.do("POST", "/checkout/preferences", body)
}
