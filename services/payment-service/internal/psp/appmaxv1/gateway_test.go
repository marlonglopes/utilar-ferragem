package appmaxv1

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

func TestName(t *testing.T) {
	if got := New(Config{}).Name(); got != "appmax-v1" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestDefaultURLs(t *testing.T) {
	c := NewClient(Config{})
	if c.cfg.AuthURL != DefaultAuthURL || c.cfg.APIURL != DefaultAPIURL {
		t.Fatalf("defaults = %q / %q", c.cfg.AuthURL, c.cfg.APIURL)
	}
	// aspas do env_file do Docker + barra final devem ser limpas
	c = NewClient(Config{AuthURL: `"https://auth.sandboxappmax.com.br/"`, APIURL: " https://api.sandboxappmax.com.br "})
	if c.cfg.AuthURL != SandboxAuthURL || c.cfg.APIURL != SandboxAPIURL {
		t.Fatalf("clean = %q / %q", c.cfg.AuthURL, c.cfg.APIURL)
	}
}

// ===================== webhook: verificação =====================

func TestVerifyWebhookNoSecretAccepts(t *testing.T) {
	// A Appmax não assina nada: sem secret configurado, aceitamos e confiamos na
	// re-consulta GET /v1/orders/{id} feita pelo handler.
	g := New(Config{})
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyWebhookWithSecret(t *testing.T) {
	g := New(Config{WebhookSecret: "s3cr3t"})
	h := http.Header{}
	if err := g.VerifyWebhook([]byte(`{}`), h); !errors.Is(err, psp.ErrInvalidSignature) {
		t.Fatalf("sem header deveria falhar: %v", err)
	}
	h.Set("X-Appmax-Token", "errado")
	if err := g.VerifyWebhook([]byte(`{}`), h); !errors.Is(err, psp.ErrInvalidSignature) {
		t.Fatalf("header errado deveria falhar: %v", err)
	}
	h.Set("X-Appmax-Token", "s3cr3t")
	if err := g.VerifyWebhook([]byte(`{}`), h); err != nil {
		t.Fatalf("header correto: %v", err)
	}
}

// ===================== webhook: eventos =====================

func webhook(event, status string) []byte {
	return []byte(`{"event":"` + event + `","event_type":"order","site_id":1,"app_id":"app_1",
		"client_key":"ck","external_key":"ek",
		"data":{"order_id":4242,"status":"` + status + `","total":20330,"freight_value":1000,
		"merchant_total":19000,"discount":0,"interest":0,"paid_at":"2026-07-18 12:00:00"},
		"partner_merchant":{"id":9}}`)
}

func TestWebhookEventStatusMapping(t *testing.T) {
	cases := []struct {
		event string
		want  psp.PaymentStatus
	}{
		{"order_approved", psp.StatusApproved},
		{"order_paid", psp.StatusApproved},
		{"order_paid_by_pix", psp.StatusApproved},
		{"order_integrated", psp.StatusApproved},
		{"order_charge_back_gain", psp.StatusApproved},
		{"order_authorized", psp.StatusAuthorized},
		{"payment_authorized_with_delay", psp.StatusAuthorized},
		{"order_refund", psp.StatusCancelled},
		{"order_partial_refund", psp.StatusCancelled},
		{"order_pix_expired", psp.StatusExpired},
		{"order_billet_overdue", psp.StatusExpired},
		{"order_refused_by_risk", psp.StatusRejected},
		{"payment_not_authorized", psp.StatusRejected},
		// Sem desfecho → cai no status do pedido (aqui "pendente").
		{"order_billet_created", psp.StatusPending},
		{"order_pix_created", psp.StatusPending},
		{"order_chargeback_in_treatment", psp.StatusPending},
		{"split_orders", psp.StatusPending},
	}
	g := New(Config{})
	for _, tc := range cases {
		ev, err := g.ParseWebhookEvent(webhook(tc.event, "pendente"))
		if err != nil {
			t.Fatalf("%s: %v", tc.event, err)
		}
		if ev == nil {
			t.Fatalf("%s: evento nulo", tc.event)
		}
		if ev.Status != tc.want {
			t.Errorf("%s: status = %q, want %q", tc.event, ev.Status, tc.want)
		}
		if ev.PSPID != "4242" {
			t.Errorf("%s: pspID = %q", tc.event, ev.PSPID)
		}
		// total vem em centavos no webhook → reais no evento normalizado
		if ev.Amount != 203.30 {
			t.Errorf("%s: amount = %v", tc.event, ev.Amount)
		}
	}
}

// Um evento sem desfecho próprio deve herdar o status do pedido.
func TestWebhookFallsBackToOrderStatus(t *testing.T) {
	g := New(Config{})
	ev, err := g.ParseWebhookEvent(webhook("order_billet_created", "aprovado"))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Status != psp.StatusApproved {
		t.Fatalf("status = %q", ev.Status)
	}
}

func TestWebhookWithoutOrderIDIsIgnored(t *testing.T) {
	g := New(Config{})
	ev, err := g.ParseWebhookEvent([]byte(`{"event":"customer_created","event_type":"customer","data":{"id":0,"email":"a@b.c"}}`))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ev != nil {
		t.Fatalf("evento sem order id deveria ser ignorado: %+v", ev)
	}
}

func TestWebhookInvalidJSON(t *testing.T) {
	g := New(Config{})
	if _, err := g.ParseWebhookEvent([]byte(`{nope`)); !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseEventPixPayload(t *testing.T) {
	body := []byte(`{"event":"order_pix_created","event_type":"order","data":{
		"order_id":10,"status":"pendente","total":5000,
		"products":[{"sku":"SKU1","name":"Martelo","price":2500,"quantity":2}],
		"cashback_used":300,"cashback_reserved":150,"cashback_status":"reserved",
		"payment_info":{"pix":{"end_to_end_id":"E123","pix_expiration_date":"2026-07-19 10:00:00",
			"pix_emv":"00020126...","pix_qrcode":"https://cdn.appmax.com.br/qr/abc.png",
			"pix_payment_link":"https://pay.appmax.com.br/abc"}}}}`)
	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentMethod != "pix" {
		t.Fatalf("method = %q", ev.PaymentMethod)
	}
	// No WEBHOOK pix_qrcode é URL (na resposta da API é base64) — campos distintos.
	if ev.PixQRCodeURL != "https://cdn.appmax.com.br/qr/abc.png" {
		t.Errorf("PixQRCodeURL = %q", ev.PixQRCodeURL)
	}
	if ev.PixEMV != "00020126..." || ev.PixEndToEndID != "E123" || ev.PixPaymentLink == "" {
		t.Errorf("pix = %+v", ev)
	}
	// Cashback é read-only e só existe aqui.
	if ev.CashbackUsed != 300 || ev.CashbackReserved != 150 || ev.CashbackStatus != "reserved" {
		t.Errorf("cashback = %d/%d/%q", ev.CashbackUsed, ev.CashbackReserved, ev.CashbackStatus)
	}
	if len(ev.Products) != 1 || ev.Products[0].SKU != "SKU1" || ev.Products[0].Price != 2500 || ev.Products[0].Quantity != 2 {
		t.Errorf("products = %+v", ev.Products)
	}
}

// O mesmo nome `pix_qrcode` carrega tipos diferentes nos dois lugares — este
// teste trava essa diferença para que ninguém "unifique" os dois campos.
func TestPixQRCodeBase64VsURL(t *testing.T) {
	apiResp := []byte(`{"data":{"payment":{"method":"pix","pix_qrcode":"iVBORw0KGgoAAAANSUhEUg==","pix_emv":"0002..."}}}`)
	pr := parsePaymentResult(apiResp)
	if pr.PixQRCodeB64 != "iVBORw0KGgoAAAANSUhEUg==" {
		t.Fatalf("API: base64 = %q", pr.PixQRCodeB64)
	}

	hook := []byte(`{"event":"order_pix_created","data":{"order_id":1,"payment_info":{"pix":{"pix_qrcode":"https://cdn/x.png"}}}}`)
	ev, _ := ParseEvent(hook)
	if ev.PixQRCodeURL != "https://cdn/x.png" {
		t.Fatalf("webhook: url = %q", ev.PixQRCodeURL)
	}
}

func TestParseEventCardPayload(t *testing.T) {
	body := []byte(`{"event":"order_approved","event_type":"order","data":{"order_id":11,"status":"aprovado","total":100,
		"payment_info":{"credit_card":{"installments":3,"card_brand":"visa","nsu":"123456",
		"authorization_code":"A1B2","captured_at":"2026-07-18 12:00:00"}}}}`)
	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentMethod != "credit_card" || ev.CardInstallments != 3 || ev.CardBrand != "visa" ||
		ev.CardNSU != "123456" || ev.CardAuthorization != "A1B2" || ev.CardCapturedAt == "" {
		t.Fatalf("card = %+v", ev)
	}
}

func TestParseEventBoletoPayload(t *testing.T) {
	body := []byte(`{"event":"order_billet_created","event_type":"order","data":{"order_id":12,"status":"pendente",
		"payment_info":{"boleto":{"boleto_overdue_date":"2026-07-25","boleto_url":"https://x/b.pdf",
		"boleto_digitable_line":"34191790010104351004791020150008"}}}}`)
	ev, err := ParseEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentMethod != "boleto" || ev.BoletoURL != "https://x/b.pdf" ||
		ev.BoletoLine != "34191790010104351004791020150008" || ev.BoletoOverdueDate != "2026-07-25" {
		t.Fatalf("boleto = %+v", ev)
	}
}

func TestParseEventNestedOrder(t *testing.T) {
	// Alguns eventos aninham em data.order — o parser precisa achar mesmo assim.
	ev, err := ParseEvent([]byte(`{"event":"order_paid","data":{"order":{"id":88,"status":"aprovado","total":999}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrderID != "88" || ev.Status != "aprovado" || ev.TotalCents != 999 {
		t.Fatalf("ev = %+v", ev)
	}
}

func TestWebhookRawBodyPreserved(t *testing.T) {
	body := webhook("order_paid", "aprovado")
	g := New(Config{})
	ev, _ := g.ParseWebhookEvent(body)
	var m map[string]any
	if err := json.Unmarshal(ev.RawBody, &m); err != nil {
		t.Fatal(err)
	}
	if m["client_key"] != "ck" || m["external_key"] != "ek" {
		t.Fatalf("raw body perdeu campos: %v", m)
	}
}

// ===================== status =====================

func TestNormalizeStatusCoversAllDocumented(t *testing.T) {
	cases := map[string]psp.PaymentStatus{
		"pendente":                       psp.StatusPending,
		"aprovado":                       psp.StatusApproved,
		"autorizado":                     psp.StatusAuthorized,
		"cancelado":                      psp.StatusCancelled,
		"estornado":                      psp.StatusCancelled,
		"recusado_por_risco":             psp.StatusRejected,
		"integrado":                      psp.StatusApproved,
		"pendente_integracao":            psp.StatusApproved,
		"pendente_integracao_em_analise": psp.StatusPending,
		"chargeback_em_tratativa":        psp.StatusPending,
		"chargeback_em_disputa":          psp.StatusPending,
		"chargeback_perdido":             psp.StatusCancelled,
		"chargeback_vencido":             psp.StatusExpired,
		"APROVADO":                       psp.StatusApproved,
		"":                               psp.StatusPending,
	}
	for in, want := range cases {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreatePaymentRejectsZeroAmount(t *testing.T) {
	g := New(Config{ClientID: "a", ClientSecret: "b"})
	if _, err := g.CreatePayment(t.Context(), psp.CreateRequest{Amount: 0, Method: psp.MethodPix}); !errors.Is(err, psp.ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestSplitNameAndDigits(t *testing.T) {
	if f, l := splitName("  "); f != "Cliente" || l != "UtiLar" {
		t.Errorf("splitName vazio = %q %q", f, l)
	}
	if f, l := splitName("Ana"); f != "Ana" || l != "Ana" {
		t.Errorf("splitName único = %q %q", f, l)
	}
	if f, l := splitName("Ana Maria Souza"); f != "Ana" || l != "Maria Souza" {
		t.Errorf("splitName = %q %q", f, l)
	}
	if got := digitsOnly("(11) 99999-8888"); got != "11999998888" {
		t.Errorf("digitsOnly = %q", got)
	}
	if got := shortRef("aaaaaaaa-bbbb-cccc"); got != "aaaaaaaa" {
		t.Errorf("shortRef = %q", got)
	}
}
