package config

import (
	"errors"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	DevMode     bool // habilita X-User-Id fallback (audit O1-C3)
}

const devSecret = "dev-only-secret-not-for-production"

// ErrInsecureJWTSecret é retornado se JWT_SECRET não está configurado ou usa
// um valor default conhecido em modo não-dev (audit O2-H3).
var ErrInsecureJWTSecret = errors.New("config: JWT_SECRET is required and must not be a development default")

func Load() (*Config, error) {
	devMode := env("DEV_MODE", "false") == "true"

	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if !devMode {
			return nil, ErrInsecureJWTSecret
		}
		jwt = devSecret
	}
	if !devMode && (jwt == "change-me" || jwt == "change-me-in-prod-please" || len(jwt) < 32) {
		return nil, ErrInsecureJWTSecret
	}

	return &Config{
		Port:        env("PORT", "8092"),
		DatabaseURL: env("ORDER_DB_URL", "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"),
		JWTSecret:   jwt,
		DevMode:     devMode,
	}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
