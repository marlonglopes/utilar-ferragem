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

	// Stock é float64, não int: a migration 005 do catalog trocou
	// products.stock para NUMERIC(14,3) pra permitir venda fracionada (2,5 m
	// de cabo, 1,5 m³ de areia). Decodificar em int faz json.Unmarshal recusar
	// qualquer produto com saldo fracionado, e o erro sai como falha genérica
	// de criação de pedido — sem dizer que o problema é o estoque.
	// Coberto por stockdecimal_test.go.
	Stock float64 `json:"stock"`
}

var (
	ErrNotFound = errors.New("catalogclient: product not found")
	ErrUpstream = errors.New("catalogclient: upstream error")
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	// serviceSecret assina o token role=service das rotas /internal.
	// A1: é o SERVICE_JWT_SECRET, distinto do JWT_SECRET de usuário — ver
	// reservation.go e pkg/servicetoken.
	serviceSecret string
	// res é o disjuntor + retry. nil = comportamento original (chamada direta).
	// Ver resilience.go.
	res *Resilience
}

// New cria um cliente. baseURL ex.: "http://localhost:8091" (sem trailing slash).
// Timeout 5s — catalog é local; lentidão = abortar pedido.
//
// Sem segredo: só os endpoints públicos (GetByID) funcionam. As chamadas de
// reserva falham com erro explícito em vez de silenciosamente pularem o
// controle de estoque.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpclient.New(5 * time.Second), // L-PAYMENT-1
	}
}

// NewWithSecret cria um cliente capaz de chamar as rotas internas de reserva.
// serviceSecret é o SERVICE_JWT_SECRET — nunca o JWT_SECRET de usuário.
func NewWithSecret(baseURL, serviceSecret string) *Client {
	c := New(baseURL)
	c.serviceSecret = serviceSecret
	return c
}

// GetByID busca um produto pelo seu UUID via GET /api/v1/products/by-id/:id.
//
// Comportamento:
//   - 200 OK         → *Product
//   - 404 Not Found  → ErrNotFound (id inválido ou produto removido)
//   - outros status  → ErrUpstream
//   - timeout/conn   → ErrUpstream
//   - catálogo fora  → ErrUnavailable (disjuntor aberto, nem foi à rede)
//
// A chamada roda sob disjuntor + retry quando WithResilience foi configurado.
// GET é idempotente, então repetir é seguro — ver resilience.go, readPolicy.
func (c *Client) GetByID(ctx context.Context, productID string) (*Product, error) {
	if productID == "" {
		return nil, fmt.Errorf("%w: empty productID", ErrUpstream)
	}

	var p Product
	err := c.guard(ctx, readPolicy, func() error {
		return c.getByIDOnce(ctx, productID, &p)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// getByIDOnce é UMA tentativa. Separada para que o retry possa repeti-la sem
// duplicar a montagem do request nem vazar o corpo da resposta anterior.
func (c *Client) getByIDOnce(ctx context.Context, productID string, out *Product) error {
	url := fmt.Sprintf("%s/api/v1/products/by-id/%s", c.baseURL, productID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			// Decode quebrado é resposta corrompida, não erro de negócio:
			// conta para o disjuntor.
			return fmt.Errorf("%w: decode: %v", ErrUpstream, err)
		}
		return nil
	case resp.StatusCode == http.StatusNotFound:
		// Resposta CORRETA do catálogo. Não abre circuito, não é retentada.
		return ErrNotFound
	case retryableStatus(resp.StatusCode):
		return fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	default:
		// 4xx que não é 404: request nosso está errado. Repetir não conserta,
		// mas também não é o catálogo estando fora — por isso não conta como
		// falha de infraestrutura.
		return fmt.Errorf("%w: status=%d", ErrNotRetryable, resp.StatusCode)
	}
}
