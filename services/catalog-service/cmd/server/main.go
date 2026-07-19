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
	"github.com/utilar/catalog-service/internal/reservation"
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

	productH := handler.NewProductHandler(database)
	categoryH := handler.NewCategoryHandler(database)
	sellerH := handler.NewSellerHandler(database)
	adminProductH := handler.NewAdminProductHandler(database)
	catalogAdminH := handler.NewCatalogAdminHandler(database)
	reservationH := handler.NewReservationHandler(database)
	importH := handler.NewImportHandler(database)
	storeCostH := handler.NewStoreCostHandler(database)

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

	r := gin.New()
	r.Use(
		gin.Recovery(),
		handler.RequestID(),
		handler.AccessLog(),
		handler.SecurityHeaders(),
		handler.CORS(cfg.AllowedOrigins),
	)

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

	// Rotas de escrita (ingestão) — protegidas por role=admin.
	admin := r.Group("/api/v1/admin", handler.RequireAdmin(cfg.JWTSecret, cfg.DevMode))
	{
		admin.POST("/products", adminProductH.Create)
		admin.PATCH("/products/by-id/:id", adminProductH.Patch)
		admin.DELETE("/products/by-id/:id", adminProductH.Delete)
		admin.POST("/products/by-id/:id/images", adminProductH.AddImage)
		admin.DELETE("/products/by-id/:id/images/:imageId", adminProductH.DeleteImage)
		admin.POST("/products/import", adminProductH.Import)

		// ⚠️ ESTA é a única rota que devolve `cost`/margem. Está sob
		// RequireAdmin; nenhuma equivalente existe fora deste grupo.
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
