package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/utilar/catalog-service/internal/config"
	"github.com/utilar/catalog-service/internal/db"
	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/catalog-service/internal/reservation"
	"github.com/utilar/catalog-service/internal/storage"
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

	// Storage de mídia. A INTERFACE é a fronteira: o handler de upload não sabe
	// se grava em disco ou no S3. STORAGE_DRIVER=local|s3 decide; local é dev,
	// S3 é produção (ver internal/storage/s3.go para o que ainda falta).
	mediaStore, err := storage.FromEnv()
	if err != nil {
		slog.Error("storage", "error", err)
		os.Exit(1)
	}
	slog.Info("media storage", "driver", mediaStore.Driver())

	productH := handler.NewProductHandler(database).WithMedia(mediaStore)
	categoryH := handler.NewCategoryHandler(database)
	sellerH := handler.NewSellerHandler(database)
	adminProductH := handler.NewAdminProductHandler(database)
	catalogAdminH := handler.NewCatalogAdminHandler(database)
	reservationH := handler.NewReservationHandler(database)
	importH := handler.NewImportHandler(database)
	storeCostH := handler.NewStoreCostHandler(database)
	productImageH := handler.NewProductImageHandler(database, mediaStore)

	if cfg.DevMode {
		slog.Warn("DEV_MODE=true — /admin aceita fallback X-User-Role; nunca use em produção")
	}

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

	// Métricas. Até aqui só o payment-service instrumentava, o que deixava o
	// painel de observabilidade sem latência nem taxa de erro para 3 dos 4
	// serviços — ou seja, cego justamente onde a loja é lida (o catálogo é o
	// caminho mais quente do sistema).
	mreg := metrics.New("catalog-service")

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
		mreg.Middleware(),
	)

	// /metrics é fail-closed por token (pkg/metrics.Handler): sem METRICS_TOKEN
	// responde 404 e nunca fica público por omissão.
	r.GET("/metrics", mreg.Handler(cfg.MetricsToken))
	if cfg.MetricsToken == "" {
		slog.Warn("METRICS_TOKEN não configurado — /metrics DESABILITADO (fail-closed) " +
			"e o painel /admin/observabilidade não terá latência deste serviço")
	}

	// L-CATALOG-1: cache headers — listings 1min, detail 5min.
	listCache := handler.CacheControl(60)
	detailCache := handler.CacheControl(300)

	api := r.Group("/api/v1")
	{
		api.GET("/categories", listCache, categoryH.List)
		api.GET("/sellers", listCache, sellerH.List)
		if searchRL != nil {
			api.GET("/products", searchRL, listCache, productH.List)
			api.GET("/products/facets", searchRL, listCache, productH.Facets)
		} else {
			api.GET("/products", listCache, productH.List)
			api.GET("/products/facets", listCache, productH.Facets)
		}
		api.GET("/products/by-id/:id", detailCache, productH.GetByID)
		api.GET("/products/:slug", detailCache, productH.GetBySlug)
		api.GET("/products/:slug/related", listCache, productH.Related)
		// Registry de atributos da categoria: contrato de forma da ficha
		// técnica (rótulo, tipo, unidade). Sem dado sensível — é o que o
		// frontend precisa pra montar os filtros técnicos.
		api.GET("/categories/:id/attributes", listCache, catalogAdminH.CategoryAttributes)
	}

	// Alvos do agregador de observabilidade. O próprio catalog entra na lista
	// (via localhost): um serviço que não se mede não sabe se é ele o lento.
	obsTargets := []handler.ServiceTarget{{Name: "catalog", BaseURL: "http://localhost:" + cfg.Port}}
	for _, t := range []handler.ServiceTarget{
		{Name: "auth", BaseURL: cfg.AuthServiceURL},
		{Name: "order", BaseURL: cfg.OrderServiceURL},
		{Name: "payment", BaseURL: cfg.PaymentServiceURL},
	} {
		// URL vazia = alvo omitido. Um "fora do ar" falso por falta de
		// configuração dispararia alerta crítico e treinaria o dono a ignorar
		// o painel — que é o pior estrago que um alerta pode causar.
		if strings.TrimSpace(t.BaseURL) != "" {
			obsTargets = append(obsTargets, t)
		}
	}
	obsH := handler.NewObservabilityHandler(obsTargets, cfg.MetricsToken)

	// Rotas de escrita (ingestão) — protegidas por role=admin.
	admin := r.Group("/api/v1/admin", handler.RequireAdmin(cfg.JWTSecret, cfg.DevMode))
	{
		admin.POST("/products", adminProductH.Create)
		admin.PATCH("/products/by-id/:id", adminProductH.Patch)
		admin.DELETE("/products/by-id/:id", adminProductH.Delete)
		// Imagem por URL (legado — o lojista cola o link de terceiro).
		admin.POST("/products/by-id/:id/images", adminProductH.AddImage)

		// UPLOAD de arquivo: multipart, várias imagens por chamada, tudo
		// normalizado no backend (1:1, letterbox branco, 3 resoluções, EXIF
		// aplicado e descartado). Ver docs/imagens-produto.md.
		//
		// Está sob o mesmo RequireAdmin do grupo: anônimo e `customer` tomam
		// 401/403 antes de um único byte ser lido. `store_operator` também NÃO
		// entra — operador de balcão não escreve no catálogo, e imagem de
		// produto é catálogo.
		admin.GET("/products/by-id/:id/images", productImageH.List)
		admin.POST("/products/by-id/:id/images/upload", productImageH.Upload)
		admin.PUT("/products/by-id/:id/images/order", productImageH.Reorder)
		admin.PUT("/products/by-id/:id/images/:imageId/cover", productImageH.SetCover)
		admin.DELETE("/products/by-id/:id/images/:imageId", productImageH.Delete)
		admin.POST("/products/import", adminProductH.Import)

		// ⚠️ ESTA é a única rota que devolve `cost`/margem. Está sob
		// RequireAdmin; nenhuma equivalente existe fora deste grupo.
		// Listagem de admin: devolve custo, margem e TODOS os status —
		// a pública esconde os dois de propósito. Sem ela a tela de gestão
		// de produtos abria com "Produto não encontrado".
		admin.GET("/products", catalogAdminH.ListProducts)
		admin.GET("/products/by-id/:id", catalogAdminH.GetProduct)
		admin.GET("/products/by-id/:id/price-history", catalogAdminH.GetPriceHistory)
		admin.PUT("/products/by-id/:id/price-tiers", catalogAdminH.SetPriceTiers)
		admin.PUT("/products/by-id/:id/attributes", catalogAdminH.SetProductAttributes)

		// Pipeline de ingestão multi-formato (CSV / XLSX / JSON / SINAPI).
		//
		// Dois passos por desenho: `batches` faz staging + DRY-RUN sem escrever
		// em `products`; `commit` aplica o que um humano revisou. Ver
		// internal/handler/admin_import.go e docs/ingestao-de-produtos.md.
		// `suggest` é o passo de MAPEAMENTO: detecta as colunas e propõe o
		// de/para com grau de confiança, para o humano confirmar. Não cria
		// perfil nem importa nada — sugerir não é decidir.
		admin.POST("/import/suggest", importH.SuggestColumns)
		admin.POST("/import/profiles", importH.CreateProfile)
		admin.GET("/import/profiles", importH.ListProfiles)
		admin.POST("/import/batches", importH.CreateBatch)
		admin.GET("/import/batches", importH.ListBatches)
		admin.GET("/import/batches/:id", importH.GetBatch)
		admin.POST("/import/batches/:id/commit", importH.CommitBatch)

		// ⚠️ SINAPI: o valor importado é CUSTO DE REFERÊNCIA PARA OBRA PÚBLICA
		// (Caixa/IBGE), carregado em `cost` — NUNCA em `price`. Os itens entram
		// como rascunho sem preço de venda. Ver docs/base-de-produtos.md.
		admin.POST("/import/sinapi", importH.ImportSINAPI)

		// Observabilidade agregada dos 4 serviços. Está sob o mesmo
		// RequireAdmin do grupo — anônimo toma 401, customer/seller/
		// store_operator tomam 403. Ver internal/handler/observability.go
		// para o PORQUÊ de a rota morar aqui e não no payment-service.
		admin.GET("/observability", obsH.Snapshot)
	}

	// Rotas internas de reserva de estoque — chamadas pelo order-service, não
	// pelo browser. Aceitam role=service (token que o order-service assina com o
	// SERVICE_JWT_SECRET) ou role=admin (operação manual/debug, token de
	// usuário). A1 (auditoria 2026-07-18): são segredos DIFERENTES — token de
	// usuário com a claim role=service não passa por aqui.
	internal := r.Group("/api/v1/internal", handler.RequireInternal(cfg.JWTSecret, cfg.ServiceJWTSecret, cfg.DevMode))
	{
		internal.POST("/reservations", reservationH.Reserve)
		internal.POST("/reservations/:orderId/commit", reservationH.Commit)
		internal.POST("/reservations/:orderId/release", reservationH.Release)
	}

	// Rotas do BALCÃO — leitura autenticada que o PDV faz. Aceita
	// role=store_operator, role=admin (token de usuário) e role=service (token
	// de serviço, para o order-service registrar o CMV do pedido de balcão).
	//
	// PORQUÊ um grupo separado do /admin: o /admin tem ESCRITA no catálogo
	// (preço, importação de planilha). Operador de balcão precisa VER o custo
	// pra negociar desconto; não pode mudar preço de produto. Grupo próprio
	// mantém a superfície do operador do tamanho da necessidade dele.
	store := r.Group("/api/v1/store", handler.RequireStore(cfg.JWTSecret, cfg.ServiceJWTSecret, cfg.DevMode))
	{
		// Em lote de propósito: o carrinho de balcão tem vários itens e uma
		// chamada por item seria N+1 no caminho mais quente da loja.
		store.GET("/products/costs", storeCostH.Costs)
		store.POST("/products/costs", storeCostH.CostsBatch)
	}

	// Sweeper de expiração: devolve à vitrine o estoque de carrinhos abandonados.
	sweeperCtx, stopSweeper := context.WithCancel(context.Background())
	defer stopSweeper()
	go reservation.NewSweeper(database).Run(sweeperCtx)

	// Servir mídia pelo próprio serviço é MODO DE DESENVOLVIMENTO. Em produção
	// o driver é S3 e quem serve é o CloudFront — a rota nem existe, porque
	// serviço de aplicação servindo estático desperdiça conexão e CPU.
	if local, ok := mediaStore.(*storage.Local); ok {
		mediaH := handler.NewMediaHandler(local)
		r.GET("/media/*path", mediaH.Serve)
		slog.Info("servindo mídia local", "root", local.Root(), "rota", "/media")
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
