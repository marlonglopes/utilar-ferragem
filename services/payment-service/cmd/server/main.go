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

	"github.com/utilar/payment-service/internal/config"
	"github.com/utilar/payment-service/internal/db"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/orderclient"
	"github.com/utilar/payment-service/internal/outbox"
	"github.com/utilar/payment-service/internal/psp"
	mpgateway "github.com/utilar/payment-service/internal/psp/mercadopago"
	stripegateway "github.com/utilar/payment-service/internal/psp/stripe"
	"github.com/utilar/pkg/idempotency"
	"github.com/utilar/pkg/ratelimit"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("db open", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		slog.Error("db migrate", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	// Select PSP gateway baseado em PSP_PROVIDER
	var gateway psp.Gateway
	switch cfg.PSPProvider {
	case "stripe":
		gateway = stripegateway.New(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	case "mercadopago":
		gateway = mpgateway.New(cfg.MPAccessToken, cfg.MPWebhookSecret)
	default:
		slog.Error("unknown PSP_PROVIDER", "provider", cfg.PSPProvider)
		os.Exit(1)
	}
	slog.Info("psp gateway selected", "provider", gateway.Name())

	drainer, err := outbox.NewDrainer(database, cfg.RedpandaBrokers)
	if err != nil {
		slog.Error("outbox drainer init", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go drainer.Run(ctx)

	// Router
	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
	)

	// Cliente HTTP pra order-service (audit C1, C2 — server-side amount/ownership).
	orderC := orderclient.New(cfg.OrderServiceURL)

	paymentH := handler.NewPaymentHandler(database, gateway, orderC, cfg.DevMode)
	webhookH := handler.NewWebhookHandler(database, gateway)

	// Webhook endpoint provider-agnostic. O `:provider` precisa bater com
	// gateway.Name() — assim o atacante não consegue forçar webhook pra um
	// provider inativo.
	r.POST("/webhooks/:provider", webhookH.Handle)

	// H4 + H1: rate limit + Idempotency-Key em POST /payments.
	// Sem REDIS_URL (dev): ambos features ficam desligadas — log warn explícito.
	var paymentRL gin.HandlerFunc
	var paymentIdem gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		rdb := redis.NewClient(opts)
		paymentRL = ratelimit.Middleware(
			ratelimit.New(rdb),
			"payment:create",
			ratelimit.Limit{Max: 10, Window: time.Minute},
			ratelimit.UserKey, // por user_id (limita atacante autenticado, não IP só)
		)
		paymentIdem = idempotency.Middleware(idempotency.New(rdb, 24*time.Hour), "payment:create")
		slog.Info("rate limit + idempotency enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — payment rate limit + idempotency DISABLED (H1, H4 unprotected)")
	}

	api := r.Group("/api/v1", handler.JWTMiddleware(cfg.JWTSecret))
	{
		// Idempotency vai ANTES do rate limit: requisição replayed do cache não
		// deve consumir cota. Ordem: idem (replay rápido) → ratelimit → handler.
		createChain := []gin.HandlerFunc{}
		if paymentIdem != nil {
			createChain = append(createChain, paymentIdem)
		}
		if paymentRL != nil {
			createChain = append(createChain, paymentRL)
		}
		createChain = append(createChain, paymentH.Create)
		api.POST("/payments", createChain...)
		api.GET("/payments/:id", paymentH.Get)
		api.POST("/payments/:id/sync", paymentH.Sync)
	}

	r.GET("/health", func(c *gin.Context) {
		if err := database.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"db": "down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("payment-service listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
