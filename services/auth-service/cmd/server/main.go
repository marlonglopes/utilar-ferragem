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
	"github.com/utilar/auth-service/internal/config"
	"github.com/utilar/auth-service/internal/db"
	"github.com/utilar/auth-service/internal/handler"
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

	authH := handler.NewAuthHandler(database, cfg)
	addrH := handler.NewAddressHandler(database)

	r := gin.New()
	r.Use(gin.Recovery(), handler.RequestID(), handler.AccessLog(), handler.CORS())

	pub := r.Group("/api/v1")
	{
		pub.POST("/auth/register", authH.Register)
		pub.POST("/auth/login", authH.Login)
		pub.POST("/auth/refresh", authH.Refresh)
		pub.POST("/auth/forgot-password", authH.ForgotPassword)
		pub.POST("/auth/reset-password", authH.ResetPassword)
		pub.POST("/auth/verify-email", authH.VerifyEmail)
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
