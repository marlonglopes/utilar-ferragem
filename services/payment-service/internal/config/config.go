package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port            string
	DatabaseURL     string
	RedpandaBrokers []string
	JWTSecret       string
	DevMode         bool
	AllowedOrigins  []string // CORS whitelist; vazio = wildcard "*"

	// OrderServiceURL é usado pra validar amount/ownership de pedidos antes de
	// criar pagamento (audit C1, C2). O JWT do cliente é propagado.
	OrderServiceURL string

	// PSP selector — qual provider usar.
	// Valores: "stripe" (recomendado em dev + test mode robusto)
	//          "mercadopago" (prod BR quando merchant onboarded)
	PSPProvider string

	// Stripe (usado quando PSPProvider=stripe)
	StripeSecretKey     string
	StripePublishableKey string
	StripeWebhookSecret string

	// Mercado Pago (usado quando PSPProvider=mercadopago)
	MPAccessToken   string
	MPPublicKey     string
	MPWebhookSecret string
}

const devSecret = "dev-only-secret-not-for-production"

func Load() (*Config, error) {
	dbURL := env("PAYMENT_DB_URL", "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable")
	brokers := strings.Split(env("REDPANDA_BROKERS", "localhost:19092"), ",")
	provider := strings.ToLower(env("PSP_PROVIDER", "stripe"))
	devMode := env("DEV_MODE", "false") == "true"

	// JWT_SECRET fail-closed (audit transversal — mesmo padrão de auth/order).
	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if !devMode {
			return nil, fmt.Errorf("JWT_SECRET is required (set DEV_MODE=true for local dev)")
		}
		jwt = devSecret
	}
	if !devMode && (jwt == "change-me" || jwt == "change-me-in-prod-please" || len(jwt) < 32) {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 chars and not a development default")
	}

	cfg := &Config{
		Port:            env("PORT", "8090"),
		DatabaseURL:     dbURL,
		RedpandaBrokers: brokers,
		JWTSecret:       jwt,
		DevMode:         devMode,
		AllowedOrigins:  parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		OrderServiceURL: env("ORDER_SERVICE_URL", "http://localhost:8092"),
		PSPProvider:     provider,

		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),

		MPAccessToken:   os.Getenv("MP_ACCESS_TOKEN"),
		MPPublicKey:     os.Getenv("MP_PUBLIC_KEY"),
		MPWebhookSecret: os.Getenv("MP_WEBHOOK_SECRET"),
	}

	// Valida credenciais do provider escolhido + webhook secret fail-closed em prod (audit C5).
	switch cfg.PSPProvider {
	case "stripe":
		if cfg.StripeSecretKey == "" {
			return nil, fmt.Errorf("STRIPE_SECRET_KEY is required when PSP_PROVIDER=stripe")
		}
		if !devMode && cfg.StripeWebhookSecret == "" {
			return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRET is required in non-dev mode (audit C5: fail-closed)")
		}
	case "mercadopago":
		if cfg.MPAccessToken == "" {
			return nil, fmt.Errorf("MP_ACCESS_TOKEN is required when PSP_PROVIDER=mercadopago")
		}
		if !devMode && cfg.MPWebhookSecret == "" {
			return nil, fmt.Errorf("MP_WEBHOOK_SECRET is required in non-dev mode (audit C5: fail-closed)")
		}
	default:
		return nil, fmt.Errorf("invalid PSP_PROVIDER=%q (expected: stripe | mercadopago)", cfg.PSPProvider)
	}

	return cfg, nil
}

func parseOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
