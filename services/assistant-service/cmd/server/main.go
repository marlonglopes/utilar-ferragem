// assistant-service — a Alice ✨, assistente da UtiLar Ferragem.
// Orquestrador Claude (tool use → catalog-service) atrás de um endpoint de chat.
// Sem ANTHROPIC_API_KEY, roda em MODO MOCK (guiado por regras + busca real).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/utilar/assistant-service/internal/catalog"
	"github.com/utilar/assistant-service/internal/config"
	"github.com/utilar/assistant-service/internal/handler"
	"github.com/utilar/assistant-service/internal/alice"
	"github.com/utilar/assistant-service/internal/llm"
	"github.com/utilar/pkg/ratelimit"
)

// Cotas do /chat. Muito mais apertadas que as dos outros serviços (catalog:
// 100/min) porque cada request aqui custa tokens de LLM, não um SELECT.
//
// Anônimo é o default da internet inteira, então leva a cota de "uma conversa
// humana": ~1 mensagem a cada 6s. Autenticado ganha folga — tem conta, tem
// identidade rastreável e é o cliente que a Alice existe pra atender.
var (
	anonLimit   = ratelimit.Limit{Max: 10, Window: time.Minute}
	authedLimit = ratelimit.Limit{Max: 30, Window: time.Minute}
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()

	var model llm.LLM
	if cfg.AnthropicAPIKey != "" {
		model = llm.NewClaude(cfg.AnthropicAPIKey, cfg.Model)
		slog.Info("alice: Claude ativo", "model", cfg.Model)
	} else {
		model = llm.NewMock()
		slog.Warn("alice: ANTHROPIC_API_KEY ausente — MODO MOCK (regras + busca real)")
	}

	engine := alice.New(model, catalog.New(cfg.CatalogServiceURL))
	chatH := handler.NewChatHandler(engine)

	// Rate limit por IP/usuário via Redis (mesmo padrão e mesmo fail-open do
	// catalog-service). Sem REDIS_URL o limiter fica desligado — e como aqui o
	// endpoint desprotegido queima orçamento da Anthropic, o log é de erro, não
	// aviso de rodapé.
	var chatRL gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		rl := ratelimit.New(redis.NewClient(opts))
		chatRL = handler.TieredRateLimit(
			ratelimit.Middleware(rl, "assistant:chat:anon", anonLimit, ratelimit.IPKey),
			ratelimit.Middleware(rl, "assistant:chat:user", authedLimit, ratelimit.UserKey),
		)
		slog.Info("rate limit enabled", "redis", opts.Addr, "anon_per_min", anonLimit.Max, "authed_per_min", authedLimit.Max)
	} else {
		slog.Error("REDIS_URL ausente — rate limit do /chat DESLIGADO; endpoint aberto a drenar o orçamento da Anthropic")
	}

	if cfg.AllowedOrigins == "" && cfg.DevMode {
		slog.Warn("DEV_MODE=true e ALLOWED_ORIGINS vazio — CORS wildcard; nunca use em produção")
	}
	if cfg.JWTSecret == "" {
		slog.Warn("JWT_SECRET ausente — todo tráfego do /chat conta como anônimo (cota apertada)")
	}

	r := gin.New()
	r.Use(gin.Recovery(), handler.CORS(cfg.AllowedOrigins, cfg.DevMode))

	// Ordem importa: corta o body ANTES de tudo; identifica o usuário antes do
	// rate limit (senão o autenticado nunca alcança o balde folgado).
	chat := []gin.HandlerFunc{
		handler.LimitBody(handler.MaxRequestBytes),
		handler.OptionalAuth(cfg.JWTSecret),
	}
	if chatRL != nil {
		chat = append(chat, chatRL)
	}
	chat = append(chat, chatH.Chat)
	r.POST("/api/v1/assistant/chat", chat...)

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "model": model.Name()}) })

	// WriteTimeout generoso: uma resposta da Alice pode passar por até maxTurns
	// (4) chamadas ao modelo mais as tools contra o catalog-service.
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		slog.Info("assistant-service (Alice) ouvindo", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
