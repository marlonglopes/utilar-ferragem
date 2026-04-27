// Package orderclient é o cliente HTTP do payment-service pro order-service.
//
// SEGURANÇA (audit C1, C2):
//
// O payment-service precisa garantir que:
//   1. (C1) o `amount` cobrado é o que foi gravado no order-service quando o
//      pedido foi criado — não o que o cliente envia no body. O cliente pode
//      tentar tampering enviando `amount: 0.01` num pedido de R$ 5000.
//   2. (C2) o `order_id` referenciado pertence ao usuário autenticado no JWT.
//      Sem isso, atacante cria pagamento referenciando pedido alheio e dispara
//      confirmação cruzada.
//
// O fix dos dois é o mesmo: payment-service faz `GET /api/v1/orders/:id` no
// order-service propagando o JWT do cliente. Como o order-service filtra por
// user_id (ownership já validado lá), se o user não é dono, recebemos 404.
// Se é dono, recebemos `order.total` do servidor — fonte autoritativa do amount.
package orderclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/utilar/pkg/httpclient"
)

// Order é o subset do Order do order-service que o payment-service consome.
// Mantido enxuto pra reduzir dependências entre serviços (anti-corruption layer).
type Order struct {
	ID            string  `json:"id"`
	UserID        string  `json:"userId"`
	Status        string  `json:"status"`
	Total         float64 `json:"total"`
	PaymentMethod string  `json:"paymentMethod"`
}

// Errors normalizados que callers devem checar via errors.Is.
var (
	ErrNotFound     = errors.New("orderclient: not found or not owned by user")
	ErrUnauthorized = errors.New("orderclient: invalid or missing JWT")
	ErrUpstream     = errors.New("orderclient: upstream error")
)

// Client é HTTP-based, stateless. Pool de conexão é o do http.DefaultTransport.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New cria um Client. baseURL deve ser tipo "http://localhost:8092"
// (sem trailing slash). Timeout 5s — order-service é local e rápido;
// se está lento, melhor abortar o checkout que segurar o cliente.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		// L-PAYMENT-1: transport defensivo (dial 1s, conn pool capped, etc).
		httpClient: httpclient.New(5 * time.Second),
	}
}

// Get busca um pedido pelo ID, propagando o JWT do cliente.
//
// Comportamento:
//   - 200 OK             → retorna *Order
//   - 401 Unauthorized   → ErrUnauthorized (JWT inválido/expirado)
//   - 404 Not Found      → ErrNotFound (pedido não existe OU não é do user)
//   - outros 4xx/5xx     → ErrUpstream
//   - timeout/conexão    → ErrUpstream
//
// O 404 é proposital — o order-service não diferencia "não existe" de "não é seu"
// pra evitar enumeration. Pro payment, ambos significam "rejeite a criação".
func (c *Client) Get(ctx context.Context, orderID, jwt string) (*Order, error) {
	if orderID == "" {
		return nil, fmt.Errorf("%w: empty orderID", ErrUpstream)
	}
	if jwt == "" {
		return nil, ErrUnauthorized
	}

	url := fmt.Sprintf("%s/api/v1/orders/%s", c.baseURL, orderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var order Order
		if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
			return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
		}
		return &order, nil
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	}
}
