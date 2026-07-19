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

	"github.com/utilar/order-service/internal/authclient"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/config"
	"github.com/utilar/order-service/internal/consumer"
	"github.com/utilar/order-service/internal/db"
	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/order-service/internal/paymentclient"
	"github.com/utilar/order-service/internal/shipping"
	"github.com/utilar/pkg/idempotency"
	"github.com/utilar/pkg/metrics"
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

	// NewWithSecret: o mesmo cliente serve pra consultar preço (rota pública) e
	// pra reservar estoque (rotas /internal, que exigem token role=service
	// assinado com o JWT_SECRET compartilhado).
	catalog := catalogclient.NewWithSecret(cfg.CatalogServiceURL, cfg.ServiceJWTSecret)
	rates := shipping.NewStore(database)

	// authclient: de onde sai o TETO DE DESCONTO autoritativo do operador de
	// balcão. Sem ele o balcão opera fail-closed (teto 0 → todo desconto vai
	// para a fila do gerente), nunca fail-open.
	authc := authclient.New(cfg.AuthServiceURL, cfg.ServiceJWTSecret)

	// paymentclient: por onde a liquidação externa (venda de balcão paga na
	// maquininha da loja) chega ao livro contábil, que vive no payment-service.
	// Ver docs/external-settlement.md.
	paymentc := paymentclient.New(cfg.PaymentServiceURL, cfg.ServiceJWTSecret)

	orderH := handler.NewOrderHandler(database, catalog, cfg.DevMode).
		WithStock(catalog).
		WithShipping(rates).
		WithOperators(authc).
		WithLedger(paymentc)
	shippingH := handler.NewShippingHandler(rates)

	// Consumer dos eventos de pagamento — é ele que faz o pedido virar 'paid'.
	// Sem KAFKA_BROKERS o loop pagamento→pedido fica aberto: o cliente paga e o
	// pedido não sai de pending_payment. Por isso o aviso é gritado no boot.
	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	if len(cfg.KafkaBrokers) > 0 {
		pc, err := consumer.New(database, cfg.KafkaBrokers, catalog)
		if err != nil {
			slog.Error("payment consumer init", "error", err)
			os.Exit(1)
		}
		go pc.Run(consumerCtx)
	} else {
		slog.Warn("KAFKA_BROKERS not set — payment consumer DISABLED; " +
			"orders will stay in pending_payment after payment confirmation")
	}

	// Métricas. Sem isto o painel de observabilidade mostra o order-service
	// "de pé" e nada mais — e o order-service é onde o pedido trava depois de
	// pago, exatamente o incidente que o painel existe para pegar.
	mreg := metrics.New("order-service")

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
		mreg.Middleware(),
	)

	// Fail-closed por token (pkg/metrics.Handler): sem METRICS_TOKEN, 404.
	r.GET("/metrics", mreg.Handler(cfg.MetricsToken))
	if cfg.MetricsToken == "" {
		slog.Warn("METRICS_TOKEN não configurado — /metrics DESABILITADO (fail-closed)")
	}

	// O3-M3: rate limit em POST /orders. 20/min/user — folga acima do uso humano,
	// mas bloqueia bot que tenta inflar tabelas.
	// L-ORDER-2: Idempotency-Key middleware (mesma key 24h) — evita pedidos
	// duplicados em retry/double-click.
	var createRL gin.HandlerFunc
	var createIdem gin.HandlerFunc
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis url", "error", err)
			os.Exit(1)
		}
		rdb := redis.NewClient(opts)
		createRL = ratelimit.Middleware(
			ratelimit.New(rdb),
			"order:create",
			ratelimit.Limit{Max: 20, Window: time.Minute},
			ratelimit.UserKey,
		)
		createIdem = idempotency.Middleware(idempotency.New(rdb, 24*time.Hour), "order:create")
		slog.Info("rate limit + idempotency enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — order rate limit + idempotency DISABLED")
	}

	api := r.Group("/api/v1", handler.RequireUser(cfg.JWTSecret, cfg.DevMode))
	{
		// Idempotency primeiro (replay rápido), depois rate limit, depois handler.
		createChain := []gin.HandlerFunc{}
		if createIdem != nil {
			createChain = append(createChain, createIdem)
		}
		if createRL != nil {
			createChain = append(createChain, createRL)
		}
		createChain = append(createChain, orderH.Create)
		api.POST("/orders", createChain...)
		api.GET("/orders", orderH.List)
		api.GET("/orders/:id", orderH.Get)
		api.PATCH("/orders/:id/cancel", orderH.Cancel)

		// Cotação de frete — o carrinho chama com o CEP antes do checkout.
		// Contrato em docs/shipping-api.md.
		api.POST("/shipping/quote", shippingH.Quote)
	}

	// PDV de balcão. Fica sob RequireUser (não RequireRole) porque a decisão de
	// papel/loja/cargo é por recurso, não por rota: quem autoriza é
	// internal/balcao, com o teto de desconto resolvido no auth-service.
	// Uma lista de papéis na rota daria a falsa sensação de que a autorização
	// já foi feita.
	bal := r.Group("/api/v1/balcao", handler.RequireUser(cfg.JWTSecret, cfg.DevMode))
	{
		bal.GET("/approvals", orderH.ListPendingApprovals)
		bal.PATCH("/orders/:id/approve", orderH.Approve)
		bal.PATCH("/orders/:id/reject", orderH.Reject)

		// Liquidação externa: a venda paga na maquininha da loja (adquirente
		// próprio, fora da Appmax). É o endpoint que declara um pedido pago
		// sem que dinheiro nenhum tenha entrado no nosso sistema — a
		// autorização (só operador da própria loja / admin) é por recurso, em
		// internal/balcao, e a trilha de auditoria é fail-closed.
		// Contrato em docs/external-settlement.md.
		bal.POST("/orders/:id/settle-external", orderH.SettleExternal)
	}

	// Rotas de operação (separação, despacho, entrega). role=admin ou operator:
	// quem embala não precisa de poder de admin sobre o resto do sistema.
	ops := r.Group("/api/v1/admin", handler.RequireRole(cfg.JWTSecret, cfg.DevMode, "admin", "operator"))
	{
		ops.PATCH("/orders/:id/picking", orderH.MarkPicking)
		ops.PATCH("/orders/:id/shipped", orderH.MarkShipped)
		ops.PATCH("/orders/:id/delivered", orderH.MarkDelivered)
		ops.PATCH("/orders/:id/cancel", orderH.AdminCancel)
	}

	// Painel do dono — grupo SEPARADO do `ops` acima, e de propósito.
	//
	// `ops` aceita role=operator porque quem separa pedido precisa marcar
	// despacho. O painel NÃO: ele agrega faturamento, margem e custo da
	// operação inteira. Operador de balcão vendo a margem de todas as lojas e o
	// faturamento consolidado é vazamento de inteligência de negócio, e
	// `store_operator` no `RequireRole` do painel seria exatamente isso.
	// Só role=admin. Ver docs/admin-dashboard-api.md § Autorização.
	dash := handler.NewAdminDashboardHandler(database, catalog, authc)
	adminDash := r.Group("/api/v1/admin", handler.RequireRole(cfg.JWTSecret, cfg.DevMode, "admin"))
	{
		adminDash.GET("/overview", dash.Overview)
		adminDash.GET("/sellers/performance", dash.SellersPerformance)
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
