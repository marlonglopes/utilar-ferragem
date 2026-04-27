package config

import (
	"os"
	"strings"
)

type Config struct {
	Port           string
	DatabaseURL    string
	AllowedOrigins []string // CORS whitelist; vazio = wildcard "*"
	RedisURL       string   // CT1-H1: vazio = rate limit desabilitado
}

func Load() *Config {
	return &Config{
		Port:           env("PORT", "8091"),
		DatabaseURL:    env("CATALOG_DB_URL", "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"),
		AllowedOrigins: parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		RedisURL:       os.Getenv("REDIS_URL"),
	}
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
