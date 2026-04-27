// Testa VerifyWebhook do Mercado Pago no formato oficial Webhook V2 (audit C4).
//
// Manifest: id:<data.id>;request-id:<x-request-id>;ts:<ts>;
// Header: X-Signature: ts=<ts>,v1=<hmac-sha256(manifest, secret)>
//
// Replay protection: delta(ts, now) > 5min → reject.
package mercadopago

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

const testWebhookSecret = "supersecret-test-key"

// signMPWebhook gera um header X-Signature válido pra um body+request-id+ts.
// Helper que replica a lógica que o MP usa no servidor.
func signMPWebhook(t *testing.T, dataID, requestID string, ts int64, secret string) string {
	t.Helper()
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%d;", dataID, requestID, ts)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(manifest))
	v1 := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("ts=%d,v1=%s", ts, v1)
}

func TestVerifyWebhook_ValidV2(t *testing.T) {
	g := New("token", testWebhookSecret)
	ts := time.Now().UnixMilli()
	body := []byte(`{"data":{"id":"pay-123"},"type":"payment","action":"payment.updated"}`)

	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, "pay-123", "req-abc", ts, testWebhookSecret))
	h.Set("X-Request-Id", "req-abc")

	if err := g.VerifyWebhook(body, h); err != nil {
		t.Errorf("expected valid signature, got %v", err)
	}
}

func TestVerifyWebhook_LegacyResourceFormat(t *testing.T) {
	g := New("token", testWebhookSecret)
	ts := time.Now().UnixMilli()
	body := []byte(`{"resource":"https://api.mercadopago.com/v1/payments/789","topic":"payment"}`)

	// Para formato legacy, data.id = "789" (último segmento da URL)
	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, "789", "req-xyz", ts, testWebhookSecret))
	h.Set("X-Request-Id", "req-xyz")

	if err := g.VerifyWebhook(body, h); err != nil {
		t.Errorf("expected valid signature for legacy format, got %v", err)
	}
}

func TestVerifyWebhook_RejectsMissingSignature(t *testing.T) {
	g := New("token", testWebhookSecret)
	body := []byte(`{"data":{"id":"pay-123"}}`)

	h := http.Header{}
	h.Set("X-Request-Id", "req-abc")
	// Sem X-Signature

	err := g.VerifyWebhook(body, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_RejectsMissingRequestID(t *testing.T) {
	g := New("token", testWebhookSecret)
	ts := time.Now().UnixMilli()
	body := []byte(`{"data":{"id":"pay-123"}}`)

	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, "pay-123", "req-abc", ts, testWebhookSecret))
	// Sem X-Request-Id

	err := g.VerifyWebhook(body, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_RejectsBadHMAC(t *testing.T) {
	g := New("token", testWebhookSecret)
	ts := time.Now().UnixMilli()
	body := []byte(`{"data":{"id":"pay-123"}}`)

	h := http.Header{}
	// Assinou com secret diferente
	h.Set("X-Signature", signMPWebhook(t, "pay-123", "req-abc", ts, "wrong-secret"))
	h.Set("X-Request-Id", "req-abc")

	err := g.VerifyWebhook(body, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyWebhook_RejectsTamperedBody(t *testing.T) {
	g := New("token", testWebhookSecret)
	ts := time.Now().UnixMilli()

	// Assinatura válida pra data.id = "original-123"
	originalID := "original-123"
	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, originalID, "req-abc", ts, testWebhookSecret))
	h.Set("X-Request-Id", "req-abc")

	// Mas body chega com data.id diferente (atacante alterou)
	tamperedBody := []byte(`{"data":{"id":"tampered-456"}}`)

	err := g.VerifyWebhook(tamperedBody, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for tampered data.id, got %v", err)
	}
}

func TestVerifyWebhook_RejectsReplayOldTimestamp(t *testing.T) {
	g := New("token", testWebhookSecret)
	// ts de 10min atrás — fora da janela de 5min
	ts := time.Now().Add(-10 * time.Minute).UnixMilli()
	body := []byte(`{"data":{"id":"pay-123"}}`)

	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, "pay-123", "req-abc", ts, testWebhookSecret))
	h.Set("X-Request-Id", "req-abc")

	err := g.VerifyWebhook(body, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for replay, got %v", err)
	}
}

func TestVerifyWebhook_RejectsReplayFutureTimestamp(t *testing.T) {
	g := New("token", testWebhookSecret)
	// ts de 10min no futuro — atacante tentando forjar
	ts := time.Now().Add(10 * time.Minute).UnixMilli()
	body := []byte(`{"data":{"id":"pay-123"}}`)

	h := http.Header{}
	h.Set("X-Signature", signMPWebhook(t, "pay-123", "req-abc", ts, testWebhookSecret))
	h.Set("X-Request-Id", "req-abc")

	err := g.VerifyWebhook(body, h)
	if !errors.Is(err, psp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for future ts, got %v", err)
	}
}

func TestVerifyWebhook_NoSecretAllowsAll(t *testing.T) {
	g := New("token", "") // dev mode (gated por config.Load fail-closed em prod)
	if err := g.VerifyWebhook([]byte(`{}`), http.Header{}); err != nil {
		t.Errorf("expected nil with empty secret, got %v", err)
	}
}

func TestVerifyWebhook_RejectsMalformedSignatureHeader(t *testing.T) {
	g := New("token", testWebhookSecret)
	body := []byte(`{"data":{"id":"pay-123"}}`)

	cases := []string{
		"missing-equals",
		"ts=onlyts",
		"v1=onlyv1",
		"=,=",
		"",
	}
	for _, sig := range cases {
		t.Run(sig, func(t *testing.T) {
			h := http.Header{}
			h.Set("X-Signature", sig)
			h.Set("X-Request-Id", "req-abc")
			err := g.VerifyWebhook(body, h)
			if !errors.Is(err, psp.ErrInvalidSignature) {
				t.Errorf("expected ErrInvalidSignature for sig=%q, got %v", sig, err)
			}
		})
	}
}

func TestParseMPSignatureHeader(t *testing.T) {
	cases := []struct {
		in      string
		wantTS  string
		wantV1  string
		wantErr bool
	}{
		{"ts=123,v1=abc", "123", "abc", false},
		{"v1=abc,ts=123", "123", "abc", false},                  // ordem invertida
		{"ts=123, v1=abc", "123", "abc", false},                 // espaço extra
		{"ts=123,v1=abc,extra=ignored", "123", "abc", false},    // campo extra
		{"ts=123", "", "", true},                                // só ts
		{"", "", "", true},                                      // vazio
		{"v1=abc", "", "", true},                                // só v1
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			ts, v1, err := parseMPSignatureHeader(c.in)
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", c.in)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if ts != c.wantTS || v1 != c.wantV1 {
				t.Errorf("parseMPSignatureHeader(%q) = (%q, %q), want (%q, %q)", c.in, ts, v1, c.wantTS, c.wantV1)
			}
		})
	}
}

func TestExtractDataID_V2Format(t *testing.T) {
	body := []byte(`{"data":{"id":"abc123"},"type":"payment"}`)
	id, err := extractDataID(body)
	if err != nil {
		t.Fatal(err)
	}
	if id != "abc123" {
		t.Errorf("got %q, want abc123", id)
	}
}

func TestExtractDataID_LegacyResourceFormat(t *testing.T) {
	body := []byte(`{"resource":"https://api.mercadopago.com/v1/payments/789","topic":"payment"}`)
	id, err := extractDataID(body)
	if err != nil {
		t.Fatal(err)
	}
	if id != "789" {
		t.Errorf("got %q, want 789", id)
	}
}

func TestExtractDataID_Missing(t *testing.T) {
	if _, err := extractDataID([]byte(`{"foo":"bar"}`)); err == nil {
		t.Error("expected error for body without data.id")
	}
}

// silence unused import on platforms where strconv is only used through helpers
var _ = strconv.ParseInt
