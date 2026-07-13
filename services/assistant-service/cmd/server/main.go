// assistant-service — a Lara ✨, assistente da UtiLar Ferragem.
// Orquestrador Claude (tool use → catalog-service) atrás de um endpoint de chat.
// Sem ANTHROPIC_API_KEY, roda em MODO MOCK (guiado por regras + busca real).
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/utilar/assistant-service/internal/catalog"
	"github.com/utilar/assistant-service/internal/config"
	"github.com/utilar/assistant-service/internal/handler"
	"github.com/utilar/assistant-service/internal/lara"
	"github.com/utilar/assistant-service/internal/llm"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	var model llm.LLM
	if cfg.AnthropicAPIKey != "" {
		model = llm.NewClaude(cfg.AnthropicAPIKey, cfg.Model)
		slog.Info("lara: Claude ativo", "model", cfg.Model)
	} else {
		model = llm.NewMock()
		slog.Warn("lara: ANTHROPIC_API_KEY ausente — MODO MOCK (regras + busca real)")
	}

	engine := lara.New(model, catalog.New(cfg.CatalogServiceURL))
	chatH := handler.NewChatHandler(engine)

	r := gin.New()
	r.Use(gin.Recovery(), cors(cfg.AllowedOrigins))

	r.POST("/api/v1/assistant/chat", chatH.Chat)
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "model": model.Name()}) })

	slog.Info("assistant-service (Lara) ouvindo", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}

// cors — whitelist por vírgula; vazio = "*".
func cors(allowed string) gin.HandlerFunc {
	set := map[string]struct{}{}
	for _, o := range strings.Split(allowed, ",") {
		if v := strings.TrimSpace(o); v != "" {
			set[v] = struct{}{}
		}
	}
	wildcard := len(set) == 0
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if wildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if _, ok := set[origin]; ok && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
