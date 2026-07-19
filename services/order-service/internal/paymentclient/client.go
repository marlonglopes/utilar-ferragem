// Package paymentclient é o cliente HTTP do order-service pro payment-service.
//
// PORQUÊ existe: o livro contábil é ÚNICO e vive no payment-service. Quando a
// venda de balcão é liquidada na maquininha da loja, quem decide e autoriza é o
// order-service (é ele que tem o pedido, o vínculo do operador com a loja e a
// trilha de auditoria do balcão) — mas o LANÇAMENTO tem que cair no mesmo livro
// onde caem as vendas do PSP. Dois serviços escrevendo o próprio livro é como
// se perde a garantia de que tudo soma zero.
package paymentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/utilar/pkg/servicetoken"
)

var (
	// ErrNotConfigured — sem URL ou sem segredo de serviço. Fail-closed
	// explícito: o chamador tem que decidir o que fazer, e não descobrir por
	// acaso que o lançamento nunca aconteceu.
	ErrNotConfigured = errors.New("paymentclient: payment-service não configurado para chamadas de serviço")
	// ErrUpstream — payment-service indisponível ou com erro.
	ErrUpstream = errors.New("paymentclient: payment-service indisponível")
	// ErrPeriodClosed — mês contábil fechado. Não é retentável: vira caso de
	// ajuste manual com justificativa, pelo endpoint contábil.
	ErrPeriodClosed = errors.New("paymentclient: período contábil fechado")
	// ErrRejected — o payment-service recusou o lançamento (4xx que não é
	// período fechado). Retentar não melhora.
	ErrRejected = errors.New("paymentclient: lançamento recusado")
)

type Client struct {
	baseURL       string
	http          *http.Client
	serviceSecret string
}

// New cria o cliente. serviceSecret é o SERVICE_JWT_SECRET — nunca o
// JWT_SECRET de usuário (A1, auditoria 2026-07-18).
func New(baseURL, serviceSecret string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Timeout curto: o operador está com o cliente no caixa. Se o
		// payment-service demorar, o pedido já foi liquidado no order-service
		// e o lançamento é replicável depois — melhor devolver a tela rápido.
		http:          &http.Client{Timeout: 5 * time.Second},
		serviceSecret: serviceSecret,
	}
}

// ExternalSettlement é o fato a lançar: uma venda paga fora do nosso PSP.
type ExternalSettlement struct {
	OrderID           string    `json:"orderId"`
	AmountBRL         float64   `json:"amount"`
	NSU               string    `json:"nsu"`
	StoreID           string    `json:"storeId"`
	OperatorID        string    `json:"operatorId,omitempty"`
	SettledBy         string    `json:"settledBy"`
	Brand             string    `json:"brand,omitempty"`
	AuthorizationCode string    `json:"authorizationCode,omitempty"`
	OccurredAt        time.Time `json:"occurredAt"`
}

// PostExternalSettlement lança a liquidação externa no livro contábil.
//
// IDEMPOTENTE do outro lado: a chave é o pedido (kind=external_sale,
// source_type=external_settlement, source_id=orderID), com UNIQUE no banco do
// payment-service. Chamar duas vezes com o mesmo pedido devolve 200 e nenhum
// lançamento novo — o que torna seguro retentar, e é por isso que o handler de
// liquidação pode reprocessar um pedido já liquidado sem medo de dobrar
// receita.
func (c *Client) PostExternalSettlement(ctx context.Context, in ExternalSettlement) error {
	if c.baseURL == "" || c.serviceSecret == "" {
		return ErrNotConfigured
	}

	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/v1/ledger/external-settlement", bytes.NewReader(body))
	if err != nil {
		return err
	}
	tok, err := servicetoken.Issue(c.serviceSecret, "order-service")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConfigured, err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if rid, ok := ctx.Value(requestIDKey{}).(string); ok && rid != "" {
		req.Header.Set("X-Request-Id", rid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	// Corpo lido e descartado com limite: sem isso a conexão não volta para o
	// pool em keep-alive, e o PDV em hora de pico abre socket por venda.
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		// 200 = já existia (idempotência), 201 = lançado agora. Os dois são
		// sucesso: o fato está no livro exatamente uma vez.
		return nil
	case resp.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrPeriodClosed, snippet)
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return fmt.Errorf("%w: status=%d %s", ErrRejected, resp.StatusCode, snippet)
	default:
		return fmt.Errorf("%w: status=%d", ErrUpstream, resp.StatusCode)
	}
}

// requestIDKey propaga o request_id para correlacionar a trilha do balcão com
// o lançamento contábil do outro serviço. Chave privada para não colidir.
type requestIDKey struct{}

// WithRequestID devolve um contexto que carrega o request_id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
