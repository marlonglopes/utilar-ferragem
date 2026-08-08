package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
)

// CRUD admin de cupons, ponta a ponta contra o banco: cria, lista, desativa,
// valida (percentual > 100 e código duplicado) e apaga.
func TestCouponAdmin_CRUDeValidacao(t *testing.T) {
	db := dashDB(t)
	defer db.Close()
	defer db.Exec("DELETE FROM coupons WHERE code IN ('ADMIN10','DUP1','BADPCT')")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewCouponHandler(db)
	r.GET("/api/v1/admin/coupons", h.List)
	r.POST("/api/v1/admin/coupons", h.Create)
	r.PATCH("/api/v1/admin/coupons/:id", h.Update)
	r.DELETE("/api/v1/admin/coupons/:id", h.Delete)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// Cria (código em minúsculas → normalizado p/ maiúsculas).
	w := do(http.MethodPost, "/api/v1/admin/coupons",
		`{"code":"admin10","type":"percent","value":10,"minSubtotal":100}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Code != "ADMIN10" {
		t.Fatalf("código não normalizado: %q", created.Code)
	}

	// Percentual > 100 → 400.
	if w := do(http.MethodPost, "/api/v1/admin/coupons",
		`{"code":"BADPCT","type":"percent","value":150}`); w.Code != http.StatusBadRequest {
		t.Fatalf("percentual>100: %d (esperado 400)", w.Code)
	}

	// Código duplicado → 409.
	do(http.MethodPost, "/api/v1/admin/coupons", `{"code":"DUP1","type":"fixed","value":10}`)
	if w := do(http.MethodPost, "/api/v1/admin/coupons",
		`{"code":"dup1","type":"fixed","value":20}`); w.Code != http.StatusConflict {
		t.Fatalf("duplicado: %d (esperado 409)", w.Code)
	}

	// Lista contém o criado.
	w = do(http.MethodGet, "/api/v1/admin/coupons", "")
	if !strings.Contains(w.Body.String(), "ADMIN10") {
		t.Fatalf("lista não traz ADMIN10: %s", w.Body.String())
	}

	// Desativa via PATCH.
	if w := do(http.MethodPatch, "/api/v1/admin/coupons/"+created.ID, `{"active":false}`); w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var active bool
	db.QueryRow("SELECT active FROM coupons WHERE id=$1", created.ID).Scan(&active)
	if active {
		t.Fatal("cupom deveria ter sido desativado")
	}

	// Apaga.
	if w := do(http.MethodDelete, "/api/v1/admin/coupons/"+created.ID, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	// PATCH em inexistente → 404.
	if w := do(http.MethodPatch, "/api/v1/admin/coupons/"+created.ID, `{"active":true}`); w.Code != http.StatusNotFound {
		t.Fatalf("update inexistente: %d (esperado 404)", w.Code)
	}
}
