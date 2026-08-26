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

	"github.com/utilar/payment-service/internal/authclient"
	"github.com/utilar/payment-service/internal/config"
	"github.com/utilar/payment-service/internal/db"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/ledger"
	"github.com/utilar/payment-service/internal/obs"
	"github.com/utilar/payment-service/internal/orderclient"
	"github.com/utilar/payment-service/internal/outbox"
	"github.com/utilar/payment-service/internal/psp"
	appmaxgateway "github.com/utilar/payment-service/internal/psp/appmax"
	appmaxv1gateway "github.com/utilar/payment-service/internal/psp/appmaxv1"
	mpgateway "github.com/utilar/payment-service/internal/psp/mercadopago"
	stripegateway "github.com/utilar/payment-service/internal/psp/stripe"
	"github.com/utilar/pkg/audit"
	"github.com/utilar/pkg/idempotency"
	"github.com/utilar/pkg/metrics"
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
	case "appmax":
		gateway = appmaxgateway.New(cfg.AppmaxAccessToken, cfg.AppmaxWebhookSecret)
	case "appmax-v1":
		// Appmax AppStore API v1 (OAuth2, centavos) — convive com o provider v3.
		gateway = appmaxv1gateway.New(appmaxv1gateway.Config{
			AuthURL:       cfg.AppmaxV1AuthURL,
			APIURL:        cfg.AppmaxV1APIURL,
			ClientID:      cfg.AppmaxV1ClientID,
			ClientSecret:  cfg.AppmaxV1ClientSecret,
			ExternalID:    cfg.AppmaxV1ExternalID,
			WebhookSecret: cfg.AppmaxWebhookSecret,
		})
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

	// Observabilidade. O registry é próprio do serviço (não o default global),
	// e as métricas de negócio se penduram nele.
	mreg := metrics.New("payment-service")
	bizMetrics := obs.New(mreg.Registerer())
	// Poller independente do drainer: se o drainer travar, uma métrica
	// alimentada por ele ficaria congelada e o alerta nunca dispararia.
	bizMetrics.StartDBPolling(ctx, database, 15*time.Second)

	// Trilha de auditoria + livro contábil.
	auditRec := audit.New(database, "payment-service")
	poster := ledger.NewPoster(database, auditRec)
	closer := ledger.NewCloser(database, auditRec)
	reconciler := ledger.NewReconciler(database, gateway, bizMetrics)

	go drainer.WithMetrics(bizMetrics).Run(ctx)

	// Router
	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		mreg.Middleware(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins, cfg.DevMode),
	)

	// /metrics: fail-closed por token (ver pkg/metrics.Handler). Sem
	// METRICS_TOKEN o endpoint responde 404 — nunca fica público por omissão.
	r.GET("/metrics", mreg.Handler(cfg.MetricsToken))
	if cfg.MetricsToken == "" {
		slog.Warn("METRICS_TOKEN não configurado — /metrics DESABILITADO (fail-closed)")
	}

	// Cliente HTTP pra order-service (audit C1, C2 — server-side amount/ownership).
	orderC := orderclient.New(cfg.OrderServiceURL)
	// M6: cliente pro auth-service pra buscar CPF do boleto.
	authC := authclient.New(cfg.AuthServiceURL)

	paymentH := handler.NewPaymentHandler(database, gateway, orderC, authC, cfg.DevMode)
	webhookH := handler.NewWebhookHandler(database, gateway).
		WithLedger(poster).
		WithMetrics(bizMetrics)
	ledgerH := handler.NewLedgerHandler(database, poster, closer, reconciler, auditRec)

	// H4 + H1: rate limit + Idempotency-Key em POST /payments; e rate limit por IP
	// no WEBHOOK. Sem REDIS_URL (dev): tudo desligado — log warn explícito.
	var paymentRL gin.HandlerFunc
	var paymentIdem gin.HandlerFunc
	var webhookRL gin.HandlerFunc
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
		// STRIDE D#3: sem limite, um flood de webhooks forjados vira um flood de
		// GetPayment ao PSP (amplificação — pode fazer a Appmax rate-limitar/banir a
		// conta) + tx no banco. Por IP: o PSP legítimo vem de poucos IPs; 120/min é
		// folga larga sobre o postback real e ainda corta o flood.
		webhookRL = ratelimit.Middleware(
			ratelimit.New(rdb), "payment:webhook",
			ratelimit.Limit{Max: 120, Window: time.Minute}, ratelimit.IPKey,
		)
		slog.Info("rate limit + idempotency enabled", "redis", opts.Addr)
	} else {
		slog.Warn("REDIS_URL not set — payment rate limit + idempotency + webhook limit DISABLED")
	}

	// Webhook endpoint provider-agnostic. O `:provider` precisa bater com
	// gateway.Name() — assim o atacante não consegue forçar webhook pra um
	// provider inativo. Rate-limit por IP (fecha a amplificação pro PSP).
	webhookChain := []gin.HandlerFunc{}
	if webhookRL != nil {
		webhookChain = append(webhookChain, webhookRL)
	}
	webhookChain = append(webhookChain, webhookH.Handle)
	r.POST("/webhooks/:provider", webhookChain...)

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

	// API contábil — role=admin obrigatório. Contrato em docs/ledger-api.md.
	// Livro contábil: admin + contador (LedgerRoles). A contabilidade é o
	// trabalho do contador; é o único lugar onde essa persona muta algo.
	adm := r.Group("/api/v1/ledger", handler.JWTMiddleware(cfg.JWTSecret), handler.RequireAnyRole(handler.LedgerRoles...))
	{
		adm.GET("/summary", ledgerH.Summary)
		adm.GET("/by-method", ledgerH.ByMethod)
		adm.GET("/daily", ledgerH.Daily)
		adm.GET("/trial-balance", ledgerH.TrialBalance)
		adm.GET("/entries", ledgerH.Entries)
		adm.GET("/transactions/:id", ledgerH.GetTransaction)
		adm.POST("/transactions/:id/reverse", ledgerH.Reverse)
		adm.GET("/periods", ledgerH.ListPeriods)
		adm.GET("/periods/:period", ledgerH.GetPeriod)
		adm.POST("/periods/:period/close", ledgerH.ClosePeriod)
		adm.POST("/reconcile", ledgerH.Reconcile)
		adm.GET("/discrepancies", ledgerH.Discrepancies)
		adm.POST("/discrepancies/:id/resolve", ledgerH.ResolveDiscrepancy)
		adm.GET("/export", ledgerH.Export)
		adm.GET("/audit", ledgerH.ListAudit)
		adm.GET("/audit/verify", ledgerH.VerifyAudit)
	}

	// Rotas SERVIÇO→SERVIÇO. Hoje só o lançamento contábil da liquidação
	// externa (venda de balcão paga na maquininha da loja), chamado pelo
	// order-service. Contrato em docs/external-settlement.md.
	//
	// FAIL-CLOSED: sem SERVICE_JWT_SECRET o grupo NÃO é registrado. A rota
	// lança receita no livro sem que dinheiro nenhum tenha passado pelo nosso
	// PSP — ela não pode existir aceitando token de usuário.
	if cfg.ServiceVerifier != nil {
		extH := handler.NewExternalSettlementHandler(poster)
		internal := r.Group("/internal/v1", handler.RequireService(cfg.ServiceVerifier))
		internal.POST("/ledger/external-settlement", extH.Post)

		// Estorno de devolução (CDC). Mesma fronteira de confiança: a decisão
		// é do order-service (é ele que tem o pedido, o prazo legal e a trilha
		// de quem aprovou); aqui só cai a partida dobrada.
		refH := handler.NewReturnRefundHandler(poster)
		internal.POST("/ledger/return-refund", refH.Post)

		// Estorno REAL no PSP (order → payment → gateway.Refund). Idempotente por
		// returnID (tabela psp_refunds); NÃO toca o livro (isso é o return-refund
		// acima, fonte única). Precisa do gateway + banco.
		pspRefH := handler.NewRefundHandler(database, gateway)
		internal.POST("/refunds", pspRefH.Post)
	} else {
		slog.Warn("SERVICE_JWT_SECRET não configurado — /internal DESABILITADO; " +
			"liquidação externa de balcão e estorno de devolução não serão " +
			"lançados no livro contábil")
	}

	// Vigia da credencial do PSP. Sem isto, uma chave expirada só aparecia na
	// primeira venda, como 502 na cara do cliente, com o /health dizendo "ok".
	pspCheck := handler.NewPSPCheck(gateway, 5*time.Minute)
	pspCtx, stopPSPCheck := context.WithCancel(context.Background())
	defer stopPSPCheck()
	go pspCheck.Run(pspCtx)

	// Configuração de pagamento em LEITURA para o painel (qual PSP, métodos,
	// saúde). Admin + contador (LedgerRoles): é informação financeira, sem
	// segredo. NUNCA devolve credencial — trocar PSP/chave segue sendo variável
	// de ambiente.
	padm := r.Group("/api/v1/admin", handler.JWTMiddleware(cfg.JWTSecret), handler.RequireAnyRole(handler.LedgerRoles...))
	padm.GET("/payment/config", handler.NewPaymentConfigHandler(gateway, pspCheck).Get)

	r.GET("/health", func(c *gin.Context) {
		if err := database.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"db": "down"})
			return
		}
		// Credencial inválida NÃO derruba o health para 503: o serviço continua
		// atendendo consulta de pagamento e webhook, e derrubar tiraria o pod do
		// balanceador sem resolver nada — o problema é configuração, não
		// capacidade. Sai como "degraded" para o painel mostrar em vermelho.
		if est := pspCheck.Estado(); !est.OK {
			c.JSON(http.StatusOK, gin.H{"status": "degraded", "db": "ok", "psp": est})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // STRIDE D: fecha slowloris de header
		MaxHeaderBytes:    1 << 16,         // 64KB — teto de header
		WriteTimeout:      30 * time.Second,
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
