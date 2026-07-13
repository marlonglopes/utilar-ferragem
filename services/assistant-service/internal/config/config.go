package config

import "os"

type Config struct {
	Port           string
	AllowedOrigins string

	// Claude (Anthropic Messages API). Vazio = MODO MOCK (Lara responde com
	// respostas guiadas por regras + dados reais do catálogo, sem chamar a API) —
	// permite demonstrar sem chave, no mesmo espírito mock do resto do Utilar.
	AnthropicAPIKey string
	// Modelo padrão: claude-opus-4-8 (o mais capaz). Ajustável via LARA_MODEL —
	// a Gi do gifthy usa claude-haiku-4-5 por custo/latência em escala.
	Model string

	// CatalogServiceURL — a Lara busca fatos (produto/preço/estoque) aqui via
	// tool use; nunca inventa. É a "única fonte de fatos" (padrão da Gi).
	CatalogServiceURL string
}

func Load() *Config {
	return &Config{
		Port:              env("PORT", "8094"),
		AllowedOrigins:    os.Getenv("ALLOWED_ORIGINS"),
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		Model:             env("LARA_MODEL", "claude-opus-4-8"),
		CatalogServiceURL: env("CATALOG_SERVICE_URL", "http://localhost:8091"),
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
