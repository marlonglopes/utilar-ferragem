package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() *Config {
	return &Config{
		Port:        env("PORT", "8092"),
		DatabaseURL: env("ORDER_DB_URL", "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
