package config

import (
	"errors"
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

// #nosec G101 — placeholder dev-only, rejeitado em prod via fail-closed em Load().
const devSecret = "dev-only-secret-not-for-production"

// ErrInsecureJWTSecret é retornado se JWT_SECRET não está configurado ou usa um
// valor default conhecido em modo não-dev.
//
// Fail-closed é obrigatório aqui: HS256 verifica normalmente com chave vazia,
// então subir sem JWT_SECRET não "rejeita todo mundo" — pelo contrário, deixa
// qualquer um assinar {"role":"admin"} e reescrever preço via POST /admin/products.
var ErrInsecureJWTSecret = errors.New("config: JWT_SECRET is required and must not be a development default")

func Load() (*Config, error) {
	devMode := os.Getenv("DEV_MODE") == "true"

	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if !devMode {
			return nil, ErrInsecureJWTSecret
		}
		jwt = devSecret
	}
	if !devMode && (strings.HasPrefix(jwt, "change-me") || jwt == devSecret || len(jwt) < 32) {
		return nil, ErrInsecureJWTSecret
	}

	return &Config{
		Port:           env("PORT", "8091"),
		DatabaseURL:    env("CATALOG_DB_URL", "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"),
		AllowedOrigins: parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		RedisURL:       os.Getenv("REDIS_URL"),
		JWTSecret:      jwt,
		DevMode:        devMode,
	}, nil
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
