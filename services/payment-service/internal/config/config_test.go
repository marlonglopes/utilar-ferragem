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
	t.Setenv("APP_ENV", "development")
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

func TestLoad_AppmaxRequiresAccessTokenInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "appmax")
	t.Setenv("APPMAX_ACCESS_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing APPMAX_ACCESS_TOKEN in prod, got nil")
	}
}

// A Appmax não assina postbacks (sem HMAC), então — diferente de Stripe/MP — não
// exigimos webhook secret em prod. A integridade vem da re-consulta GetPayment (C3).
func TestLoad_AppmaxAcceptsWithoutWebhookSecretInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "appmax")
	t.Setenv("APPMAX_ACCESS_TOKEN", "appmax_test_token")
	t.Setenv("APPMAX_WEBHOOK_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PSPProvider != "appmax" {
		t.Errorf("expected provider=appmax, got %q", cfg.PSPProvider)
	}
	if cfg.AppmaxAccessToken != "appmax_test_token" {
		t.Errorf("expected access token to be loaded")
	}
}

// SWITCH Stripe→Appmax v1: o provider recomendado (OAuth2, Payment Split,
// parcelamento) precisa validar como fail-closed — sem client id/secret e sem as
// URLs em prod, o boot recusa. Trava o switch pra não subir apontando pro lugar
// errado (ou cobrar de verdade num deploy que ia pro sandbox).
func TestLoad_AppmaxV1RequiresClientCredsInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "appmax-v1")
	// sem CLIENT_ID/SECRET → erro
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing APPMAX_V1_CLIENT_ID/SECRET in prod, got nil")
	}
}

func TestLoad_AppmaxV1RequiresURLsInProd(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "appmax-v1")
	t.Setenv("APPMAX_V1_CLIENT_ID", "cid")
	t.Setenv("APPMAX_V1_CLIENT_SECRET", "csecret")
	// creds ok, mas sem AUTH_URL/API_URL em prod → erro (evita apontar pro ambiente errado)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing APPMAX_V1_AUTH_URL/API_URL in prod, got nil")
	}
}

func TestLoad_AppmaxV1AcceptsWithFullCreds(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("PSP_PROVIDER", "appmax-v1")
	t.Setenv("APPMAX_V1_CLIENT_ID", "cid")
	t.Setenv("APPMAX_V1_CLIENT_SECRET", "csecret")
	t.Setenv("APPMAX_V1_AUTH_URL", "https://sandbox.appmax.com.br/oauth")
	t.Setenv("APPMAX_V1_API_URL", "https://sandbox.appmax.com.br")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PSPProvider != "appmax-v1" {
		t.Errorf("expected provider=appmax-v1, got %q", cfg.PSPProvider)
	}
	if cfg.AppmaxV1ClientID != "cid" || cfg.AppmaxV1ClientSecret != "csecret" {
		t.Errorf("expected appmax-v1 creds to be loaded")
	}
}
