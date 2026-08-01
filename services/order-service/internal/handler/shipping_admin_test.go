package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/order-service/internal/shipping"
)

// CRUD da tabela de frete — o que permite ao dono corrigir as faixas (o seed
// veio com CEP de SP; a loja é no RS). Ponta a ponta contra o banco: cria,
// lista, atualiza, valida faixa invertida (400) e apaga.
func TestShippingAdmin_CRUDeValidacao(t *testing.T) {
	db := dashDB(t) // skipa sem banco
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewShippingAdminHandler(db, shipping.NewStore(db))
	r.GET("/api/v1/admin/shipping-rates", h.List)
	r.POST("/api/v1/admin/shipping-rates", h.Create)
	r.PATCH("/api/v1/admin/shipping-rates/:id", h.Update)
	r.DELETE("/api/v1/admin/shipping-rates/:id", h.Delete)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// Faixa do RS (90000000–99999999) — a correta para a loja.
	rs := `{"zoneName":"Fronteira Oeste RS","cepStart":97000000,"cepEnd":97999999,
	        "serviceCode":"standard","serviceName":"Entrega padrão","baseCost":18.5,
	        "costPerItem":1.2,"deliveryDays":3,"freeAbove":250,"active":true}`
	w := do(http.MethodPost, "/api/v1/admin/shipping-rates", rs)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("create não devolveu id")
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM shipping_rates WHERE id=$1`, created.ID) })

	// Aparece na listagem (que inclui inativas também).
	w = do(http.MethodGet, "/api/v1/admin/shipping-rates", "")
	if !strings.Contains(w.Body.String(), created.ID) {
		t.Errorf("a faixa criada não apareceu na listagem")
	}

	// Atualiza o custo.
	upd := `{"zoneName":"Fronteira Oeste RS","cepStart":97000000,"cepEnd":97999999,
	         "serviceCode":"standard","serviceName":"Entrega padrão","baseCost":22.0,
	         "costPerItem":1.2,"deliveryDays":4,"freeAbove":250,"active":true}`
	if w := do(http.MethodPatch, "/api/v1/admin/shipping-rates/"+created.ID, upd); w.Code != http.StatusOK {
		t.Errorf("update: %d %s", w.Code, w.Body.String())
	}

	// Faixa invertida (cepEnd < cepStart) → 400, não 500 do CHECK.
	bad := `{"zoneName":"X","cepStart":97999999,"cepEnd":97000000,"serviceCode":"standard",
	         "serviceName":"P","baseCost":10,"deliveryDays":2}`
	if w := do(http.MethodPost, "/api/v1/admin/shipping-rates", bad); w.Code != http.StatusBadRequest {
		t.Errorf("faixa invertida: esperava 400, veio %d", w.Code)
	}

	// deliveryDays zero → 400.
	badDays := `{"zoneName":"X","cepStart":97000000,"cepEnd":97999999,"serviceCode":"standard",
	            "serviceName":"P","baseCost":10,"deliveryDays":0}`
	if w := do(http.MethodPost, "/api/v1/admin/shipping-rates", badDays); w.Code != http.StatusBadRequest {
		t.Errorf("deliveryDays zero: esperava 400, veio %d", w.Code)
	}

	// Apaga.
	if w := do(http.MethodDelete, "/api/v1/admin/shipping-rates/"+created.ID, ""); w.Code != http.StatusOK {
		t.Errorf("delete: %d %s", w.Code, w.Body.String())
	}
}
