package appmax

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

// Testes de integração AO VIVO contra o sandbox da Appmax v3.
//
// SÓ rodam quando APPMAX_ACCESS_TOKEN está setado (senão SKIPam) — não afetam
// CI/testes normais. Base URL vem de APPMAX_BASE_URL (sandbox):
//
//	APPMAX_ACCESS_TOKEN=... \
//	APPMAX_BASE_URL=https://homolog.sandboxappmax.com.br/api/v3 \
//	go test ./services/payment-service/internal/psp/appmax/ -run Integration -v
//
// Criam registros no SANDBOX (sem dinheiro real). CPF de teste válido é
// obrigatório (o antifraude rejeita CPF repetido/ inválido).

func liveClient(t *testing.T) *Client {
	t.Helper()
	tok := os.Getenv("APPMAX_ACCESS_TOKEN")
	if tok == "" {
		t.Skip("APPMAX_ACCESS_TOKEN ausente — pulando integração ao vivo")
	}
	base := os.Getenv("APPMAX_BASE_URL")
	if base == "" {
		t.Skip("APPMAX_BASE_URL ausente — setar o host de sandbox")
	}
	return NewWithBaseURL(tok, base)
}

// CPF de teste válido (dígitos verificadores corretos, não-repetido).
const testCPF = "39053344705"

func TestIntegration_CustomerOrderPix(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// 1) customer
	custID, custRaw, err := c.CreateCustomer(ctx, CustomerInput{
		FirstName: "Cliente", LastName: "Teste UtiLar",
		Email:     "integ-utilar@example.com",
		Telephone: "11999990000",
		Postcode:  "01310100", AddressStreet: "Av Paulista", AddressStreetNumber: "1000",
		AddressStreetDistrict: "Bela Vista", AddressCity: "São Paulo", AddressState: "SP",
		IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if custID == 0 {
		t.Fatalf("customer_id não veio; raw=%s", custRaw)
	}
	t.Logf("✓ customer_id=%d", custID)

	// 2) order (reais)
	orderID, orderRaw, err := c.CreateOrder(ctx, OrderInput{
		Total: 49.90, CustomerID: custID,
		Products: []OrderProduct{{SKU: "UTILAR-INTEG", Name: "Produto teste UtiLar", Qty: 1, Price: 49.90}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if orderID == 0 {
		t.Fatalf("order_id não veio; raw=%s", orderRaw)
	}
	t.Logf("✓ order_id=%d", orderID)

	// 3) pix
	pix, err := c.PayPix(ctx, orderID, custID, testCPF)
	if err != nil {
		t.Fatalf("PayPix: %v", err)
	}
	if pix.PixEmv == "" && pix.PixQrCode == "" {
		t.Errorf("Pix sem EMV/QR na resposta; raw=%s", pix.Raw)
	}
	t.Logf("✓ Pix emv=%.30s... expira=%s", pix.PixEmv, pix.PixExpiresAt)

	// 4) GetOrder (reconciliação — base do webhook)
	got, err := c.GetOrder(ctx, itoa(orderID))
	if err != nil {
		t.Logf("⚠ GetOrder falhou (endpoint a confirmar no sandbox): %v", err)
	} else {
		t.Logf("✓ GetOrder status=%q total=%.2f", got.Status, got.Total)
	}
}

func TestIntegration_GatewayPixEndToEnd(t *testing.T) {
	tok := os.Getenv("APPMAX_ACCESS_TOKEN")
	base := os.Getenv("APPMAX_BASE_URL")
	if tok == "" || base == "" {
		t.Skip("APPMAX_ACCESS_TOKEN/APPMAX_BASE_URL ausentes")
	}
	g := &Gateway{client: NewWithBaseURL(tok, base)}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	res, err := g.CreatePayment(ctx, psp.CreateRequest{
		OrderID:    "integ-" + itoa(time.Now().Unix()),
		Amount:     29.90,
		Currency:   "BRL",
		Method:     psp.MethodPix,
		PayerEmail: "integ-gw@example.com",
		PayerName:  "Cliente Teste",
		PayerCPF:   testCPF,
		PayerPhone: "11999990000",
	})
	if err != nil {
		t.Fatalf("Gateway.CreatePayment: %v", err)
	}
	if res.PSPID == "" {
		t.Error("PSPID (order id) vazio")
	}
	if !strings.Contains(string(res.ClientData), "pix_emv") {
		t.Errorf("ClientData sem pix_emv: %s", res.ClientData)
	}
	t.Logf("✓ gateway pix: pspID=%s status=%s", res.PSPID, res.Status)
}

func itoa(n int64) string {
	// pequeno helper local pra não importar strconv só por isso
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
