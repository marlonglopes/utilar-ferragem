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
