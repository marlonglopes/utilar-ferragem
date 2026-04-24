package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port             string
	DatabaseURL      string
	RedpandaBrokers  []string
	MPAccessToken    string
	MPPublicKey      string
	MPWebhookSecret  string
	JWTSecret        string
}

func Load() (*Config, error) {
	dbURL := env("PAYMENT_DB_URL", "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable")
	brokers := strings.Split(env("REDPANDA_BROKERS", "localhost:19092"), ",")

	mpToken := os.Getenv("MP_ACCESS_TOKEN")
	if mpToken == "" {
		return nil, fmt.Errorf("MP_ACCESS_TOKEN is required")
	}

	return &Config{
		Port:            env("PORT", "8090"),
		DatabaseURL:     dbURL,
		RedpandaBrokers: brokers,
		MPAccessToken:   mpToken,
		MPPublicKey:     os.Getenv("MP_PUBLIC_KEY"),
		MPWebhookSecret: os.Getenv("MP_WEBHOOK_SECRET"),
		JWTSecret:       env("JWT_SECRET", "change-me"),
	}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
