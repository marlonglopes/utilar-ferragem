package config

import "os"

type Config struct {
	Port           string
	AllowedOrigins string

	// DevMode libera conveniências de desenvolvimento que NUNCA podem valer em
	// produção — hoje, apenas o CORS wildcard quando ALLOWED_ORIGINS está vazio.
	// Sem DEV_MODE, ALLOWED_ORIGINS vazio = nenhuma origem liberada (fail-closed).
	DevMode bool

	// RedisURL habilita o rate limit por IP (pkg/ratelimit, mesmo padrão do
	// catalog-service). Vazio = limiter DESLIGADO — aceitável só em dev, porque
	// o /chat gasta tokens da API da Anthropic a cada request.
	RedisURL string

	// JWTSecret valida o Bearer opcional. A Alice atende visitante anônimo (é o
	// produto), então o JWT não é exigido; ele só serve pra dar ao usuário
	// logado uma cota de rate limit mais folgada. Vazio = todo mundo é anônimo.
	JWTSecret string

	// Claude (Anthropic Messages API). Vazio = MODO MOCK (Alice responde com
	// respostas guiadas por regras + dados reais do catálogo, sem chamar a API) —
	// permite demonstrar sem chave, no mesmo espírito mock do resto do Utilar.
	AnthropicAPIKey string
	// Modelo padrão: claude-sonnet-5 (equilíbrio custo-capacidade). Ajustável via ALICE_MODEL —
	// a Gi do gifthy usa claude-haiku-4-5 por custo/latência em escala.
	Model string

	// CatalogServiceURL — a Alice busca fatos (produto/preço/estoque) aqui via
	// tool use; nunca inventa. É a "única fonte de fatos" (padrão da Gi).
	CatalogServiceURL string

	// OrderServiceURL — origem dos padrões AGREGADOS de co-compra
	// ("quem levou X também levou Y"). Vazio = sugestão por co-compra desligada;
	// a Alice continua sugerindo complementares por regra técnica, que vêm da
	// base de conhecimento. Nunca se inventa co-compra.
	OrderServiceURL string

	// ServiceToken é a credencial de serviço para os endpoints INTERNOS
	// (custo/margem no catálogo, co-compra no order-service).
	//
	// Vazio = o modo balcão roda SEM custo. Escolha deliberada: sem credencial,
	// a Alice diz que não tem o custo em vez de tentar adivinhá-lo, e o site
	// público segue idêntico. É o padrão que falha fechado.
	ServiceToken string
}

func Load() *Config {
	return &Config{
		Port:              env("PORT", "8094"),
		AllowedOrigins:    os.Getenv("ALLOWED_ORIGINS"),
		DevMode:           os.Getenv("DEV_MODE") == "true",
		RedisURL:          os.Getenv("REDIS_URL"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		Model:             env("ALICE_MODEL", "claude-sonnet-5"),
		CatalogServiceURL: env("CATALOG_SERVICE_URL", "http://localhost:8091"),
		OrderServiceURL:   os.Getenv("ORDER_SERVICE_URL"),
		ServiceToken:      os.Getenv("SERVICE_TOKEN"),
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
