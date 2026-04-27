// Package catalogclient é o cliente HTTP do order-service pro catalog-service.
//
// SEGURANÇA (audit O2-H5):
//
// O order-service não pode confiar no `unitPrice` que o cliente envia ao criar
// um pedido — atacante pode mandar `unitPrice: 0.01` num produto de R$ 5000.
// O fix é consultar o catalog-service e usar `product.price` como source-of-truth.
//
// catalog-service é público (sem auth), então não há JWT propagation aqui.
// Diferente do payment→order client onde o JWT é necessário pra ownership scoping.
package catalogclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/utilar/pkg/httpclient"
)

// Product é o subset de Product do catalog-service que o order-service
// precisa pra validar preço. Mantido enxuto pra reduzir acoplamento.
type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}

var (
	ErrNotFound = errors.New("catalogclient: product not found")
	ErrUpstream = errors.New("catalogclient: upstream error")
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New cria um cliente. baseURL ex.: "http://localhost:8091" (sem trailing slash).
// Timeout 5s — catalog é local; lentidão = abortar pedido.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpclient.New(5 * time.Second), // L-PAYMENT-1
	}
}

// GetByID busca um produto pelo seu UUID via GET /api/v1/products/by-id/:id.
//
// Comportamento:
//   - 200 OK         → *Product
//   - 404 Not Found  → ErrNotFound (id inválido ou produto removido)
//   - outros status  → ErrUpstream
//   - timeout/conn   → ErrUpstream
func (c *Client) GetByID(ctx context.Context, productID string) (*Product, error) {
	if productID == "" {
		return nil, fmt.Errorf("%w: empty productID", ErrUpstream)
	}

	url := fmt.Sprintf("%s/api/v1/products/by-id/%s", c.baseURL, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var p Product
		if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
			return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
		}
		return &p, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	}
}
