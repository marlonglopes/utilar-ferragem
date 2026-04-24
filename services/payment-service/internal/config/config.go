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

func Load() (*Config, error) {
	dbURL := env("PAYMENT_DB_URL", "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable")
	brokers := strings.Split(env("REDPANDA_BROKERS", "localhost:19092"), ",")
	provider := strings.ToLower(env("PSP_PROVIDER", "stripe"))

	cfg := &Config{
		Port:            env("PORT", "8090"),
		DatabaseURL:     dbURL,
		RedpandaBrokers: brokers,
		JWTSecret:       env("JWT_SECRET", "change-me"),
		PSPProvider:     provider,

		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),

		MPAccessToken:   os.Getenv("MP_ACCESS_TOKEN"),
		MPPublicKey:     os.Getenv("MP_PUBLIC_KEY"),
		MPWebhookSecret: os.Getenv("MP_WEBHOOK_SECRET"),
	}

	// Valida credenciais do provider escolhido
	switch cfg.PSPProvider {
	case "stripe":
		if cfg.StripeSecretKey == "" {
			return nil, fmt.Errorf("STRIPE_SECRET_KEY is required when PSP_PROVIDER=stripe")
		}
	case "mercadopago":
		if cfg.MPAccessToken == "" {
			return nil, fmt.Errorf("MP_ACCESS_TOKEN is required when PSP_PROVIDER=mercadopago")
		}
	default:
		return nil, fmt.Errorf("invalid PSP_PROVIDER=%q (expected: stripe | mercadopago)", cfg.PSPProvider)
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
