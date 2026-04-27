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

	"github.com/utilar/catalog-service/internal/config"
	"github.com/utilar/catalog-service/internal/db"
	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/pkg/ratelimit"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

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

	productH := handler.NewProductHandler(database)
	categoryH := handler.NewCategoryHandler(database)
	sellerH := handler.NewSellerHandler(database)

	// CT1-H1: rate limit em /products (search). 100/min/IP. Outros endpoints
	// (categories, sellers, by-id) têm tráfego baixo e são alvos pouco
	// interessantes pra brute-force, não recebem limit (ainda).
	var searchRL gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		rl := ratelimit.New(redis.NewClient(opts))
		searchRL = ratelimit.Middleware(rl, "catalog:search", ratelimit.Limit{Max: 100, Window: time.Minute}, ratelimit.IPKey)
		slog.Info("rate limit enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — catalog rate limit DISABLED (CT1-H1 unprotected)")
	}

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
	)

	api := r.Group("/api/v1")
	{
		api.GET("/categories", categoryH.List)
		api.GET("/sellers", sellerH.List)
		if searchRL != nil {
			api.GET("/products", searchRL, productH.List)
			api.GET("/products/facets", searchRL, productH.Facets)
		} else {
			api.GET("/products", productH.List)
			api.GET("/products/facets", productH.Facets)
		}
		api.GET("/products/by-id/:id", productH.GetByID)
		api.GET("/products/:slug", productH.GetBySlug)
		api.GET("/products/:slug/related", productH.Related)
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
		slog.Info("catalog-service listening", "port", cfg.Port)
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

