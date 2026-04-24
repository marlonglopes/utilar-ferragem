package config

import (
	"os"
	"time"
)

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	EmailVerifyTTL     time.Duration
	PasswordResetTTL   time.Duration
}

func Load() *Config {
	return &Config{
		Port:             env("PORT", "8093"),
		DatabaseURL:      env("AUTH_DB_URL", "postgres://utilar:utilar@localhost:5438/auth_service?sslmode=disable"),
		JWTSecret:        env("JWT_SECRET", "change-me-in-prod-please"),
		AccessTokenTTL:   15 * time.Minute,
		RefreshTokenTTL:  30 * 24 * time.Hour, // 30 dias
		EmailVerifyTTL:   24 * time.Hour,
		PasswordResetTTL: 1 * time.Hour,
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
