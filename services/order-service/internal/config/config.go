package config

import (
	"errors"
	"os"
	"strings"

	"github.com/utilar/pkg/devguard"
	"github.com/utilar/pkg/servicetoken"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	DevMode     bool // habilita X-User-Id fallback (audit O1-C3)
	// ServiceJWTSecret assina os tokens `role=service` que o order-service usa
	// para chamar as rotas internas do catalog-service (reserva de estoque) e do
	// auth-service (contexto de operador de balcão).
	//
	// A1 (auditoria 2026-07-18): separado do JWTSecret de propósito. O poder de
	// EMITIR identidade de serviço fica restrito a quem realmente precisa dele —
	// o assistant-service, que é o mais exposto, não recebe esta variável.
	ServiceJWTSecret  string
	AllowedOrigins    []string
	CatalogServiceURL string // base URL do catalog-service pra validação de price (O2-H5)
	// AuthServiceURL — de onde vem o contexto autoritativo do operador de
	// balcão (loja, cargo e teto de desconto). Ver internal/authclient.
	AuthServiceURL string
	// PaymentServiceURL — para onde vai o lançamento contábil da liquidação
	// externa (venda de balcão paga na maquininha da loja). O livro contábil é
	// único e vive no payment-service; ver internal/paymentclient.
	PaymentServiceURL string
	RedisURL          string // O3-M3: vazio = rate limit desabilitado
	// KafkaBrokers — brokers do Redpanda onde o payment-service publica os
	// eventos do outbox. Vazio desliga o consumer (e o pedido nunca vira 'paid'
	// automaticamente), então logamos alto no boot.
	KafkaBrokers []string
}

// #nosec G101 — placeholder dev-only, rejeitado em prod via fail-closed em Load().
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
	if !devMode && (strings.HasPrefix(jwt, "change-me") || jwt == devSecret || len(jwt) < 32) {
		return nil, ErrInsecureJWTSecret
	}

	// A2 (auditoria 2026-07-18): DEV_MODE liga o fallback de header
	// X-User-Role, que não tem verificação criptográfica nenhuma. Se essa
	// variável for ligada por engano em produção, qualquer requisição com
	// `X-User-Role: admin` vira acesso de administrador — sem alarme e sem
	// sintoma. Aqui o serviço se RECUSA A SUBIR: indisponibilidade barulhenta
	// é preferível a comprometimento silencioso.
	if err := devguard.Check(devMode, os.Getenv("ORDER_DB_URL")); err != nil {
		return nil, err
	}

	// A1 (auditoria 2026-07-18): fail-closed. O order-service EMITE token de
	// serviço; sem SERVICE_JWT_SECRET fora de DEV_MODE ele não sobe, em vez de
	// voltar a assinar com o segredo de usuário.
	serviceSecret, err := servicetoken.SecretFromEnv(devMode, jwt)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:              env("PORT", "8092"),
		DatabaseURL:       env("ORDER_DB_URL", "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"),
		JWTSecret:         jwt,
		ServiceJWTSecret:  serviceSecret,
		DevMode:           devMode,
		AllowedOrigins:    parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		CatalogServiceURL: env("CATALOG_SERVICE_URL", "http://localhost:8091"),
		AuthServiceURL:    env("AUTH_SERVICE_URL", "http://localhost:8093"),
		PaymentServiceURL: env("PAYMENT_SERVICE_URL", "http://localhost:8090"),
		RedisURL:          os.Getenv("REDIS_URL"),
		KafkaBrokers:      parseOrigins(os.Getenv("KAFKA_BROKERS")),
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
