// Testes de hardening: JWT_SECRET (transversal) + webhook secret fail-closed
// (audit C5). Cada serviço backend valida o mesmo padrão de fail-closed.
package config

import (
	"strings"
	"testing"
)

func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEV_MODE", "false")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("PSP_PROVIDER", "stripe")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_aaa")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_aaa")
}

func TestLoad_RejectsEmptyJWTSecretInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for empty JWT_SECRET in prod, got nil")
	}
}

func TestLoad_RejectsKnownDefaultJWTSecret(t *testing.T) {
	for _, secret := range []string{"change-me", "change-me-in-prod-please"} {
		t.Run(secret, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("JWT_SECRET", secret)
			if _, err := Load(); err == nil {
				t.Errorf("expected error for default secret %q, got nil", secret)
			}
		})
	}
}

func TestLoad_RejectsShortJWTSecretInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "tooshort") // < 32 chars
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short JWT_SECRET, got nil")
	}
}

func TestLoad_AcceptsStrongJWTSecretInProd(t *testing.T) {
	setBaseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DevMode {
		t.Error("expected DevMode=false")
	}
}

func TestLoad_StripeWebhookSecretRequiredInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing STRIPE_WEBHOOK_SECRET in prod, got nil")
	}
}

func TestLoad_MPWebhookSecretRequiredInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "mercadopago")
	t.Setenv("MP_ACCESS_TOKEN", "MP_TEST_TOKEN")
	t.Setenv("MP_WEBHOOK_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing MP_WEBHOOK_SECRET in prod, got nil")
	}
}

func TestLoad_DevModeSkipsWebhookSecretCheck(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("DEV_MODE", "true")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "") // vazio mas dev mode

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error in dev mode: %v", err)
	}
	if !cfg.DevMode {
		t.Error("expected DevMode=true")
	}
	if cfg.StripeWebhookSecret != "" {
		t.Error("expected StripeWebhookSecret to be empty (dev mode)")
	}
}

func TestLoad_RejectsInvalidPSPProvider(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "asaas")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid PSP_PROVIDER, got nil")
	}
}
