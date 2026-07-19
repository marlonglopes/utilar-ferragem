package catalogclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// Reserva de estoque no catalog-service
// ----------------------------------------------------------------------------
// As rotas /api/v1/internal/reservations exigem role=service ou role=admin.
// Como os serviços já compartilham o JWT_SECRET (auth-service emite, todos
// validam), o order-service assina o próprio token de serviço em vez de
// carregar mais um segredo. Token de vida curta (2min) — só precisa sobreviver
// à chamada HTTP; se vazar num log, expira antes de ser útil.
// ============================================================================

// ErrInsufficientStock — algum item do pedido não tem saldo. Carrega o detalhe
// (produto, pedido, disponível) pra o handler montar uma mensagem acionável.
var ErrInsufficientStock = errors.New("catalogclient: insufficient stock")

// Shortage espelha o `details` do 409 do catalog-service.
type Shortage struct {
	ProductID string `json:"productId"`
	Requested int    `json:"requested"`
	// Available é float64 pelo mesmo motivo de Product.Stock: o saldo do
	// catalog é NUMERIC e pode vir fracionado. Se o decode falhar aqui, o
	// cliente perde justamente a informação útil — quanto ainda tem.
	Available float64 `json:"available"`
}

// StockError embrulha ErrInsufficientStock com o detalhe do item que faltou.
type StockError struct {
	Shortage Shortage
}

func (e *StockError) Error() string {
	return fmt.Sprintf("insufficient stock for product %s: requested %d, available %g",
		e.Shortage.ProductID, e.Shortage.Requested, e.Shortage.Available)
}

func (e *StockError) Unwrap() error { return ErrInsufficientStock }

// ReservationItem é um par produto/quantidade.
type ReservationItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// serviceToken assina um JWT HS256 com role=service válido por 2 minutos.
func (c *Client) serviceToken() (string, error) {
	if c.jwtSecret == "" {
		return "", errors.New("catalogclient: JWT secret not configured for service calls")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "order-service",
		"role": "service",
		"iat":  now.Unix(),
		"exp":  now.Add(2 * time.Minute).Unix(),
	})
	return tok.SignedString([]byte(c.jwtSecret))
}

// Reserve reserva os itens de um pedido. All-or-nothing do lado do catalog.
//
// ttl é a validade da reserva; zero usa o default do catalog-service (30min).
func (c *Client) Reserve(ctx context.Context, orderID string, items []ReservationItem, ttl time.Duration) error {
	body := map[string]any{
		"orderId": orderID,
		"items":   items,
	}
	if ttl > 0 {
		body["ttlMinutes"] = int(ttl.Minutes())
	}

	resp, err := c.doInternal(ctx, http.MethodPost, "/api/v1/internal/reservations", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusConflict:
		var env struct {
			Details Shortage `json:"details"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return fmt.Errorf("%w: (detail unavailable)", ErrInsufficientStock)
		}
		return &StockError{Shortage: env.Details}
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("%w: reserve status=%d", ErrUpstream, resp.StatusCode)
	}
}

// Commit transforma a reserva do pedido em baixa definitiva (pedido pago).
func (c *Client) Commit(ctx context.Context, orderID string) error {
	return c.settle(ctx, orderID, "commit")
}

// Release devolve o estoque reservado (pedido cancelado / pagamento falho).
func (c *Client) Release(ctx context.Context, orderID string) error {
	return c.settle(ctx, orderID, "release")
}

func (c *Client) settle(ctx context.Context, orderID, action string) error {
	path := fmt.Sprintf("/api/v1/internal/reservations/%s/%s", orderID, action)
	resp, err := c.doInternal(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s status=%d", ErrUpstream, action, resp.StatusCode)
	}
	return nil
}

// doInternal monta e executa uma chamada autenticada às rotas internas.
func (c *Client) doInternal(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal: %v", ErrUpstream, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/json")

	token, err := c.serviceToken()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return resp, nil
}
