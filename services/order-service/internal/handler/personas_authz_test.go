package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
)

// Fronteira de persona no servidor (backoffice 2026-07). Monta os MESMOS grupos
// de main.go — usando os mesmos conjuntos handler.Ops*Roles, para o teste e o
// servidor não poderem divergir — e prova quem entra e quem toma 403 em cada
// tipo de rota. É o "fail-closed no servidor" do requisito: menu escondido não
// protege nada; isto sim.
func TestPersonas_OpsAuthzFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	// devMode=false + token REAL assinado (dashToken/dashSecret): é a barreira
	// criptográfica de produção que interessa. Em devMode o header X-User-Role
	// daria 401 para papel não-permitido; queremos provar o 403 real.
	read := r.Group("/api/v1/admin", handler.RequireRole(dashSecret, false, handler.OpsReadRoles...))
	read.GET("/orders", ok)
	write := r.Group("/api/v1/admin", handler.RequireRole(dashSecret, false, handler.OpsWriteRoles...))
	write.PATCH("/orders/:id/picking", ok)
	refund := r.Group("/api/v1/admin", handler.RequireRole(dashSecret, false, handler.OpsRefundRoles...))
	refund.PATCH("/returns/:rid/refund", ok)

	call := func(method, path, role string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+dashToken(t, role))
		}
		r.ServeHTTP(w, req)
		return w.Code
	}

	const (
		orders  = "/api/v1/admin/orders"
		picking = "/api/v1/admin/orders/o1/picking"
		rfnd    = "/api/v1/admin/returns/r1/refund"
	)

	cases := []struct {
		nome, method, path, role string
		want                     int
	}{
		// Leitura: contador vê (faturamento); todos que operam veem.
		{"contador lê pedidos", http.MethodGet, orders, "contador", http.StatusOK},
		{"vendas lê pedidos", http.MethodGet, orders, "vendas", http.StatusOK},
		{"almoxarife lê pedidos", http.MethodGet, orders, "almoxarife", http.StatusOK},

		// Agir: vendas/almoxarife sim; contador NÃO (read-only fora do contábil).
		{"vendas separa", http.MethodPatch, picking, "vendas", http.StatusOK},
		{"almoxarife separa", http.MethodPatch, picking, "almoxarife", http.StatusOK},
		{"contador NÃO separa", http.MethodPatch, picking, "contador", http.StatusForbidden},

		// Reembolso (dinheiro saindo): só admin.
		{"admin reembolsa", http.MethodPatch, rfnd, "admin", http.StatusOK},
		{"vendas NÃO reembolsa", http.MethodPatch, rfnd, "vendas", http.StatusForbidden},
		{"almoxarife NÃO reembolsa", http.MethodPatch, rfnd, "almoxarife", http.StatusForbidden},
		{"contador NÃO reembolsa", http.MethodPatch, rfnd, "contador", http.StatusForbidden},

		// Quem não é staff nunca entra; anônimo é 401, não 403.
		{"customer barrado", http.MethodGet, orders, "customer", http.StatusForbidden},
		{"seller barrado", http.MethodGet, orders, "seller", http.StatusForbidden},
		{"anônimo 401", http.MethodGet, orders, "", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			if got := call(tc.method, tc.path, tc.role); got != tc.want {
				t.Errorf("%s %s como %q → %d, queria %d", tc.method, tc.path, tc.role, got, tc.want)
			}
		})
	}
}
