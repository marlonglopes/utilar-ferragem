package appmaxv1

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

// ===================== harness =====================

// stub é um servidor httptest que fala OAuth2 + /v1/*.
type stub struct {
	srv *httptest.Server

	mu        sync.Mutex
	tokenHits int32
	calls     []call
	handler   func(w http.ResponseWriter, r *http.Request, body []byte) bool
	tokenTTL  int64
}

type call struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   string
}

func newStub(t *testing.T) *stub {
	t.Helper()
	s := &stub{tokenTTL: 3600}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&s.tokenHits, 1)
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("token content-type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=client_credentials") {
			t.Errorf("token body = %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-` + itoa(int(n)) + `","token_type":"Bearer","expires_in":` + itoa(int(s.tokenTTL)) + `}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.calls = append(s.calls, call{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(body)})
		h := s.handler
		s.mu.Unlock()
		if h != nil && h(w, r, body) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no stub for ` + r.URL.Path + `"}`))
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *stub) on(f func(w http.ResponseWriter, r *http.Request, body []byte) bool) {
	s.mu.Lock()
	s.handler = f
	s.mu.Unlock()
}

func (s *stub) callsFor(path string) []call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []call
	for _, c := range s.calls {
		if c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// client monta um Client apontado pro stub, sem dormir de verdade no backoff.
func (s *stub) client(t *testing.T) (*Client, *[]time.Duration) {
	t.Helper()
	c := NewClient(Config{
		AuthURL:      s.srv.URL,
		APIURL:       s.srv.URL,
		ClientID:     "cid",
		ClientSecret: "csecret",
		BackoffBase:  10 * time.Millisecond,
	})
	var slept []time.Duration
	var mu sync.Mutex
	c.sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		slept = append(slept, d)
		mu.Unlock()
		return nil
	}
	return c, &slept
}

func jsonRespond(w http.ResponseWriter, status int, body string) bool {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
	return true
}

// ===================== conversão de valores =====================

func TestToCentsRounds(t *testing.T) {
	cases := []struct {
		reais float64
		want  int64
	}{
		{19.99, 1999}, // 19.99*100 = 1998.9999… — truncar perderia 1 centavo
		{0.1, 10},     //
		{1234.565, 123457},
		{0, 0},
		{203.30, 20330},
	}
	for _, c := range cases {
		if got := ToCents(c.reais); got != c.want {
			t.Errorf("ToCents(%v) = %d, want %d", c.reais, got, c.want)
		}
	}
	if got := FromCents(20330); got != 203.30 {
		t.Errorf("FromCents(20330) = %v", got)
	}
}

func TestInstallmentAmount(t *testing.T) {
	first, others := InstallmentAmount(21147, 3)
	if others != 7049 || first != 7049 {
		t.Fatalf("3x de 21147: first=%d others=%d", first, others)
	}
	first, others = InstallmentAmount(1000, 3)
	if others != 333 || first != 334 || first+2*others != 1000 {
		t.Fatalf("3x de 1000: first=%d others=%d", first, others)
	}
}

// ===================== token =====================

func TestTokenCachedAcrossCalls(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		return jsonRespond(w, 200, `{"data":{"order":{"id":7,"status":"pendente"}}}`)
	})
	c, _ := s.client(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := c.GetOrder(ctx, "7"); err != nil {
			t.Fatalf("GetOrder: %v", err)
		}
	}
	if n := atomic.LoadInt32(&s.tokenHits); n != 1 {
		t.Fatalf("token pedido %d vezes, esperado 1 (cache)", n)
	}
	for _, cl := range s.callsFor("/v1/orders/7") {
		if cl.Auth != "Bearer tok-1" {
			t.Fatalf("Authorization = %q", cl.Auth)
		}
	}
}

func TestTokenProactiveRenewal(t *testing.T) {
	s := newStub(t)
	s.tokenTTL = 3600
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		return jsonRespond(w, 200, `{"data":{"order":{"id":1}}}`)
	})
	c, _ := s.client(t)
	ctx := context.Background()

	if _, err := c.Token(ctx); err != nil {
		t.Fatal(err)
	}
	// expires_in=3600 com skew de 5min → cache deve valer ~55min.
	c.mu.RLock()
	ttl := time.Until(c.tokenExpiry)
	c.mu.RUnlock()
	if ttl < 54*time.Minute || ttl > 55*time.Minute+time.Second {
		t.Fatalf("janela do cache = %v, esperado ~55min", ttl)
	}

	// Simula a chegada da janela de renovação: falta menos de 5min de vida real.
	c.mu.Lock()
	c.tokenExpiry = time.Now().Add(-time.Second)
	c.mu.Unlock()
	if _, err := c.GetOrder(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&s.tokenHits); n != 2 {
		t.Fatalf("token hits = %d, esperado 2 (renovação proativa)", n)
	}
}

func TestTokenConcurrentSingleFetch(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		return jsonRespond(w, 200, `{"data":{"order":{"id":1}}}`)
	})
	c, _ := s.client(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Token(context.Background())
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&s.tokenHits); n != 1 {
		t.Fatalf("token hits = %d, esperado 1 (thundering herd travado)", n)
	}
}

func TestReauthOn401(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		if atomic.AddInt32(&hits, 1) == 1 {
			return jsonRespond(w, 401, `{"error":"token expirado"}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":42,"status":"aprovado","total_paid":20330}}}`)
	})
	c, _ := s.client(t)

	ov, err := c.GetOrder(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetOrder após re-auth: %v", err)
	}
	if ov.TotalCents() != 20330 {
		t.Fatalf("total = %d", ov.TotalCents())
	}
	if n := atomic.LoadInt32(&s.tokenHits); n != 2 {
		t.Fatalf("token hits = %d, esperado 2 (re-auth após 401)", n)
	}
	calls := s.callsFor("/v1/orders/42")
	if len(calls) != 2 || calls[0].Auth == calls[1].Auth {
		t.Fatalf("esperado 2 chamadas com tokens distintos: %+v", calls)
	}
}

func TestPersistent401BecomesInvalidRequest(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		return jsonRespond(w, 401, `{"error":"credenciais inválidas"}`)
	})
	c, _ := s.client(t)
	_, err := c.GetOrder(context.Background(), "1")
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("err = %v, esperado ErrInvalidRequest", err)
	}
	if !strings.Contains(err.Error(), "credenciais inválidas") {
		t.Fatalf("erro deve preservar o corpo: %v", err)
	}
}

// ===================== rate limit =====================

func TestRetryAfterBackoff(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "0")
			return jsonRespond(w, 429, `{"error":"rate limit","retryAfter":2}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":9,"status":"pendente"}}}`)
	})
	c, slept := s.client(t)

	if _, err := c.GetOrder(context.Background(), "9"); err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if len(*slept) != 1 || (*slept)[0] != 2*time.Second {
		t.Fatalf("backoff = %v, esperado [2s] (Retry-After respeitado)", *slept)
	}
}

func TestRetryAfterFromBodyWhenHeaderMissing(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		if atomic.AddInt32(&hits, 1) == 1 {
			return jsonRespond(w, 429, `{"error":"slow down","retryAfter":7}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":9}}}`)
	})
	c, slept := s.client(t)
	if _, err := c.GetOrder(context.Background(), "9"); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 7*time.Second {
		t.Fatalf("backoff = %v, esperado [7s] do corpo", *slept)
	}
}

func TestExponentialBackoffWithoutRetryAfter(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		if atomic.AddInt32(&hits, 1) <= 3 {
			return jsonRespond(w, 429, `{"error":"rate limit"}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":9}}}`)
	})
	c, slept := s.client(t)
	if _, err := c.GetOrder(context.Background(), "9"); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond}
	if len(*slept) != 3 {
		t.Fatalf("backoffs = %v", *slept)
	}
	for i, w := range want {
		if (*slept)[i] != w {
			t.Fatalf("backoff[%d] = %v, want %v (exponencial)", i, (*slept)[i], w)
		}
	}
}

// Rota financeira não pode ser re-tentada em 5xx: sem idempotência na API,
// um retry pode virar cobrança dupla.
func TestFinancialRouteNotRetriedOn5xx(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		atomic.AddInt32(&hits, 1)
		return jsonRespond(w, 500, `{"error":"boom"}`)
	})
	c, _ := s.client(t)
	if _, err := c.PayPix(context.Background(), 1, "12345678909"); err == nil {
		t.Fatal("esperado erro")
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("chamadas = %d, esperado 1 (sem retry em rota financeira)", n)
	}
}

// ===================== fluxos =====================

func TestPixFlowEndToEnd(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		switch r.URL.Path {
		case "/v1/customers":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			for _, k := range []string{"first_name", "last_name", "email", "phone", "ip"} {
				if in[k] == nil || in[k] == "" {
					t.Errorf("customer sem campo obrigatório %q: %s", k, body)
				}
			}
			return jsonRespond(w, 201, `{"data":{"customer":{"id":1}}}`)
		case "/v1/orders":
			var in OrderInput
			_ = json.Unmarshal(body, &in)
			if in.ProductsValue != 20330 {
				t.Errorf("products_value = %d, esperado 20330 centavos", in.ProductsValue)
			}
			if len(in.Products) != 1 || in.Products[0].UnitValue != 20330 || in.Products[0].Type == "" {
				t.Errorf("products inválidos: %+v", in.Products)
			}
			return jsonRespond(w, 201, `{"data":{"order":{"id":555,"status":"pendente"}}}`)
		case "/v1/payments/pix":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if toInt64(in["order_id"]) != 555 {
				t.Errorf("order_id = %v", in["order_id"])
			}
			return jsonRespond(w, 200, `{"data":{"order":{"id":555,"status":"pendente"},"payment":{"method":"pix","pix_qrcode":"iVBORw0KGgoAAAANS","pix_emv":"00020126BR...6304ABCD","pix_expiration_date":"2026-07-19 10:00:00"}}}`)
		}
		return false
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}

	res, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Amount:     203.30,
		Method:     psp.MethodPix,
		PayerName:  "Maria Silva Souza",
		PayerEmail: "maria@example.com",
		PayerCPF:   "123.456.789-09",
		PayerPhone: "(11) 99999-8888",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if res.PSPID != "555" {
		t.Fatalf("PSPID = %q", res.PSPID)
	}
	if res.Status != psp.StatusPending {
		t.Fatalf("status = %q", res.Status)
	}

	var cd map[string]any
	if err := json.Unmarshal(res.ClientData, &cd); err != nil {
		t.Fatal(err)
	}
	if cd["provider"] != ProviderName {
		t.Errorf("provider = %v", cd["provider"])
	}
	// base64 CRU, sem prefixo data: — o front monta o data URI.
	if cd["pix_qrcode"] != "iVBORw0KGgoAAAANS" || strings.HasPrefix(cd["pix_qrcode"].(string), "data:") {
		t.Errorf("pix_qrcode = %v", cd["pix_qrcode"])
	}
	if cd["pix_emv"] != "00020126BR...6304ABCD" {
		t.Errorf("pix_emv = %v", cd["pix_emv"])
	}
	if cd["installments"] != float64(1) {
		t.Errorf("installments = %v", cd["installments"])
	}
}

func TestBoletoFlowTolerantFieldNames(t *testing.T) {
	// A doc não publica o JSON do boleto; aceitamos os aliases do webhook.
	variants := []string{
		`{"data":{"payment":{"boleto_url":"https://x/b.pdf","boleto_digitable_line":"34191...","boleto_overdue_date":"2026-07-25"}}}`,
		`{"data":{"payment":{"pdf":"https://x/b.pdf","digitable_line":"34191...","due_date":"2026-07-25"}}}`,
	}
	for i, v := range variants {
		s := newStub(t)
		body := v
		s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
			switch r.URL.Path {
			case "/v1/customers":
				return jsonRespond(w, 201, `{"data":{"customer":{"id":2}}}`)
			case "/v1/orders":
				return jsonRespond(w, 201, `{"data":{"order":{"id":77,"status":"pendente"}}}`)
			case "/v1/payments/boleto":
				return jsonRespond(w, 200, body)
			}
			return false
		})
		c, _ := s.client(t)
		g := &Gateway{client: c}
		res, err := g.CreatePayment(context.Background(), psp.CreateRequest{
			OrderID: "order-1", Amount: 100, Method: psp.MethodBoleto,
			PayerName: "Joao Teste", PayerEmail: "j@x.com", PayerCPF: "12345678909", PayerPhone: "11999998888",
		})
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		var cd map[string]any
		_ = json.Unmarshal(res.ClientData, &cd)
		if cd["boleto_url"] != "https://x/b.pdf" || cd["boleto_line"] != "34191..." {
			t.Fatalf("variant %d: clientData = %v", i, cd)
		}
	}
}

func TestBoletoRequiresCPF(t *testing.T) {
	s := newStub(t)
	c, _ := s.client(t)
	g := &Gateway{client: c}
	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		Amount: 10, Method: psp.MethodBoleto, PayerName: "X Y", PayerEmail: "a@b.c", PayerPhone: "11999998888",
	})
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestCardFlow(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		switch r.URL.Path {
		case "/v1/customers":
			return jsonRespond(w, 201, `{"data":{"customer":{"id":3}}}`)
		case "/v1/orders":
			return jsonRespond(w, 201, `{"data":{"order":{"id":900,"status":"pendente"}}}`)
		case "/v1/payments/credit-card":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			pd, _ := in["payment_data"].(map[string]any)
			cc, _ := pd["credit_card"].(map[string]any)
			if cc["token"] != "tok_browser_123" {
				t.Errorf("token = %v", cc["token"])
			}
			if cc["holder_document_number"] != "12345678909" {
				t.Errorf("holder_document_number = %v", cc["holder_document_number"])
			}
			if toInt(cc["installments"]) != 1 {
				t.Errorf("installments = %v", cc["installments"])
			}
			return jsonRespond(w, 200, `{"data":{"order":{"id":900,"status":"aprovado"},"payment":{"method":"creditcard","installments":3,"paid_at":"2026-07-18 12:00:00"},"upsell_hash":"uh_abc"}}`)
		}
		return false
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}

	res, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "o-1", Amount: 500, Method: psp.MethodCard, CardToken: "tok_browser_123",
		PayerName: "Ana Paula", PayerEmail: "a@b.c", PayerCPF: "12345678909", PayerPhone: "11999998888",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if res.Status != psp.StatusApproved {
		t.Fatalf("status = %q", res.Status)
	}
	var cd map[string]any
	_ = json.Unmarshal(res.ClientData, &cd)
	if cd["installments"] != float64(3) {
		t.Fatalf("installments = %v", cd["installments"])
	}
}

func TestCardRequiresToken(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		switch r.URL.Path {
		case "/v1/customers":
			return jsonRespond(w, 201, `{"data":{"customer":{"id":3}}}`)
		case "/v1/orders":
			return jsonRespond(w, 201, `{"data":{"order":{"id":1}}}`)
		}
		return false
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}
	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		Amount: 10, Method: psp.MethodCard, PayerName: "A B", PayerEmail: "a@b.c", PayerCPF: "12345678909", PayerPhone: "11999998888",
	})
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenize(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if r.URL.Path != "/v1/payments/tokenize" {
			return false
		}
		if !strings.Contains(string(body), `"expiration_month":"12"`) {
			t.Errorf("body = %s", body)
		}
		return jsonRespond(w, 200, `{"data":{"token":"tok_single_use"}}`)
	})
	c, _ := s.client(t)
	tok, _, err := c.Tokenize(context.Background(), CardInput{
		Number: "4111111111111111", CVV: "123", ExpirationMonth: "12", ExpirationYear: "2030", HolderName: "ANA P",
	})
	if err != nil || tok != "tok_single_use" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestGetOrderParsesAmountsInCents(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if r.URL.Path != "/v1/orders/321" {
			return false
		}
		return jsonRespond(w, 200, `{"data":{
			"order":{"id":321,"status":"aprovado","total_paid":21147,
			  "amounts":{"sub_total":20000,"shipping_value":330,"discount":0,"installment_fee":817},
			  "created_at":"2026-07-18 10:00:00","updated_at":"2026-07-18 10:05:00"},
			"customer":{"id":1},
			"payment":{"method":"creditcard","installments":3,"installments_amount":7049,
			  "card":{"brand":"visa","number":"411111******1111"},"paid_at":"2026-07-18 10:05:00"},
			"refund":{"refunded_at":null}}}`)
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}

	got, err := g.GetPayment(context.Background(), "321")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}
	if got.Status != psp.StatusApproved {
		t.Errorf("status = %q", got.Status)
	}
	// 21147 centavos → 211.47 reais.
	if got.Amount != 211.47 {
		t.Errorf("amount = %v, esperado 211.47", got.Amount)
	}

	ov, _ := c.GetOrder(context.Background(), "321")
	if ov.Installments != 3 || ov.CardBrand != "visa" || ov.InstallmentsAmt != 7049 {
		t.Errorf("payment: %+v", ov)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		return jsonRespond(w, 404, `{"error":"order not found"}`)
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}
	_, err := g.GetPayment(context.Background(), "999")
	if !errors.Is(err, psp.ErrNotFound) {
		t.Fatalf("err = %v, esperado ErrNotFound", err)
	}
}

func TestInstallmentsTable(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if r.URL.Path != "/v1/payments/installments" {
			return false
		}
		if !strings.Contains(string(body), `"total_value":20330`) {
			t.Errorf("total_value deve ir em centavos: %s", body)
		}
		return jsonRespond(w, 200, `{"data":{"installments":{"1":{"total":20330},"3":{"total":21147}}}}`)
	})
	c, _ := s.client(t)
	tbl, _, err := c.Installments(context.Background(), 3, 20330, "PP")
	if err != nil {
		t.Fatal(err)
	}
	if tbl[1] != 20330 || tbl[3] != 21147 {
		t.Fatalf("tabela = %v", tbl)
	}
}

func TestRefundAndTracking(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		switch r.URL.Path {
		case "/v1/orders/refund-request":
			if !strings.Contains(string(body), `"type":"total"`) {
				t.Errorf("body = %s", body)
			}
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		case "/v1/orders/shipping-tracking-code":
			if !strings.Contains(string(body), `"shipping_tracking_code":"BR123"`) {
				t.Errorf("body = %s", body)
			}
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return false
	})
	c, _ := s.client(t)
	ctx := context.Background()
	if _, err := c.RefundRequest(ctx, 5, "total"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RefundRequest(ctx, 5, "meia"); !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("refund type inválido deveria falhar: %v", err)
	}
	if _, err := c.SetShippingTrackingCode(ctx, 5, "BR123"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetShippingTrackingCode(ctx, 5, "  "); !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("tracking vazio deveria falhar: %v", err)
	}
}

// ===================== split =====================

func TestSplitBlockedWhenSumExceedsCap(t *testing.T) {
	s := newStub(t)
	var splitHits int32
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/split-order") {
			atomic.AddInt32(&splitHits, 1)
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":10,"status":"pendente"}}}`)
	})
	c, _ := s.client(t)

	// Pedido de R$100,00 (10000 centavos). Teto seguro = 80% = 8000.
	_, err := c.SplitOrder(context.Background(), 10,
		[]SplitEntry{{Amount: 5000, RecipientHash: "r1"}, {Amount: 4000, RecipientHash: "r2"}},
		SplitOptions{ReferenceCents: 10000})
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("split acima do teto deveria ser bloqueado: %v", err)
	}
	if !strings.Contains(err.Error(), "9000") || !strings.Contains(err.Error(), "8000") {
		t.Errorf("erro deve mostrar soma e teto: %v", err)
	}
	if n := atomic.LoadInt32(&splitHits); n != 0 {
		t.Fatalf("split não deveria ter ido pra rede (%d chamadas)", n)
	}
}

func TestSplitAcceptedWithinCap(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/split-order") {
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			arr, _ := in["split"].([]any)
			if len(arr) != 2 {
				t.Errorf("split = %s", body)
			}
			first, _ := arr[0].(map[string]any)
			// valor FIXO em centavos, nunca percentual
			if toInt64(first["amount"]) != 3000 || first["recipient_hash"] != "r1" {
				t.Errorf("entry = %v", first)
			}
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":10,"status":"pendente"}}}`)
	})
	c, _ := s.client(t)
	if _, err := c.SplitOrder(context.Background(), 10,
		[]SplitEntry{{Amount: 3000, RecipientHash: "r1"}, {Amount: 2000, RecipientHash: "r2"}},
		SplitOptions{ReferenceCents: 10000}); err != nil {
		t.Fatalf("split dentro do teto: %v", err)
	}
}

func TestSplitRejectedOnApprovedOrder(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/split-order") {
			t.Error("split não deveria ser enviado para pedido aprovado")
			return jsonRespond(w, 200, `{}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":10,"status":"aprovado"}}}`)
	})
	c, _ := s.client(t)
	_, err := c.SplitOrder(context.Background(), 10,
		[]SplitEntry{{Amount: 1000, RecipientHash: "r1"}}, SplitOptions{ReferenceCents: 10000})
	if !errors.Is(err, psp.ErrInvalidRequest) || !strings.Contains(err.Error(), "aprovado") {
		t.Fatalf("err = %v", err)
	}
}

func TestSplitValidatesEntries(t *testing.T) {
	s := newStub(t)
	c, _ := s.client(t)
	ctx := context.Background()
	cases := []struct {
		name    string
		entries []SplitEntry
		opts    SplitOptions
	}{
		{"vazio", nil, SplitOptions{ReferenceCents: 100}},
		{"amount zero", []SplitEntry{{Amount: 0, RecipientHash: "r"}}, SplitOptions{ReferenceCents: 100}},
		{"hash vazio", []SplitEntry{{Amount: 10, RecipientHash: " "}}, SplitOptions{ReferenceCents: 100}},
		{"sem referência", []SplitEntry{{Amount: 10, RecipientHash: "r"}}, SplitOptions{}},
	}
	for _, tc := range cases {
		if _, err := c.SplitOrder(ctx, 1, tc.entries, tc.opts); !errors.Is(err, psp.ErrInvalidRequest) {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

// ===================== recipients =====================

func TestRecipientCamelCasePayload(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if r.URL.Path != "/v1/recipient" {
			return false
		}
		// Este endpoint é o único em camelCase.
		for _, k := range []string{`"storeUrl"`, `"dateOfBirth"`, `"companyDocumentNumber"`, `"companyAddressNeighborhood"`} {
			if !strings.Contains(string(body), k) {
				t.Errorf("payload deve ser camelCase, faltou %s: %s", k, body)
			}
		}
		return jsonRespond(w, 201, `{"data":{"recipient":{"hash":"rcp_abc123"}}}`)
	})
	c, _ := s.client(t)
	hash, _, err := c.CreateRecipient(context.Background(), RecipientInput{
		Triage:  RecipientTriage{Revenue: 100000, StoreURL: "https://utilar.com.br"},
		Account: RecipientAccount{Email: "s@x.com", Name: "Seller", CPF: "12345678909", Phone: "5511999998888", DateOfBirth: "1990-01-01"},
		Company: RecipientCompany{CompanyName: "Seller ME", CompanyDocumentNumber: "12345678000199", CompanyAddressNeighborhood: "Centro"},
	})
	if err != nil || hash != "rcp_abc123" {
		t.Fatalf("hash=%q err=%v", hash, err)
	}
}

func TestRecipientStatusAndBalances(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			return jsonRespond(w, 200, `{"data":{"status":"Onboarding completed"}}`)
		case strings.HasSuffix(r.URL.Path, "/balances"):
			return jsonRespond(w, 200, `{"data":{"balances":[{"type":"available","value":15000},{"type":"to_release","value":2500}]}}`)
		}
		return false
	})
	c, _ := s.client(t)
	ctx := context.Background()

	st, _, err := c.RecipientStatus(ctx, "rcp_abc")
	if err != nil || st != RecipientCompleted {
		t.Fatalf("status=%q err=%v", st, err)
	}
	b, err := c.RecipientBalances(ctx, "rcp_abc")
	if err != nil || b.Available != 15000 || b.ToRelease != 2500 {
		t.Fatalf("balances = %+v err=%v", b, err)
	}
}

func TestRecipientBalancesObjectShape(t *testing.T) {
	// Shape alternativo — o JSON não é documentado, o parser precisa tolerar.
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		return jsonRespond(w, 200, `{"data":{"available":900,"to_release":100}}`)
	})
	c, _ := s.client(t)
	b, err := c.RecipientBalances(context.Background(), "h")
	if err != nil || b.Available != 900 || b.ToRelease != 100 {
		t.Fatalf("balances = %+v err=%v", b, err)
	}
}

func TestWithdrawEndpoints(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		switch {
		case strings.HasSuffix(r.URL.Path, "/anticipation/simulate"):
			if r.URL.Query().Get("value") != "10000" {
				t.Errorf("query = %q (centavos)", r.URL.RawQuery)
			}
			return jsonRespond(w, 200, `{"data":{"value":10000,"fee":250}}`)
		case strings.HasSuffix(r.URL.Path, "/withdraw-request/anticipation"):
			if !strings.Contains(string(body), `"value":10000`) {
				t.Errorf("body = %s", body)
			}
			return jsonRespond(w, 200, `{"data":{"status":2}}`)
		case strings.HasSuffix(r.URL.Path, "/withdraw-request/available"):
			return jsonRespond(w, 200, `{"data":{"status":2}}`)
		}
		return false
	})
	c, _ := s.client(t)
	ctx := context.Background()
	if _, err := c.SimulateAnticipation(ctx, "h", 10000); err != nil {
		t.Fatal(err)
	}
	raw, err := c.RequestAnticipation(ctx, "h", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if int(digFloat(raw, "status")) != WithdrawStatusPending {
		t.Fatalf("status = %s", raw)
	}
	if _, err := c.RequestAvailableWithdraw(ctx, "h", 5000); err != nil {
		t.Fatal(err)
	}
}
