// Testes de hardening de configuração: garantem que JWT_SECRET fail-closed
// (audit A2-C2) impede o serviço de subir com config insegura.
package config

import (
	"errors"
	"strings"
	"testing"
)

func TestLoad_RejectsEmptyJWTSecretInProd(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DEV_MODE", "false")

	_, err := Load()
	if !errors.Is(err, ErrInsecureJWTSecret) {
		t.Fatalf("expected ErrInsecureJWTSecret, got %v", err)
	}
}

func TestLoad_RejectsKnownDefaultJWTSecret(t *testing.T) {
	cases := []string{"change-me", "change-me-in-prod-please"}
	for _, secret := range cases {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("JWT_SECRET", secret)
			t.Setenv("DEV_MODE", "false")

			_, err := Load()
			if !errors.Is(err, ErrInsecureJWTSecret) {
				t.Errorf("expected ErrInsecureJWTSecret for %q, got %v", secret, err)
			}
		})
	}
}

func TestLoad_RejectsShortJWTSecretInProd(t *testing.T) {
	t.Setenv("JWT_SECRET", "tooshort") // < 32 chars
	t.Setenv("DEV_MODE", "false")

	_, err := Load()
	if !errors.Is(err, ErrInsecureJWTSecret) {
		t.Fatalf("expected ErrInsecureJWTSecret, got %v", err)
	}
}

func TestLoad_AcceptsStrongJWTSecretInProd(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("DEV_MODE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DevMode {
		t.Error("expected DevMode=false")
	}
	if len(cfg.JWTSecret) != 64 {
		t.Errorf("JWTSecret len=%d, expected 64", len(cfg.JWTSecret))
	}
}

func TestLoad_AcceptsAnyJWTSecretInDevMode(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DEV_MODE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevMode {
		t.Error("expected DevMode=true")
	}
	if cfg.JWTSecret == "" {
		t.Error("expected dev fallback secret to be applied, got empty")
	}
}
