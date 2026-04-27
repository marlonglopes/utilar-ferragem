package config

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	Port             string
	DatabaseURL      string
	JWTSecret        string
	DevMode          bool // habilita comportamentos de dev (logs verbose, fallback de auth)
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	EmailVerifyTTL   time.Duration
	PasswordResetTTL time.Duration
}

// devSecret só é aceito em DEV_MODE=true. Em prod, JWT_SECRET é obrigatório
// e qualquer valor que comece com "change-me" é rejeitado (audit A2-C2).
const devSecret = "dev-only-secret-not-for-production"

// ErrInsecureJWTSecret é retornado se JWT_SECRET não está configurado ou usa
// um valor default conhecido em modo não-dev.
var ErrInsecureJWTSecret = errors.New("config: JWT_SECRET is required and must not be a development default")

// Load lê configuração de env vars. Retorna erro fatal se config crítica
// estiver insegura (ex: JWT_SECRET ausente em prod) — caller deve abortar startup.
func Load() (*Config, error) {
	devMode := env("DEV_MODE", "false") == "true"

	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if !devMode {
			return nil, ErrInsecureJWTSecret
		}
		jwt = devSecret
	}
	// Recusa qualquer fallback antigo conhecido mesmo se vier por engano via env
	if !devMode && (jwt == "change-me" || jwt == "change-me-in-prod-please" || len(jwt) < 32) {
		return nil, ErrInsecureJWTSecret
	}

	return &Config{
		Port:             env("PORT", "8093"),
		DatabaseURL:      env("AUTH_DB_URL", "postgres://utilar:utilar@localhost:5438/auth_service?sslmode=disable"),
		JWTSecret:        jwt,
		DevMode:          devMode,
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  30 * 24 * time.Hour, // 30 dias
		EmailVerifyTTL:   24 * time.Hour,
		PasswordResetTTL: 1 * time.Hour,
	}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
