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

	// JWTSecret valida os tokens do auth-service nas rotas /admin (escrita de
	// catálogo). Compartilhado entre os serviços. Vazio + DevMode permite o
	// fallback X-User-Id/X-User-Role pra dev sem auth-service.
	JWTSecret string
	DevMode   bool
}

func Load() *Config {
	return &Config{
		Port:           env("PORT", "8091"),
		DatabaseURL:    env("CATALOG_DB_URL", "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"),
		AllowedOrigins: parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		RedisURL:       os.Getenv("REDIS_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		DevMode:        os.Getenv("DEV_MODE") == "true",
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
