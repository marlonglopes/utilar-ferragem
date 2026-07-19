package config

import (
	"errors"
	"os"
	"strings"

	"github.com/utilar/pkg/devguard"
	"github.com/utilar/pkg/servicetoken"
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

	// ServiceJWTSecret valida os tokens `role=service` das rotas
	// /api/v1/internal (reserva de estoque), emitidos pelo order-service.
	//
	// A1 (auditoria 2026-07-18): é DELIBERADAMENTE distinto do JWTSecret. Com um
	// segredo só, qualquer processo que o tivesse — inclusive o
	// assistant-service, que é público e recebe texto livre de visitante — podia
	// assinar `role=service` ou `role=admin` e reescrever o catálogo. Ver
	// pkg/servicetoken.
	ServiceJWTSecret string

	// MetricsToken protege /metrics (fail-closed: vazio = 404, ver
	// pkg/metrics.Handler) E é o token com que o agregador de observabilidade
	// deste serviço LÊ o /metrics dos outros. É o mesmo segredo dos dois lados
	// de propósito: são os quatro serviços da mesma malha interna, e um token
	// por par multiplicaria a superfície de rotação sem reduzir risco nenhum.
	MetricsToken string

	// URLs dos outros serviços — alvos do agregador de
	// /api/v1/admin/observability. Alvo com URL vazia é simplesmente omitido do
	// painel: melhor um serviço ausente da lista que um "fora do ar" falso
	// disparando alerta crítico porque ninguém configurou o endereço.
	AuthServiceURL    string
	OrderServiceURL   string
	PaymentServiceURL string
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

	// A2 (auditoria 2026-07-18): DEV_MODE liga o fallback de header
	// X-User-Role, que não tem verificação criptográfica nenhuma. Se essa
	// variável for ligada por engano em produção, qualquer requisição com
	// `X-User-Role: admin` vira acesso de administrador — sem alarme e sem
	// sintoma. Aqui o serviço se RECUSA A SUBIR: indisponibilidade barulhenta
	// é preferível a comprometimento silencioso.
	if err := devguard.Check(devMode, os.Getenv("CATALOG_DB_URL")); err != nil {
		return nil, err
	}

	// A1 (auditoria 2026-07-18): fail-closed. Fora de DEV_MODE, sem
	// SERVICE_JWT_SECRET o serviço não sobe — subir aceitando role=service
	// assinado com o segredo de usuário seria pior que ficar indisponível.
	serviceSecret, err := servicetoken.SecretFromEnv(devMode, jwt)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:             env("PORT", "8091"),
		DatabaseURL:      env("CATALOG_DB_URL", "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"),
		AllowedOrigins:   parseOrigins(os.Getenv("ALLOWED_ORIGINS")),
		RedisURL:         os.Getenv("REDIS_URL"),
		JWTSecret:        jwt,
		DevMode:          devMode,
		ServiceJWTSecret: serviceSecret,

		MetricsToken:      os.Getenv("METRICS_TOKEN"),
		AuthServiceURL:    env("AUTH_SERVICE_URL", "http://localhost:8093"),
		OrderServiceURL:   env("ORDER_SERVICE_URL", "http://localhost:8092"),
		PaymentServiceURL: env("PAYMENT_SERVICE_URL", "http://localhost:8090"),
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
