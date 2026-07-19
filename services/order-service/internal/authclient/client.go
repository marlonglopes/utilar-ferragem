// Package authclient é o cliente HTTP do order-service pro auth-service.
//
// PORQUÊ existe: o teto de desconto de um operador é o número que decide se uma
// venda sai aprovada ou vai para a fila do gerente — dinheiro saindo do caixa.
// Ele NÃO viaja no JWT (ver o comentário de Claims em
// auth-service/internal/auth/jwt.go): num token de 15 minutos, rebaixar um
// vendedor levaria até 15 minutos para valer. Então o order-service pergunta ao
// auth-service no momento da decisão, exatamente como já pergunta o preço ao
// catalog-service.
//
// Bancos são separados por serviço; não existe SELECT cross-DB para isso.
package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNotOperator — o usuário não tem vínculo ativo de operador de balcão.
var ErrNotOperator = errors.New("authclient: user is not an active store operator")

// Operator espelha model.StoreOperator do auth-service (subset consumido aqui).
type Operator struct {
	UserID             string  `json:"userId"`
	StoreID            string  `json:"storeId"`
	StoreCode          string  `json:"storeCode"`
	StoreName          string  `json:"storeName"`
	Level              string  `json:"level"`
	DiscountCeilingPct float64 `json:"discountCeilingPct"`
	CanApproveDiscount bool    `json:"canApproveDiscount"`
	Active             bool    `json:"active"`
}

type Client struct {
	baseURL   string
	http      *http.Client
	jwtSecret string

	// Cache curto. 30s é o compromisso: elimina a chamada HTTP repetida quando
	// o operador registra três vendas seguidas, e mantém um rebaixamento
	// valendo em menos de meio minuto (contra os 15min do TTL do token, que é
	// justamente o que estamos evitando).
	mu    sync.RWMutex
	cache map[string]cacheEntry
	ttl   time.Duration
}

type cacheEntry struct {
	op  *Operator
	exp time.Time
}

func New(baseURL, jwtSecret string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: 5 * time.Second},
		jwtSecret: jwtSecret,
		cache:     make(map[string]cacheEntry),
		ttl:       30 * time.Second,
	}
}

// GetOperator busca o contexto autoritativo do operador.
//
// Devolve ErrNotOperator em 404 (usuário sem vínculo, ou vínculo desativado) —
// o auth-service já trata inativo como 404 para que quem chama não dependa de
// lembrar de checar a flag.
func (c *Client) GetOperator(ctx context.Context, userID string) (*Operator, error) {
	if userID == "" {
		return nil, ErrNotOperator
	}

	c.mu.RLock()
	e, ok := c.cache[userID]
	c.mu.RUnlock()
	if ok && time.Now().Before(e.exp) {
		if e.op == nil {
			return nil, ErrNotOperator
		}
		return e.op, nil
	}

	tok, err := c.serviceToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/internal/operators/%s", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authclient: get operator: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		// Cacheia o negativo também: um customer comum tentando bater no PDV
		// não deve gerar uma chamada HTTP por request.
		c.store(userID, nil)
		return nil, ErrNotOperator
	default:
		return nil, fmt.Errorf("authclient: unexpected status %d", resp.StatusCode)
	}

	var op Operator
	if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
		return nil, fmt.Errorf("authclient: decode operator: %w", err)
	}
	if !op.Active {
		c.store(userID, nil)
		return nil, ErrNotOperator
	}
	c.store(userID, &op)
	return &op, nil
}

func (c *Client) store(userID string, op *Operator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[userID] = cacheEntry{op: op, exp: time.Now().Add(c.ttl)}
}

// serviceToken assina um JWT HS256 com role=service, válido por 2 minutos —
// mesmo padrão do catalogclient (os serviços já compartilham o JWT_SECRET, e um
// token de vida curta que vaze em log expira antes de servir para algo).
func (c *Client) serviceToken() (string, error) {
	if c.jwtSecret == "" {
		return "", errors.New("authclient: JWT secret not configured for service calls")
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
