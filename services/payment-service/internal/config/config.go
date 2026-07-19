package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/utilar/pkg/devguard"
)

type Config struct {
	Port            string
	DatabaseURL     string
	RedpandaBrokers []string
	JWTSecret       string
	DevMode         bool
	AllowedOrigins  []string // CORS whitelist; vazio = wildcard "*"

	// OrderServiceURL é usado pra validar amount/ownership de pedidos antes de
	// criar pagamento (audit C1, C2). O JWT do cliente é propagado.
	OrderServiceURL string

	// AuthServiceURL é usado pra buscar CPF/name pro boleto (audit M6).
	AuthServiceURL string

	// PSP selector — qual provider usar.
	// Valores: "stripe" (recomendado em dev + test mode robusto)
	//          "mercadopago" (prod BR quando merchant onboarded)
	PSPProvider string

	// Stripe (usado quando PSPProvider=stripe)
	StripeSecretKey      string
	StripePublishableKey string
	StripeWebhookSecret  string

	// Mercado Pago (usado quando PSPProvider=mercadopago)
	MPAccessToken   string
	MPPublicKey     string
	MPWebhookSecret string

	// Appmax (usado quando PSPProvider=appmax) — sub-adquirente BR.
	// AppmaxAccessToken é a credencial da API (vai no corpo de cada request).
	// AppmaxWebhookSecret é OPCIONAL: a Appmax não assina postbacks, a integridade
	// vem da re-consulta via GetPayment (audit C3). Se setado, exigimos header
	// X-Appmax-Token no webhook.
	AppmaxAccessToken   string
	AppmaxWebhookSecret string

	// Appmax AppStore API v1 (usado quando PSPProvider=appmax-v1) — API OAuth2
	// distinta da v3 admin acima. Valores em centavos, Bearer token com TTL de 1h.
	// Sandbox: auth=https://auth.sandboxappmax.com.br api=https://api.sandboxappmax.com.br
	// O webhook continua sem assinatura → APPMAX_WEBHOOK_SECRET é opcional (compartilhado
	// com o provider v3, já que o header X-Appmax-Token é o mesmo mecanismo).
	AppmaxV1AuthURL      string
	AppmaxV1APIURL       string
	AppmaxV1ClientID     string
	AppmaxV1ClientSecret string
	AppmaxV1ExternalID   string

	// Redis (rate limit + idempotency). Vazio em dev = features desabilitadas.
	RedisURL string

	// MetricsToken protege GET /metrics (bearer token).
	//
	// FAIL-CLOSED: vazio = endpoint DESABILITADO (404), nunca aberto. /metrics
	// entrega volume financeiro, taxa de recusa por método e topologia interna;
	// "só a rede interna acessa" é uma suposição que não sobrevive a um pod
	// comprometido no mesmo cluster. Ver docs/observability-alerts.md.
	MetricsToken string
}

// #nosec G101 — placeholder dev-only, rejeitado em prod via fail-closed em Load().
const devSecret = "dev-only-secret-not-for-production"

func Load() (*Config, error) {
	dbURL := env("PAYMENT_DB_URL", "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable")
	brokers := strings.Split(env("REDPANDA_BROKERS", "localhost:19092"), ",")
	provider := strings.ToLower(env("PSP_PROVIDER", "stripe"))
	devMode := env("DEV_MODE", "false") == "true"

	// JWT_SECRET fail-closed (audit transversal — mesmo padrão de auth/order).
	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if !devMode {
			return nil, fmt.Errorf("JWT_SECRET is required (set DEV_MODE=true for local dev)")
		}
		jwt = devSecret
	}
	if !devMode && (strings.HasPrefix(jwt, "change-me") || jwt == devSecret || len(jwt) < 32) {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 chars and not a development default")
	}

	// A2 (auditoria 2026-07-18): DEV_MODE liga o fallback de header
	// X-User-Role, sem verificação criptográfica. Ligado por engano em
	// produção, `X-User-Role: admin` vira acesso de administrador — sem alarme
	// e sem sintoma. Recusar subir é preferível a comprometimento silencioso.
	if err := devguard.Check(devMode, os.Getenv("PAYMENT_DB_URL")); err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:            env("PORT", "8090"),
		DatabaseURL:     dbURL,
		RedpandaBrokers: brokers,
		JWTSecret:       jwt,
		DevMode:         devMode,
		AllowedOrigins:  parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		OrderServiceURL: env("ORDER_SERVICE_URL", "http://localhost:8092"),
		AuthServiceURL:  env("AUTH_SERVICE_URL", "http://localhost:8093"),
		PSPProvider:     provider,

		StripeSecretKey:      os.Getenv("STRIPE_SECRET_KEY"),
		StripePublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
		StripeWebhookSecret:  os.Getenv("STRIPE_WEBHOOK_SECRET"),

		MPAccessToken:   os.Getenv("MP_ACCESS_TOKEN"),
		MPPublicKey:     os.Getenv("MP_PUBLIC_KEY"),
		MPWebhookSecret: os.Getenv("MP_WEBHOOK_SECRET"),

		AppmaxAccessToken:   os.Getenv("APPMAX_ACCESS_TOKEN"),
		AppmaxWebhookSecret: os.Getenv("APPMAX_WEBHOOK_SECRET"),

		AppmaxV1AuthURL:      os.Getenv("APPMAX_V1_AUTH_URL"),
		AppmaxV1APIURL:       os.Getenv("APPMAX_V1_API_URL"),
		AppmaxV1ClientID:     os.Getenv("APPMAX_V1_CLIENT_ID"),
		AppmaxV1ClientSecret: os.Getenv("APPMAX_V1_CLIENT_SECRET"),
		AppmaxV1ExternalID:   os.Getenv("APPMAX_V1_EXTERNAL_ID"),

		RedisURL:     os.Getenv("REDIS_URL"),
		MetricsToken: os.Getenv("METRICS_TOKEN"),
	}

	// Valida credenciais do provider escolhido + webhook secret fail-closed em prod (audit C5).
	switch cfg.PSPProvider {
	case "stripe":
		if cfg.StripeSecretKey == "" {
			return nil, fmt.Errorf("STRIPE_SECRET_KEY is required when PSP_PROVIDER=stripe")
		}
		if !devMode && cfg.StripeWebhookSecret == "" {
			return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRET is required in non-dev mode (audit C5: fail-closed)")
		}
	case "mercadopago":
		if cfg.MPAccessToken == "" {
			return nil, fmt.Errorf("MP_ACCESS_TOKEN is required when PSP_PROVIDER=mercadopago")
		}
		if !devMode && cfg.MPWebhookSecret == "" {
			return nil, fmt.Errorf("MP_WEBHOOK_SECRET is required in non-dev mode (audit C5: fail-closed)")
		}
	case "appmax":
		if cfg.AppmaxAccessToken == "" {
			return nil, fmt.Errorf("APPMAX_ACCESS_TOKEN is required when PSP_PROVIDER=appmax")
		}
		// Nota: a Appmax não assina postbacks (sem HMAC), então não há webhook
		// secret obrigatório. A integridade do webhook é garantida pela
		// re-consulta via GetPayment no handler (audit C3). APPMAX_WEBHOOK_SECRET
		// é opcional (defesa em profundidade via header X-Appmax-Token).
	case "appmax-v1":
		// Fail-closed nas credenciais OAuth2 — sem elas nenhum request /v1/* sai.
		if cfg.AppmaxV1ClientID == "" {
			return nil, fmt.Errorf("APPMAX_V1_CLIENT_ID is required when PSP_PROVIDER=appmax-v1")
		}
		if cfg.AppmaxV1ClientSecret == "" {
			return nil, fmt.Errorf("APPMAX_V1_CLIENT_SECRET is required when PSP_PROVIDER=appmax-v1")
		}
		// Em prod exigimos as URLs explícitas: um deploy que esqueceu de apontar
		// pro sandbox cobraria de verdade (e vice-versa).
		if !devMode && (cfg.AppmaxV1AuthURL == "" || cfg.AppmaxV1APIURL == "") {
			return nil, fmt.Errorf("APPMAX_V1_AUTH_URL and APPMAX_V1_API_URL are required in non-dev mode (evita apontar pro ambiente errado)")
		}
		// Nota: a Appmax não assina webhooks (nem v3 nem v1). A integridade vem da
		// re-consulta GET /v1/orders/{id} (audit C3); APPMAX_WEBHOOK_SECRET é
		// opcional (header X-Appmax-Token, defesa em profundidade).
	default:
		return nil, fmt.Errorf("invalid PSP_PROVIDER=%q (expected: stripe | mercadopago | appmax | appmax-v1)", cfg.PSPProvider)
	}

	return cfg, nil
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
