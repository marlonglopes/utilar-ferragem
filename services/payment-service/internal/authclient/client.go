// Package authclient é o cliente HTTP do payment-service pro auth-service.
//
// Uso atual: buscar CPF + nome do user pra preencher boleto MP (audit M6).
// MP rejeita boleto com payer.identification.number vazio em prod, então
// não dá pra confiar no body do request — buscamos no auth via JWT.
package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// User é o subset que payment precisa. Espelha auth-service/internal/model.User.
type User struct {
	ID    string  `json:"id"`
	Email string  `json:"email"`
	Name  string  `json:"name"`
	CPF   *string `json:"cpf,omitempty"`
}

var (
	ErrUnauthorized = errors.New("authclient: invalid or missing JWT")
	ErrUpstream     = errors.New("authclient: upstream error")
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Me chama GET /api/v1/me com o JWT do cliente. Retorna o User logado.
func (c *Client) Me(ctx context.Context, jwt string) (*User, error) {
	if jwt == "" {
		return nil, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/me", nil)
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
		var u User
		if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
			return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
		}
		return &u, nil
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	}
}
