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

	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/config"
	"github.com/utilar/order-service/internal/db"
	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/pkg/ratelimit"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err.Error(),
			"hint", "set JWT_SECRET to a 32+ char random value, or DEV_MODE=true for local dev")
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

	catalog := catalogclient.New(cfg.CatalogServiceURL)
	orderH := handler.NewOrderHandler(database, catalog, cfg.DevMode)

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
	)

	// O3-M3: rate limit em POST /orders. 20/min/user — folga acima do uso humano,
	// mas bloqueia bot que tenta inflar tabelas.
	var createRL gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		createRL = ratelimit.Middleware(
			ratelimit.New(redis.NewClient(opts)),
			"order:create",
			ratelimit.Limit{Max: 20, Window: time.Minute},
			ratelimit.UserKey,
		)
		slog.Info("rate limit enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — order rate limit DISABLED (O3-M3 unprotected)")
	}

	api := r.Group("/api/v1", handler.RequireUser(cfg.JWTSecret, cfg.DevMode))
	{
		if createRL != nil {
			api.POST("/orders", createRL, orderH.Create)
		} else {
			api.POST("/orders", orderH.Create)
		}
		api.GET("/orders", orderH.List)
		api.GET("/orders/:id", orderH.Get)
		api.PATCH("/orders/:id/cancel", orderH.Cancel)
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
		slog.Info("order-service listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
