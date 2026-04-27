// Testes de hardening de config: JWT_SECRET fail-closed (audit O2-H3).
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

func TestLoad_RejectsKnownDefaults(t *testing.T) {
	for _, secret := range []string{"change-me", "change-me-in-prod-please"} {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("JWT_SECRET", secret)
			t.Setenv("DEV_MODE", "false")
			if _, err := Load(); !errors.Is(err, ErrInsecureJWTSecret) {
				t.Errorf("expected ErrInsecureJWTSecret for %q, got %v", secret, err)
			}
		})
	}
}

func TestLoad_AcceptsStrongSecretInProd(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("DEV_MODE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DevMode {
		t.Error("expected DevMode=false")
	}
}

func TestLoad_DevModeEnablesXUserIdFallback(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("DEV_MODE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevMode {
		t.Error("expected DevMode=true to flag X-User-Id fallback")
	}
}
