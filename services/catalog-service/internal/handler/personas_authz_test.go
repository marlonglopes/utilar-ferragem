package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// Fronteira de persona do catálogo (backoffice 2026-07). Monta os grupos com os
// MESMOS conjuntos de main.go (handler.Catalog*Roles) e prova, com token REAL
// assinado (devMode=false), quem entra e quem toma 403.
//
// O ponto crítico é o CUSTO: /admin/products é a única rota que devolve `cost`.
// Só admin e vendas entram — contador e almoxarife tomam 403, e é assim que o
// custo não vaza para eles (não se filtra o campo; nega-se a rota).
func TestPersonas_CatalogAuthzFailClosed(t *testing.T) {
	const secret = "test-secret-at-least-32-chars-long-xx"
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	cat := r.Group("/api/v1/admin", handler.RequireRole(secret, false, handler.CatalogAdminRoles...))
	cat.GET("/products", ok) // a rota do custo
	obs := r.Group("/api/v1/admin", handler.RequireRole(secret, false, handler.CatalogObsRoles...))
	obs.GET("/observability", ok)

	call := func(path, role string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+signToken(t, secret, "u-"+role, role))
		}
		r.ServeHTTP(w, req)
		return w.Code
	}

	const (
		products = "/api/v1/admin/products"
		obsPath  = "/api/v1/admin/observability"
	)

	cases := []struct {
		nome, path, role string
		want             int
	}{
		// Custo/catálogo: só admin e vendas.
		{"admin vê catálogo", products, "admin", http.StatusOK},
		{"vendas vê catálogo (e custo)", products, "vendas", http.StatusOK},
		{"contador NÃO vê catálogo/custo", products, "contador", http.StatusForbidden},
		{"almoxarife NÃO vê catálogo/custo", products, "almoxarife", http.StatusForbidden},
		{"store_operator NÃO (usa /store, não /admin)", products, "store_operator", http.StatusForbidden},
		{"customer barrado", products, "customer", http.StatusForbidden},
		{"seller barrado", products, "seller", http.StatusForbidden},
		{"anônimo 401", products, "", http.StatusUnauthorized},

		// Observabilidade: admin e contador; vendas/almoxarife não.
		{"contador vê saúde", obsPath, "contador", http.StatusOK},
		{"admin vê saúde", obsPath, "admin", http.StatusOK},
		{"vendas NÃO vê saúde", obsPath, "vendas", http.StatusForbidden},
		{"almoxarife NÃO vê saúde", obsPath, "almoxarife", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			if got := call(tc.path, tc.role); got != tc.want {
				t.Errorf("GET %s como %q → %d, queria %d", tc.path, tc.role, got, tc.want)
			}
		})
	}
}

// Config da loja (aviso da vitrine): o PUT é a VOZ INSTITUCIONAL da loja com o
// cliente, então é ADMIN-ONLY — nem `vendas` (que mantém o catálogo) muda o
// banner da home. Prova a fronteira com token real (devMode=false), espelhando
// o grupo `storeAdmin` de main.go. O GET é público e não entra aqui.
func TestPersonas_StoreSettingsAuthzFailClosed(t *testing.T) {
	const secret = "test-secret-at-least-32-chars-long-xx"
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	// "admin" = roles.Admin (o mesmo que main.go passa no grupo storeAdmin).
	storeAdmin := r.Group("/api/v1/admin", handler.RequireRole(secret, false, "admin"))
	storeAdmin.PUT("/store/settings", ok)

	call := func(role string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/store/settings", nil)
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+signToken(t, secret, "u-"+role, role))
		}
		r.ServeHTTP(w, req)
		return w.Code
	}

	cases := []struct {
		nome, role string
		want       int
	}{
		{"admin edita o aviso", "admin", http.StatusOK},
		{"vendas NÃO edita (não é a voz da loja)", "vendas", http.StatusForbidden},
		{"contador NÃO edita", "contador", http.StatusForbidden},
		{"almoxarife NÃO edita", "almoxarife", http.StatusForbidden},
		{"store_operator NÃO edita", "store_operator", http.StatusForbidden},
		{"customer barrado", "customer", http.StatusForbidden},
		{"seller barrado", "seller", http.StatusForbidden},
		{"anônimo 401", "", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			if got := call(tc.role); got != tc.want {
				t.Errorf("PUT /store/settings como %q → %d, queria %d", tc.role, got, tc.want)
			}
		})
	}
}

// Estoque (tela do almoxarife): VER é mais amplo que AJUSTAR. Prova que o
// almoxarife entra (e o custo nunca aparece porque a rota nem devolve), o
// vendas vê mas não ajusta pelo fluxo com motivo, e o contador fica fora.
func TestPersonas_StockAuthzFailClosed(t *testing.T) {
	const secret = "test-secret-at-least-32-chars-long-xx"
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	read := r.Group("/api/v1/admin", handler.RequireRole(secret, false, handler.StockReadRoles...))
	read.GET("/stock", ok)
	write := r.Group("/api/v1/admin", handler.RequireRole(secret, false, handler.StockWriteRoles...))
	write.POST("/stock/:id/adjust", ok)

	call := func(method, path, role string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+signToken(t, secret, "u-"+role, role))
		}
		r.ServeHTTP(w, req)
		return w.Code
	}

	const (
		list   = "/api/v1/admin/stock"
		adjust = "/api/v1/admin/stock/p1/adjust"
	)
	cases := []struct {
		nome, method, path, role string
		want                     int
	}{
		{"almoxarife vê estoque", http.MethodGet, list, "almoxarife", http.StatusOK},
		{"vendas vê estoque", http.MethodGet, list, "vendas", http.StatusOK},
		{"admin vê estoque", http.MethodGet, list, "admin", http.StatusOK},
		{"contador NÃO vê estoque", http.MethodGet, list, "contador", http.StatusForbidden},
		{"customer barrado", http.MethodGet, list, "customer", http.StatusForbidden},
		{"anônimo 401", http.MethodGet, list, "", http.StatusUnauthorized},

		{"almoxarife ajusta", http.MethodPost, adjust, "almoxarife", http.StatusOK},
		{"admin ajusta", http.MethodPost, adjust, "admin", http.StatusOK},
		{"vendas NÃO ajusta (fluxo do almoxarifado)", http.MethodPost, adjust, "vendas", http.StatusForbidden},
		{"contador NÃO ajusta", http.MethodPost, adjust, "contador", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			if got := call(tc.method, tc.path, tc.role); got != tc.want {
				t.Errorf("%s %s como %q → %d, queria %d", tc.method, tc.path, tc.role, got, tc.want)
			}
		})
	}
}
