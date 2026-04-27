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

	"github.com/utilar/auth-service/internal/config"
	"github.com/utilar/auth-service/internal/db"
	"github.com/utilar/auth-service/internal/handler"
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

	authH := handler.NewAuthHandler(database, cfg)
	addrH := handler.NewAddressHandler(database)

	// A14-M5: cleanup periódico de tokens expirados (refresh, reset, verify).
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	handler.StartTokenCleanup(cleanupCtx, database)

	// A6-H2: rate limiter via Redis. Em dev sem REDIS_URL, mid noop (limiters nil-safe).
	var loginRL, forgotRL, resetRL, verifyRL gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		rl := ratelimit.New(redis.NewClient(opts))
		loginRL = ratelimit.Middleware(rl, "auth:login", ratelimit.Limit{Max: 5, Window: time.Minute}, ratelimit.IPKey)
		forgotRL = ratelimit.Middleware(rl, "auth:forgot", ratelimit.Limit{Max: 5, Window: time.Minute}, ratelimit.IPKey)
		resetRL = ratelimit.Middleware(rl, "auth:reset", ratelimit.Limit{Max: 5, Window: time.Minute}, ratelimit.IPKey)
		verifyRL = ratelimit.Middleware(rl, "auth:verify", ratelimit.Limit{Max: 10, Window: time.Minute}, ratelimit.IPKey)
		slog.Info("rate limit enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — auth rate limit DISABLED (A6-H2 unprotected)")
	}

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
	)

	pub := r.Group("/api/v1")
	{
		pub.POST("/auth/register", authH.Register)
		pub.POST("/auth/login", withRL(loginRL, authH.Login)...)
		pub.POST("/auth/refresh", authH.Refresh)
		pub.POST("/auth/forgot-password", withRL(forgotRL, authH.ForgotPassword)...)
		pub.POST("/auth/reset-password", withRL(resetRL, authH.ResetPassword)...)
		pub.POST("/auth/verify-email", withRL(verifyRL, authH.VerifyEmail)...)
	}

	priv := r.Group("/api/v1", handler.JWTAuth(cfg.JWTSecret))
	{
		priv.GET("/me", authH.Me)
		priv.POST("/auth/logout", authH.Logout)
		priv.GET("/addresses", addrH.List)
		priv.POST("/addresses", addrH.Create)
		priv.DELETE("/addresses/:id", addrH.Delete)
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
		slog.Info("auth-service listening", "port", cfg.Port)
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

// withRL prepende um rate-limit middleware (se não-nil) na lista de handlers
// passada pra rota. Quando rl é nil (REDIS_URL ausente), passa só o handler
// final — desligar o limiter em dev sem ter que ramificar a chamada de rota.
func withRL(rl gin.HandlerFunc, h gin.HandlerFunc) []gin.HandlerFunc {
	if rl == nil {
		return []gin.HandlerFunc{h}
	}
	return []gin.HandlerFunc{rl, h}
}
