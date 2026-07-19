// Package appmaxv1 implementa a integração com a **Appmax AppStore API v1**
// (appstore-docs.appmax.com.br), que é uma API DIFERENTE da v3 admin já
// implementada em internal/psp/appmax.
//
// Diferenças estruturais v1-AppStore x v3-admin:
//
//	                v3 (internal/psp/appmax)        v1 (este pacote)
//	auth            access-token (header + body)    OAuth2 client_credentials (Bearer)
//	base            admin.appmax.com.br/api/v3      api.appmax.com.br/v1
//	valores         REAIS (decimal 10,2)            CENTAVOS (inteiros) em TODA a API
//	envelope        {success,text,data,status}      {data} | {error|errors}
//	rotas           /customer /order /payment/*     /v1/customers /v1/orders /v1/payments/*
//	split           n/d                             /v1/orders/{id}/split-order + /v1/recipient
//
// Os dois convivem: PSP_PROVIDER=appmax (v3) ou PSP_PROVIDER=appmax-v1 (este).
//
// LACUNAS CONHECIDAS (ver docs/appmax-v1-appstore.md):
//   - A API NÃO tem chave de idempotência. Um retry de rede em POST /v1/payments/*
//     pode gerar cobrança duplicada. Mitigação: o handler só chama CreatePayment uma
//     vez por payment row e, em rotas financeiras, este client só re-tenta 401 e 429
//     (que comprovadamente não processaram o request) — ver isFinancialRoute.
//   - Webhooks NÃO são assinados (sem HMAC, sem token). A integridade vem da
//     re-consulta GET /v1/orders/{id} (mesmo padrão do v3, audit C3).
//   - Split é valor FIXO em centavos e a Appmax redistribui em silêncio se a soma
//     exceder o partner_total — trava própria em SplitOrder.
package appmaxv1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/utilar/payment-service/internal/psp"
	"github.com/utilar/pkg/httpclient"
	"github.com/utilar/pkg/requestid"
)

// Endpoints padrão (produção). Sandbox via env — ver Config.
const (
	DefaultAuthURL = "https://auth.appmax.com.br"
	DefaultAPIURL  = "https://api.appmax.com.br"

	SandboxAuthURL = "https://auth.sandboxappmax.com.br"
	SandboxAPIURL  = "https://api.sandboxappmax.com.br"
)

// tokenSkew é a margem de renovação proativa. Com expires_in=3600 (1h), o token
// é renovado aos 55min — nunca esperamos ele expirar de fato.
const tokenSkew = 5 * time.Minute

// Config são as credenciais/URLs do provider (APPMAX_V1_*).
type Config struct {
	AuthURL      string // APPMAX_V1_AUTH_URL  (default: produção)
	APIURL       string // APPMAX_V1_API_URL   (default: produção)
	ClientID     string // APPMAX_V1_CLIENT_ID
	ClientSecret string // APPMAX_V1_CLIENT_SECRET
	ExternalID   string // APPMAX_V1_EXTERNAL_ID — ver nota em externalID()

	// WebhookSecret é OPCIONAL (a Appmax não assina). Se setado, exigimos o
	// header X-Appmax-Token igual — defesa em profundidade.
	WebhookSecret string

	// HTTPTimeout é o timeout total por request (default 30s).
	HTTPTimeout time.Duration

	// MaxRetries/BackoffBase controlam o backoff exponencial em 429/5xx.
	// Zero = defaults (3 tentativas, base 300ms). Exposto para os testes.
	MaxRetries  int
	BackoffBase time.Duration
}

// Client é o cliente HTTP da Appmax AppStore v1 com cache de token OAuth2.
type Client struct {
	cfg  Config
	http *http.Client

	mu          sync.RWMutex
	token       string
	tokenExpiry time.Time

	// authMu serializa o fetch do token — evita N goroutines batendo no /oauth2/token
	// ao mesmo tempo quando o cache expira (thundering herd).
	authMu sync.Mutex

	// sleep é injetável nos testes para não dormir de verdade no backoff.
	sleep func(context.Context, time.Duration) error
}

// NewClient monta o client aplicando defaults.
func NewClient(cfg Config) *Client {
	cfg.AuthURL = strings.TrimRight(cleanEnv(cfg.AuthURL), "/")
	cfg.APIURL = strings.TrimRight(cleanEnv(cfg.APIURL), "/")
	cfg.ClientID = cleanEnv(cfg.ClientID)
	cfg.ClientSecret = cleanEnv(cfg.ClientSecret)
	cfg.ExternalID = cleanEnv(cfg.ExternalID)
	cfg.WebhookSecret = cleanEnv(cfg.WebhookSecret)
	if cfg.AuthURL == "" {
		cfg.AuthURL = DefaultAuthURL
	}
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = 300 * time.Millisecond
	}
	return &Client{
		cfg:   cfg,
		http:  httpclient.New(cfg.HTTPTimeout),
		sleep: sleepCtx,
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

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ===================== OAuth2 =====================

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// cachedToken devolve o token em cache se ainda estiver dentro da janela segura.
func (c *Client) cachedToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token
	}
	return ""
}

// Token devolve um access token válido, renovando proativamente.
// Não existe refresh token na Appmax v1 — renovar = pedir outro client_credentials.
func (c *Client) Token(ctx context.Context) (string, error) {
	if t := c.cachedToken(); t != "" {
		return t, nil
	}
	return c.refreshToken(ctx)
}

// refreshToken força um novo client_credentials e atualiza o cache.
func (c *Client) refreshToken(ctx context.Context) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	// Outra goroutine pode ter renovado enquanto esperávamos o lock.
	if t := c.cachedToken(); t != "" {
		return t, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.AuthURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: oauth2/token: %v", psp.ErrUpstream, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: oauth2/token read: %v", psp.ErrUpstream, err)
	}
	if resp.StatusCode >= 400 {
		return "", httpError(resp.StatusCode, http.MethodPost, "/oauth2/token", raw)
	}

	var tr tokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil || tr.AccessToken == "" {
		// Alguns gateways aninham em data — parser tolerante.
		if t := digString(raw, "access_token"); t != "" {
			tr.AccessToken = t
			tr.ExpiresIn = int64(digFloat(raw, "expires_in"))
		}
	}
	if tr.AccessToken == "" {
		// SEGURANÇA (audit AV1-H4): NÃO ecoamos `raw` aqui. Este é o corpo do
		// endpoint de TOKEN — se a Appmax mudar o shape da resposta (o parser é
		// tolerante justamente porque isso já aconteceu), o token viria dentro
		// dele e cairia na mensagem de erro, que é logada e sobe pelo stack até
		// o handler. Erro de auth reporta o QUE falhou, nunca o corpo.
		return "", fmt.Errorf("%w: oauth2/token respondeu %d sem access_token reconhecível (%d bytes) — confira APPMAX_V1_CLIENT_ID/SECRET e a URL de auth",
			psp.ErrUpstream, resp.StatusCode, len(raw))
	}

	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour // default documentado (expires_in=3600)
	}
	// Renovação proativa: expira o cache `tokenSkew` antes do vencimento real.
	// 3600s - 300s = 3300s → renova aos 55min, como manda a doc.
	exp := time.Now().Add(ttl - tokenSkew)
	if !exp.After(time.Now()) {
		exp = time.Now().Add(ttl / 2)
	}

	c.mu.Lock()
	c.token, c.tokenExpiry = tr.AccessToken, exp
	c.mu.Unlock()
	return tr.AccessToken, nil
}

// invalidateToken zera o cache (usado ao receber 401).
func (c *Client) invalidateToken() {
	c.mu.Lock()
	c.token, c.tokenExpiry = "", time.Time{}
	c.mu.Unlock()
}

// ===================== Transporte =====================

// httpError traduz status HTTP → erro normalizado psp.*, preservando o corpo.
func httpError(status int, method, path string, body []byte) error {
	base := psp.ErrUpstream
	switch {
	case status == http.StatusNotFound:
		base = psp.ErrNotFound
	case status == http.StatusUnprocessableEntity, status == http.StatusBadRequest,
		status == http.StatusConflict, status == http.StatusUnauthorized,
		status == http.StatusForbidden:
		base = psp.ErrInvalidRequest
	case status >= 400 && status < 500 && status != http.StatusTooManyRequests:
		base = psp.ErrInvalidRequest
	}
	return fmt.Errorf("%w: appmax-v1 %s %s → %d: %s", base, method, path, status, truncate(body, 2000))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…(truncado)"
}

// retryAfter extrai a espera sugerida do 429 (header Retry-After em segundos ou
// data HTTP; fallback no campo `retryAfter` do corpo).
func retryAfter(h http.Header, body []byte) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}
	if f := digFloat(body, "retryAfter"); f > 0 {
		return time.Duration(f) * time.Second
	}
	return 0
}

// doJSON executa um request JSON autenticado em /v1/*, com:
//   - Bearer token (cache + renovação proativa)
//   - 1 re-tentativa em 401 (token revogado no meio do voo → re-auth e repete)
//   - backoff exponencial em 429/5xx respeitando Retry-After
//
// ATENÇÃO (idempotência): a Appmax v1 não expõe chave de idempotência. O retry de
// 5xx em rotas financeiras (/v1/payments/*) é DESLIGADO por isso — só 401 e 429
// (que comprovadamente não processaram o request) são re-tentados nelas.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal %s: %v", psp.ErrInvalidRequest, path, err)
		}
		payload = b
	}

	financial := isFinancialRoute(path)
	var lastErr error

	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		token, err := c.Token(ctx)
		if err != nil {
			return nil, err
		}

		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIURL+path, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: appmax-v1 %s %s: %v", psp.ErrUpstream, method, path, err)
			if financial {
				return nil, lastErr // erro de rede em rota financeira: não repetimos (sem idempotência)
			}
			if werr := c.backoff(ctx, attempt, 0); werr != nil {
				return nil, lastErr
			}
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%w: appmax-v1 %s %s read: %v", psp.ErrUpstream, method, path, readErr)
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt < c.cfg.MaxRetries:
			// Token revogado/expirado antes da hora → re-auth e tenta de novo (1x é
			// o suficiente; o loop naturalmente para porque o cache já foi renovado).
			slog.Warn("appmax-v1 401 — re-autenticando", "path", path, "attempt", attempt)
			c.invalidateToken()
			lastErr = httpError(resp.StatusCode, method, path, raw)
			continue

		case resp.StatusCode == http.StatusTooManyRequests && attempt < c.cfg.MaxRetries:
			wait := retryAfter(resp.Header, raw)
			slog.Warn("appmax-v1 429 rate limit",
				"path", path, "attempt", attempt, "retry_after", wait.String(),
				"limit", resp.Header.Get("X-RateLimit-Limit"),
				"remaining", resp.Header.Get("X-RateLimit-Remaining"))
			lastErr = httpError(resp.StatusCode, method, path, raw)
			if err := c.backoff(ctx, attempt, wait); err != nil {
				return nil, lastErr
			}
			continue

		case resp.StatusCode >= 500 && attempt < c.cfg.MaxRetries && !financial:
			lastErr = httpError(resp.StatusCode, method, path, raw)
			if err := c.backoff(ctx, attempt, 0); err != nil {
				return nil, lastErr
			}
			continue

		case resp.StatusCode >= 400:
			return raw, httpError(resp.StatusCode, method, path, raw)
		}
		return raw, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: appmax-v1 %s %s: sem resposta", psp.ErrUpstream, method, path)
	}
	return nil, lastErr
}

// isFinancialRoute marca rotas onde um retry cego poderia duplicar dinheiro.
func isFinancialRoute(path string) bool {
	return strings.HasPrefix(path, "/v1/payments/") && path != "/v1/payments/installments"
}

// backoff dorme respeitando Retry-After; sem ele, exponencial (base * 2^attempt).
func (c *Client) backoff(ctx context.Context, attempt int, suggested time.Duration) error {
	d := suggested
	if d <= 0 {
		d = c.cfg.BackoffBase * time.Duration(1<<uint(attempt))
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return c.sleep(ctx, d)
}

// ===================== Conversão de valores =====================

// ToCents converte reais (float64) → centavos (int64) com ARREDONDAMENTO.
// Truncar aqui perderia centavos em valores como 19.99*100 = 1998.9999...
func ToCents(reais float64) int64 { return int64(math.Round(reais * 100)) }

// FromCents converte centavos → reais.
func FromCents(cents int64) float64 { return float64(cents) / 100 }

// ===================== Tipos de request =====================

// Address é o endereço opcional do customer (snake_case, como o resto de /v1).
type Address struct {
	Postcode   string `json:"postcode,omitempty"`
	Street     string `json:"street,omitempty"`
	Number     string `json:"number,omitempty"`
	Complement string `json:"complement,omitempty"`
	District   string `json:"district,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
}

// Tracking são os UTMs opcionais do customer.
type Tracking struct {
	UTMSource   string `json:"utm_source,omitempty"`
	UTMCampaign string `json:"utm_campaign,omitempty"`
}

// CustomerInput — POST /v1/customers.
// Obrigatórios pela doc: first_name, last_name, email, phone, ip.
type CustomerInput struct {
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	IP             string    `json:"ip"`
	DocumentNumber string    `json:"document_number,omitempty"`
	Address        *Address  `json:"address,omitempty"`
	Tracking       *Tracking `json:"tracking,omitempty"`
}

// OrderProduct — item de /v1/orders. TODOS os campos são obrigatórios pela doc.
// UnitValue em CENTAVOS.
type OrderProduct struct {
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitValue int64  `json:"unit_value"`
	Type      string `json:"type"`
}

// ProductTypePhysical é o `type` usado por padrão nos itens.
// A doc lista `type` como obrigatório mas não publica o enum; "physical"/"digital"
// é a convenção herdada do v3 (digital_product bool). Ver docs/appmax-v1-appstore.md.
const (
	ProductTypePhysical = "physical"
	ProductTypeDigital  = "digital"
)

// OrderInput — POST /v1/orders. Todos os valores em CENTAVOS.
// A Appmax NÃO calcula juros: products_value/discount_value/shipping_value são finais.
type OrderInput struct {
	CustomerID    int64          `json:"customer_id"`
	ProductsValue int64          `json:"products_value"`
	DiscountValue int64          `json:"discount_value"`
	ShippingValue int64          `json:"shipping_value"`
	Products      []OrderProduct `json:"products"`
	ExternalID    string         `json:"external_id,omitempty"`
}

// CardInput — dados do cartão para /v1/payments/tokenize.
type CardInput struct {
	Number          string `json:"number"`
	CVV             string `json:"cvv"`
	ExpirationMonth string `json:"expiration_month"`
	ExpirationYear  string `json:"expiration_year"`
	HolderName      string `json:"holder_name"`
}

// CardChargeInput — POST /v1/payments/credit-card.
type CardChargeInput struct {
	Token                string `json:"token"`
	HolderDocumentNumber string `json:"holder_document_number"`
	HolderName           string `json:"holder_name"`
	Installments         int    `json:"installments"`
	SoftDescriptor       string `json:"soft_descriptor,omitempty"`
}

// ===================== Tipos de resposta (parser tolerante) =====================

// PaymentResult é a forma normalizada de qualquer /v1/payments/*.
// ATENÇÃO ao PixQRCode: aqui ele é PNG em base64 SEM o prefixo `data:` — no
// WEBHOOK o campo de mesmo nome é uma URL. Ver ParseEvent.
type PaymentResult struct {
	OrderID      int64
	Status       string
	Method       string
	Installments int
	PixQRCodeB64 string // base64 PNG (resposta de API)
	PixEMV       string // copia-e-cola
	PixExpiresAt string
	BoletoURL    string
	BoletoLine   string
	UpsellHash   string
	PaidAt       string
	Raw          json.RawMessage
}

// OrderView é a forma tolerante de GET /v1/orders/{id}. Valores em CENTAVOS.
type OrderView struct {
	ID              int64
	Status          string
	TotalPaid       int64
	SubTotal        int64
	ShippingValue   int64
	Discount        int64
	InstallmentFee  int64
	CreatedAt       string
	UpdatedAt       string
	PaymentMethod   string
	Installments    int
	InstallmentsAmt int64
	CardBrand       string
	CardNumber      string
	PaidAt          string
	RefundedAt      string
	Raw             json.RawMessage
}

// TotalCents é o total autoritativo do pedido em centavos. Preferimos
// total_paid; se o pedido ainda não foi pago, recompomos pelos amounts.
func (o *OrderView) TotalCents() int64 {
	if o.TotalPaid > 0 {
		return o.TotalPaid
	}
	return o.SubTotal + o.ShippingValue + o.InstallmentFee - o.Discount
}

// ===================== Endpoints — customers / orders =====================

// CreateCustomer cria (ou faz match de) um cliente e devolve data.customer.id.
func (c *Client) CreateCustomer(ctx context.Context, in CustomerInput) (int64, json.RawMessage, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/customers", in)
	if err != nil {
		return 0, raw, err
	}
	id := digID(raw, "customer_id", "customer")
	if id == 0 {
		return 0, raw, fmt.Errorf("%w: customer id não encontrado: %s", psp.ErrUpstream, truncate(raw, 500))
	}
	return id, raw, nil
}

// CreateOrder cria o pedido (valores em centavos) e devolve data.order.id.
func (c *Client) CreateOrder(ctx context.Context, in OrderInput) (int64, json.RawMessage, error) {
	if in.ExternalID == "" {
		in.ExternalID = c.cfg.ExternalID
	}
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/orders", in)
	if err != nil {
		return 0, raw, err
	}
	id := digID(raw, "order_id", "order")
	if id == 0 {
		return 0, raw, fmt.Errorf("%w: order id não encontrado: %s", psp.ErrUpstream, truncate(raw, 500))
	}
	return id, raw, nil
}

// GetOrder consulta o pedido — fonte autoritativa de status e valor.
// É este endpoint que dá integridade ao webhook (que não é assinado).
func (c *Client) GetOrder(ctx context.Context, orderID string) (*OrderView, error) {
	raw, err := c.doJSON(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(orderID), nil)
	if err != nil {
		return nil, err
	}
	return parseOrderView(raw), nil
}

// ===================== Endpoints — payments =====================

// Tokenize troca os dados do cartão por um token de USO ÚNICO.
//
// AVISO PCI: o fluxo correto é tokenizar no BROWSER via Appmax JS — assim o PAN
// nunca toca nosso backend (SAQ-A). Este método existe para testes/servidores
// já em escopo PCI. Prefira receber o token pronto em psp.CreateRequest.CardToken.
func (c *Client) Tokenize(ctx context.Context, card CardInput) (string, json.RawMessage, error) {
	body := map[string]any{"payment_data": map[string]any{"credit_card": card}}
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/payments/tokenize", body)
	if err != nil {
		return "", raw, err
	}
	tok := digString(raw, "token")
	if tok == "" {
		return "", raw, fmt.Errorf("%w: tokenize sem token: %s", psp.ErrUpstream, truncate(raw, 500))
	}
	return tok, raw, nil
}

// PayCreditCard cobra o cartão tokenizado.
func (c *Client) PayCreditCard(ctx context.Context, orderID, customerID int64, in CardChargeInput) (*PaymentResult, error) {
	if in.Installments <= 0 {
		in.Installments = 1
	}
	body := map[string]any{
		"order_id":    orderID,
		"customer_id": customerID,
		"payment_data": map[string]any{
			"credit_card": in,
		},
	}
	return c.pay(ctx, "/v1/payments/credit-card", body, orderID)
}

// PayPix gera a cobrança Pix. Retorna QR em base64 (PNG) + EMV copia-e-cola.
func (c *Client) PayPix(ctx context.Context, orderID int64, documentNumber string) (*PaymentResult, error) {
	body := map[string]any{
		"order_id":     orderID,
		"payment_data": map[string]any{"pix": map[string]any{"document_number": documentNumber}},
	}
	return c.pay(ctx, "/v1/payments/pix", body, orderID)
}

// PayBoleto gera o boleto.
//
// A doc do endpoint não publica o JSON de resposta verbatim; usamos um parser
// tolerante aceitando boleto_url|pdf, boleto_digitable_line|digitable_line e
// boleto_overdue_date|due_date (nomes observados no payload de webhook).
func (c *Client) PayBoleto(ctx context.Context, orderID int64, documentNumber string) (*PaymentResult, error) {
	body := map[string]any{
		"order_id":     orderID,
		"payment_data": map[string]any{"boleto": map[string]any{"document_number": documentNumber}},
	}
	return c.pay(ctx, "/v1/payments/boleto", body, orderID)
}

func (c *Client) pay(ctx context.Context, path string, body map[string]any, orderID int64) (*PaymentResult, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	r := parsePaymentResult(raw)
	if r.OrderID == 0 {
		r.OrderID = orderID
	}
	return r, nil
}

// Installments consulta a tabela de parcelamento.
// Resposta: {"data":{"installments":{"1":{"total":20330},"3":{"total":21147}}}}
// `total` é o valor TOTAL da compra parcelada, em centavos — a divisão pelo número
// de parcelas para achar o valor da parcela é responsabilidade da integração.
//
// settings: modo de juros — "PP" (Parcela Paga / comprador) ou "AM" (Absorve
// Merchant / lojista), conforme configurado na conta.
func (c *Client) Installments(ctx context.Context, installments int, totalValueCents int64, settings string) (map[int]int64, json.RawMessage, error) {
	body := map[string]any{
		"installments": installments,
		"total_value":  totalValueCents,
		"settings":     settings,
	}
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/payments/installments", body)
	if err != nil {
		return nil, raw, err
	}
	out := map[int]int64{}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		scope := unwrapData(m)
		if inst, ok := scope["installments"].(map[string]any); ok {
			for k, v := range inst {
				n, err := strconv.Atoi(k)
				if err != nil {
					continue
				}
				switch t := v.(type) {
				case map[string]any:
					out[n] = toInt64(t["total"])
				default:
					out[n] = toInt64(v)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, raw, fmt.Errorf("%w: installments vazio: %s", psp.ErrUpstream, truncate(raw, 500))
	}
	return out, raw, nil
}

// InstallmentAmount devolve o valor DA PARCELA (centavos) para n parcelas,
// dividindo o total. O resto de centavos vai na primeira parcela — decisão nossa,
// a Appmax não documenta o arredondamento.
func InstallmentAmount(totalCents int64, n int) (first, others int64) {
	if n <= 0 {
		return totalCents, 0
	}
	others = totalCents / int64(n)
	first = others + (totalCents - others*int64(n))
	return first, others
}

// RefundRequest solicita estorno. refundType: "total" | "partial".
// ATENÇÃO: pedido COM split só aceita estorno TOTAL.
func (c *Client) RefundRequest(ctx context.Context, orderID int64, refundType string) (json.RawMessage, error) {
	if refundType != "total" && refundType != "partial" {
		return nil, fmt.Errorf("%w: refund type inválido %q (total|partial)", psp.ErrInvalidRequest, refundType)
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/orders/refund-request", map[string]any{
		"order_id": orderID,
		"type":     refundType,
	})
}

// SetShippingTrackingCode registra o código de rastreio.
// OBRIGATÓRIO para liberar o saque do split — sem ele o saldo do recebedor não sai.
func (c *Client) SetShippingTrackingCode(ctx context.Context, orderID int64, code string) (json.RawMessage, error) {
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("%w: shipping_tracking_code vazio", psp.ErrInvalidRequest)
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/orders/shipping-tracking-code", map[string]any{
		"order_id":               orderID,
		"shipping_tracking_code": code,
	})
}

// ===================== Payment Split =====================

// SplitEntry é uma linha do split. Amount é VALOR FIXO EM CENTAVOS —
// a Appmax NÃO suporta split percentual.
type SplitEntry struct {
	Amount        int64  `json:"amount"`
	RecipientHash string `json:"recipient_hash"`
}

// DefaultSplitSafetyRatio é a fração do valor de referência que aceitamos
// distribuir. O split incide sobre o LÍQUIDO (partner_total = pedido - taxas
// Appmax) e não há endpoint público para ler o partner_total antes; se a soma
// estourar, a Appmax REDISTRIBUI PROPORCIONALMENTE EM SILÊNCIO (sem erro).
// 0.80 cobre folgadamente as taxas típicas de cartão parcelado.
const DefaultSplitSafetyRatio = 0.80

// SplitOptions parametriza a trava local do split.
type SplitOptions struct {
	// ReferenceCents é o valor BRUTO do pedido em centavos (nosso lado).
	// Obrigatório: é a base da trava.
	ReferenceCents int64
	// SafetyRatio é a fração do ReferenceCents permitida (0 = DefaultSplitSafetyRatio).
	SafetyRatio float64
}

// SplitOrder cria o split de um pedido.
//
// Regras da Appmax (aplicadas/checadas aqui):
//   - só pode ser criado APÓS criar o pedido e ANTES da aprovação;
//     é PROIBIDO em pedido já `aprovado` (validamos consultando o pedido).
//   - valores FIXOS em centavos, nunca percentual.
//   - a soma incide sobre o partner_total (líquido). Se exceder, a Appmax
//     redistribui proporcionalmente e NÃO retorna erro — por isso a trava local.
//   - pedido com split só aceita estorno TOTAL.
func (c *Client) SplitOrder(ctx context.Context, orderID int64, entries []SplitEntry, opts SplitOptions) (json.RawMessage, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: split vazio", psp.ErrInvalidRequest)
	}
	var sum int64
	for _, e := range entries {
		if e.Amount <= 0 {
			return nil, fmt.Errorf("%w: split amount deve ser > 0 (centavos)", psp.ErrInvalidRequest)
		}
		if strings.TrimSpace(e.RecipientHash) == "" {
			return nil, fmt.Errorf("%w: split recipient_hash vazio", psp.ErrInvalidRequest)
		}
		// SEGURANÇA (audit appmaxv1 2026-07-18, AV1-H2): soma com detecção de
		// overflow. Sem isto, duas entries de ~4.6e18 centavos estouram o int64,
		// `sum` fica NEGATIVO e passa direto pela comparação `sum > cap` — a
		// trava do split é contornada com dois números grandes.
		if sum > math.MaxInt64-e.Amount {
			return nil, fmt.Errorf("%w: soma do split estoura int64 — valores absurdos (%d centavos) são recusados", psp.ErrInvalidRequest, e.Amount)
		}
		sum += e.Amount
	}

	ratio := opts.SafetyRatio
	if ratio <= 0 || ratio > 1 {
		ratio = DefaultSplitSafetyRatio
	}
	if opts.ReferenceCents <= 0 {
		return nil, fmt.Errorf("%w: split ReferenceCents obrigatório (base da trava)", psp.ErrInvalidRequest)
	}
	cap := int64(math.Floor(float64(opts.ReferenceCents) * ratio))
	if sum > cap {
		// Fail-closed: sem isso a Appmax redistribuiria em silêncio e o recebedor
		// receberia um valor diferente do combinado, sem nenhum sinal de erro.
		slog.Error("appmax-v1 split bloqueado localmente: soma excede o teto seguro",
			"order_id", orderID, "sum_cents", sum, "cap_cents", cap,
			"reference_cents", opts.ReferenceCents, "safety_ratio", ratio)
		return nil, fmt.Errorf("%w: split soma %d centavos excede o teto seguro %d (ref=%d, ratio=%.2f) — a Appmax redistribuiria em silêncio",
			psp.ErrInvalidRequest, sum, cap, opts.ReferenceCents, ratio)
	}

	// Split é proibido em pedido aprovado. FAIL-CLOSED (audit AV1-H3): se não
	// conseguimos confirmar o status, NÃO enviamos o split.
	//
	// A versão anterior era `if err == nil` — ou seja, qualquer falha na consulta
	// (timeout, 5xx, ou um atacante capaz de derrubar só essa chamada) fazia a
	// verificação ser PULADA e o split seguir. Guarda que some quando o sistema
	// está sob stress é guarda que não existe.
	ov, gerr := c.GetOrder(ctx, strconv.FormatInt(orderID, 10))
	if gerr != nil {
		return nil, fmt.Errorf("%w: não foi possível confirmar o status do pedido %d antes do split (fail-closed): %v",
			psp.ErrUpstream, orderID, gerr)
	}
	// Status VAZIO é o caso traiçoeiro: um 200 com corpo ilegível (ou um shape
	// que o parser tolerante não reconhece) produz OrderView zerado, e
	// NormalizeStatus("") devolve `pending` — ou seja, "não consegui ler" ficava
	// indistinguível de "pedido pendente" e o split passava. Ausência de
	// informação NUNCA pode ser tratada como permissão.
	if ov == nil || strings.TrimSpace(ov.Status) == "" {
		return nil, fmt.Errorf("%w: pedido %d não retornou status legível — split bloqueado (fail-closed)",
			psp.ErrUpstream, orderID)
	}
	if NormalizeStatus(ov.Status) == psp.StatusApproved {
		return nil, fmt.Errorf("%w: split proibido em pedido já aprovado (status=%q)", psp.ErrInvalidRequest, ov.Status)
	}

	slog.Info("appmax-v1 split", "order_id", orderID, "recipients", len(entries), "sum_cents", sum)
	return c.doJSON(ctx, http.MethodPost,
		"/v1/orders/"+strconv.FormatInt(orderID, 10)+"/split-order",
		map[string]any{"split": entries})
}

// RecipientInput — POST /v1/recipient.
//
// ATENÇÃO: este é o ÚNICO endpoint da v1 em camelCase; todo o resto é snake_case.
// O recebedor é IMUTÁVEL depois de criado (não há endpoint de update/delete).
type RecipientInput struct {
	Triage  RecipientTriage  `json:"triage"`
	Account RecipientAccount `json:"account"`
	Company RecipientCompany `json:"company"`
}

type RecipientTriage struct {
	Revenue  any    `json:"revenue"`
	StoreURL string `json:"storeUrl"`
}

type RecipientAccount struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	CPF         string `json:"cpf"`
	Phone       string `json:"phone"`
	DateOfBirth string `json:"dateOfBirth"`
}

type RecipientCompany struct {
	CompanyName                string `json:"companyName"`
	CompanyDocumentNumber      string `json:"companyDocumentNumber"`
	CompanyPostcode            string `json:"companyPostcode"`
	CompanyAddress             string `json:"companyAddress"`
	CompanyAddressNumber       string `json:"companyAddressNumber"`
	CompanyAddressState        string `json:"companyAddressState"`
	CompanyAddressComplement   string `json:"companyAddressComplement"`
	CompanyAddressNeighborhood string `json:"companyAddressNeighborhood"`
	CompanyCity                string `json:"companyCity"`
}

// Status possíveis de um recebedor. São só estes TRÊS — não existe status de
// rejeição na API; um onboarding reprovado simplesmente nunca sai de verificação.
const (
	RecipientAwaitingFaceMatch = "Awaiting face match completion"
	RecipientOnVerification    = "Onboarding on verification"
	RecipientCompleted         = "Onboarding completed"
)

// CreateRecipient cria o recebedor do split e devolve o hash.
func (c *Client) CreateRecipient(ctx context.Context, in RecipientInput) (string, json.RawMessage, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/recipient", in)
	if err != nil {
		return "", raw, err
	}
	hash := digString(raw, "hash", "recipient_hash", "recipientHash")
	if hash == "" {
		return "", raw, fmt.Errorf("%w: recipient sem hash: %s", psp.ErrUpstream, truncate(raw, 500))
	}
	return hash, raw, nil
}

// RecipientFacematchLink dispara o link de face match por SMS.
// NOTA: o SMS NÃO é disparado em sandbox — o link vem só na resposta.
func (c *Client) RecipientFacematchLink(ctx context.Context, hash, phone string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost,
		"/v1/recipient/"+url.PathEscape(hash)+"/facematch-link",
		map[string]any{"phone": phone})
}

// RecipientStatus devolve o status do onboarding (um dos 3 Recipient* acima).
// Só RecipientCompleted pode receber split e sacar.
func (c *Client) RecipientStatus(ctx context.Context, hash string) (string, json.RawMessage, error) {
	raw, err := c.doJSON(ctx, http.MethodGet, "/v1/recipient/"+url.PathEscape(hash)+"/status", nil)
	if err != nil {
		return "", raw, err
	}
	return digString(raw, "status", "onboarding_status"), raw, nil
}

// Balances são os saldos do recebedor, em centavos.
type Balances struct {
	Available int64
	ToRelease int64
	Raw       json.RawMessage
}

// RecipientBalances lê os saldos. O JSON exato não é documentado → parser
// tolerante: aceita objeto {available,to_release} ou lista [{type,value}].
func (c *Client) RecipientBalances(ctx context.Context, hash string) (*Balances, error) {
	raw, err := c.doJSON(ctx, http.MethodGet, "/v1/recipient/"+url.PathEscape(hash)+"/balances", nil)
	if err != nil {
		return nil, err
	}
	b := &Balances{Raw: raw}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return b, nil
	}
	scope := unwrapData(m)
	if v, ok := scope["balances"]; ok {
		switch t := v.(type) {
		case map[string]any:
			scope = t
		case []any:
			for _, it := range t {
				e, ok := it.(map[string]any)
				if !ok {
					continue
				}
				val := toInt64(firstOf(e, "value", "amount", "balance"))
				switch strings.ToLower(fmt.Sprint(firstOf(e, "type", "name"))) {
				case "available":
					b.Available = val
				case "to_release", "torelease":
					b.ToRelease = val
				}
			}
			return b, nil
		}
	}
	b.Available = toInt64(firstOf(scope, "available", "available_balance", "availableBalance"))
	b.ToRelease = toInt64(firstOf(scope, "to_release", "toRelease", "to_release_balance"))
	return b, nil
}

// AVISO DE AUTORIZAÇÃO (audit AV1-M1) — LEIA ANTES DE EXPOR ISTO EM HTTP.
//
// Os três métodos abaixo MOVEM DINHEIRO PARA FORA da plataforma. Hoje eles NÃO
// têm rota HTTP: são chamáveis só a partir do processo, o que é a razão de não
// haver falha de autorização explorável no estado atual.
//
// Se um dia virarem endpoint, o mínimo obrigatório é:
//   1. role=admin (handler.AdminOnly) — nunca o JWT do vendedor direto, senão
//      quem sacaria é quem escolhe o `hash`;
//   2. checar que o recipient_hash pertence ao vendedor do JWT (IDOR: o hash é
//      o ÚNICO parâmetro, então sem essa checagem qualquer vendedor autenticado
//      saca o saldo de outro só trocando a string);
//   3. Idempotency-Key (pkg/idempotency): a Appmax v1 não tem idempotência, um
//      duplo clique é um duplo saque;
//   4. lançamento no livro (ledger.SellerWithdrawal) na mesma operação;
//   5. registro em pkg/audit com actor, IP e valor.

// maxWithdrawCents é um teto de sanidade (R$ 1.000.000,00). Não substitui
// autorização — existe pra que um bug de unidade (reais tratados como centavos,
// ou o inverso) não vire uma ordem de saque de valor absurdo.
const maxWithdrawCents int64 = 100_000_000

// validateWithdrawValue recusa valor não-positivo e absurdo ANTES da rede.
// Valor <= 0 chegava a ser enviado à Appmax: sem contrato documentado sobre
// negativos, o comportamento do outro lado é desconhecido — e "desconhecido"
// numa rota de saque é inaceitável.
func validateWithdrawValue(hash string, valueCents int64) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("%w: recipient_hash vazio", psp.ErrInvalidRequest)
	}
	if valueCents <= 0 {
		return fmt.Errorf("%w: valor de saque deve ser > 0 centavos (veio %d)", psp.ErrInvalidRequest, valueCents)
	}
	if valueCents > maxWithdrawCents {
		return fmt.Errorf("%w: valor de saque %d centavos acima do teto de sanidade %d — confira a unidade (centavos, não reais)",
			psp.ErrInvalidRequest, valueCents, maxWithdrawCents)
	}
	return nil
}

// SimulateAnticipation simula a antecipação de um valor (centavos, na query).
func (c *Client) SimulateAnticipation(ctx context.Context, hash string, valueCents int64) (json.RawMessage, error) {
	if err := validateWithdrawValue(hash, valueCents); err != nil {
		return nil, err
	}
	return c.doJSON(ctx, http.MethodGet,
		"/v1/recipient/"+url.PathEscape(hash)+"/withdraw-request/anticipation/simulate?value="+strconv.FormatInt(valueCents, 10), nil)
}

// WithdrawStatusPending é o status `2` retornado pelo saque por antecipação.
const WithdrawStatusPending = 2

// RequestAnticipation solicita saque por antecipação (status 2 = pending).
// Ver AVISO DE AUTORIZAÇÃO acima.
func (c *Client) RequestAnticipation(ctx context.Context, hash string, valueCents int64) (json.RawMessage, error) {
	if err := validateWithdrawValue(hash, valueCents); err != nil {
		return nil, err
	}
	// Log de saque é obrigatório: é a linha que permite reconstruir quem tirou
	// dinheiro e quando. O hash é identificador do recebedor, não segredo.
	slog.Info("appmax-v1 saque por antecipação solicitado",
		"recipient_hash", hash, "value_cents", valueCents,
		"request_id", requestid.FromContext(ctx))
	return c.doJSON(ctx, http.MethodPost,
		"/v1/recipient/"+url.PathEscape(hash)+"/withdraw-request/anticipation",
		map[string]any{"value": valueCents})
}

// RequestAvailableWithdraw solicita saque do saldo disponível.
// Lembrete: sem shipping-tracking-code no pedido, o saldo não é liberado.
// Ver AVISO DE AUTORIZAÇÃO acima.
func (c *Client) RequestAvailableWithdraw(ctx context.Context, hash string, valueCents int64) (json.RawMessage, error) {
	if err := validateWithdrawValue(hash, valueCents); err != nil {
		return nil, err
	}
	slog.Info("appmax-v1 saque de saldo disponível solicitado",
		"recipient_hash", hash, "value_cents", valueCents,
		"request_id", requestid.FromContext(ctx))
	return c.doJSON(ctx, http.MethodPost,
		"/v1/recipient/"+url.PathEscape(hash)+"/withdraw-request/available",
		map[string]any{"value": valueCents})
}

// ===================== Parsers tolerantes =====================
// A doc da v1 não publica JSON verbatim em vários pontos (boleto, balances),
// então navegamos por múltiplos caminhos/aliases — mesmo espírito dos helpers
// digID/digFloat/parseDisplay do pacote appmax v3.

// unwrapData desce para `data` quando presente.
func unwrapData(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return m
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func mapAt(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func str(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// digID procura um id numérico em data.<flat>, data.<nested>.id, data.id, topo.
func digID(raw []byte, flatKey, nestedKey string) int64 {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	candidates := []any{}
	if data, ok := m["data"].(map[string]any); ok {
		if nested := mapAt(data, nestedKey); nested != nil {
			candidates = append(candidates, nested["id"], nested[flatKey])
		}
		candidates = append(candidates, data[flatKey], data["id"])
	}
	candidates = append(candidates, m[flatKey], m["id"])
	for _, c := range candidates {
		if n := toInt64(c); n != 0 {
			return n
		}
	}
	return 0
}

// digString varre recursivamente (profundidade limitada) por uma das chaves.
func digString(raw []byte, keys ...string) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return digStringVal(v, keys, 0)
}

func digStringVal(v any, keys []string, depth int) string {
	if depth > 6 {
		return ""
	}
	switch t := v.(type) {
	case map[string]any:
		for _, k := range keys {
			if s, ok := t[k].(string); ok && s != "" {
				return s
			}
		}
		for _, sub := range t {
			if s := digStringVal(sub, keys, depth+1); s != "" {
				return s
			}
		}
	case []any:
		for _, sub := range t {
			if s := digStringVal(sub, keys, depth+1); s != "" {
				return s
			}
		}
	}
	return ""
}

// digFloat procura um número em data.<key> ou no topo.
func digFloat(raw []byte, key string) float64 {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return 0
	}
	scope := unwrapData(m)
	if f, ok := toFloat(scope[key]); ok {
		return f
	}
	if f, ok := toFloat(m[key]); ok {
		return f
	}
	return 0
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(math.Round(n))
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			return i
		}
	case string:
		s := strings.TrimSpace(n)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(math.Round(f))
		}
	}
	return 0
}

func toInt(v any) int { return int(toInt64(v)) }

// parsePaymentResult normaliza a resposta de /v1/payments/*.
func parsePaymentResult(raw []byte) *PaymentResult {
	r := &PaymentResult{Raw: raw}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return r
	}
	data := unwrapData(m)

	if order := mapAt(data, "order"); order != nil {
		r.OrderID = toInt64(order["id"])
		r.Status = str(order, "status")
	}
	if r.OrderID == 0 {
		r.OrderID = toInt64(firstOf(data, "order_id", "id"))
	}

	pay := mapAt(data, "payment")
	if pay == nil {
		pay = data
	}
	r.Method = str(pay, "method")
	r.Installments = toInt(firstOf(pay, "installments"))
	r.PaidAt = str(pay, "paid_at")
	if r.Status == "" {
		r.Status = str(pay, "status")
	}

	// Pix — na RESPOSTA DE API pix_qrcode é PNG base64 (sem prefixo `data:`).
	// No WEBHOOK o mesmo nome carrega uma URL. Ver ParseEvent/PixQRCodeURL.
	r.PixQRCodeB64 = str(pay, "pix_qrcode", "pix_qr_code", "qrcode")
	r.PixEMV = str(pay, "pix_emv", "pix_qrcode_text", "emv")
	r.PixExpiresAt = str(pay, "pix_expiration_date", "pix_expires_at", "expiration_date")

	// Boleto — nomes não documentados nessa página; aceitamos os aliases do webhook.
	r.BoletoURL = str(pay, "boleto_url", "pdf", "billet_url")
	r.BoletoLine = str(pay, "boleto_digitable_line", "digitable_line", "billet_digitable_line")
	if r.PixExpiresAt == "" {
		r.PixExpiresAt = str(pay, "boleto_overdue_date", "due_date")
	}

	r.UpsellHash = str(data, "upsell_hash")
	if r.UpsellHash == "" {
		r.UpsellHash = str(pay, "upsell_hash")
	}
	return r
}

// parseOrderView normaliza GET /v1/orders/{id}. Valores em CENTAVOS.
func parseOrderView(raw []byte) *OrderView {
	o := &OrderView{Raw: raw}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return o
	}
	data := unwrapData(m)

	order := mapAt(data, "order")
	if order == nil {
		order = data
	}
	o.ID = toInt64(firstOf(order, "id", "order_id"))
	o.Status = str(order, "status")
	o.TotalPaid = toInt64(firstOf(order, "total_paid", "total"))
	o.CreatedAt = str(order, "created_at")
	o.UpdatedAt = str(order, "updated_at")

	if amounts := mapAt(order, "amounts"); amounts != nil {
		o.SubTotal = toInt64(firstOf(amounts, "sub_total", "subtotal"))
		o.ShippingValue = toInt64(firstOf(amounts, "shipping_value", "shipping"))
		o.Discount = toInt64(firstOf(amounts, "discount", "discount_value"))
		o.InstallmentFee = toInt64(firstOf(amounts, "installment_fee"))
	}

	if pay := mapAt(data, "payment"); pay != nil {
		o.PaymentMethod = str(pay, "method")
		o.Installments = toInt(pay["installments"])
		o.InstallmentsAmt = toInt64(pay["installments_amount"])
		o.PaidAt = str(pay, "paid_at")
		if card := mapAt(pay, "card"); card != nil {
			o.CardBrand = str(card, "brand")
			o.CardNumber = str(card, "number")
		}
	}
	if ref := mapAt(data, "refund"); ref != nil {
		o.RefundedAt = str(ref, "refunded_at")
	}
	return o
}
