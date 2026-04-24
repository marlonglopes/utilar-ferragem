package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() *Config {
	return &Config{
		Port:        env("PORT", "8091"),
		DatabaseURL: env("CATALOG_DB_URL", "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
