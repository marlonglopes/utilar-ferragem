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
	"github.com/utilar/payment-service/internal/config"
	"github.com/utilar/payment-service/internal/db"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/mercadopago"
	"github.com/utilar/payment-service/internal/outbox"
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

	mpClient := mercadopago.New(cfg.MPAccessToken)

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
	r.Use(gin.Recovery())

	paymentH := handler.NewPaymentHandler(database, mpClient)
	webhookH := handler.NewWebhookHandler(database, cfg.MPWebhookSecret)

	r.POST("/webhooks/mp", webhookH.HandleMercadoPago)

	api := r.Group("/api/v1", handler.JWTMiddleware(cfg.JWTSecret))
	{
		api.POST("/payments", paymentH.Create)
		api.GET("/payments/:id", paymentH.Get)
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
