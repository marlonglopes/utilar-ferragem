package config

import (
	"errors"
	"strings"
	"testing"
)

// Regressão: o catalog-service subia com JWT_SECRET vazio logando "admin routes
// reject all requests" — afirmação falsa. HS256 verifica normalmente com chave
// vazia, então qualquer um podia assinar {"role":"admin"} e reescrever preços
// via POST /api/v1/admin/products. Load() agora é fail-closed como o order-service.
func TestLoadRejectsInsecureJWTSecret(t *testing.T) {
	valid := strings.Repeat("a", 32)

	cases := []struct {
		name    string
		devMode string
		secret  string
		wantErr bool
	}{
		{"prod sem secret", "false", "", true},
		{"prod com secret curto", "false", "short", true},
		{"prod com change-me", "false", "change-me", true},
		{"prod com change-me-in-prod-please", "false", "change-me-in-prod-please", true},
		{"prod com secret valido", "false", valid, false},
		{"dev sem secret usa fallback", "true", "", false},
		{"dev com secret curto e permitido", "true", "short", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEV_MODE", tc.devMode)
			t.Setenv("JWT_SECRET", tc.secret)
			// A1: fora de DEV_MODE o boot exige o segredo de serviço, distinto
			// do de usuário. Ver service_secret_test.go.
			t.Setenv("SERVICE_JWT_SECRET", strings.Repeat("s", 64))

			cfg, err := Load()

			if tc.wantErr {
				if !errors.Is(err, ErrInsecureJWTSecret) {
					t.Fatalf("esperava ErrInsecureJWTSecret, veio err=%v cfg=%+v", err, cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("não esperava erro, veio %v", err)
			}
			if cfg.JWTSecret == "" {
				t.Fatal("JWTSecret vazio — HS256 verificaria com chave vazia e o admin viraria forjável")
			}
		})
	}
}

func TestLoadDevFallbackSecretIsNotUsableInProd(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("JWT_SECRET", "")

	dev, err := Load()
	if err != nil {
		t.Fatalf("dev mode deveria carregar: %v", err)
	}

	// O mesmo secret que o dev mode injeta precisa ser recusado em produção.
	t.Setenv("DEV_MODE", "false")
	t.Setenv("JWT_SECRET", dev.JWTSecret)

	if _, err := Load(); !errors.Is(err, ErrInsecureJWTSecret) {
		t.Fatalf("secret de dev foi aceito em prod (len=%d)", len(dev.JWTSecret))
	}
}
