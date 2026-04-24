package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
}

func Load() *Config {
	return &Config{
		Port:        env("PORT", "8092"),
		DatabaseURL: env("ORDER_DB_URL", "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"),
		// Mesmo secret do auth-service para validar JWTs emitidos por ele.
		JWTSecret: env("JWT_SECRET", "change-me-in-prod-please"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
