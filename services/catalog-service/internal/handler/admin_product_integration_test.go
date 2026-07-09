package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/handler"
)

// Testes de integração da ingestão admin — exigem Postgres :5436 com seed
// (mesma infra do product_test.go; SKIPam se o banco não estiver acessível).
//
// Cada teste usa SKUs com o prefixo TEST-INT- e limpa antes/depois, pra não
// poluir o seed nem colidir entre execuções.

const testSKUPrefix = "TEST-INT-"

func cleanupTestProducts(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM products WHERE sku LIKE $1`, testSKUPrefix+"%"); err != nil {
		t.Logf("cleanup: %v", err)
	}
	// slugs criados sem sku (import sem sku) — limpa por slug conhecido.
	_, _ = db.Exec(`DELETE FROM products WHERE slug LIKE 'zzz-teste-integ-%'`)
}

// adminRouterDB monta rotas admin (RequireAdmin em devMode) + públicas contra o DB real.
func adminRouterDB(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	adminH := handler.NewAdminProductHandler(db)
	productH := handler.NewProductHandler(db)

	admin := r.Group("/api/v1/admin", handler.RequireAdmin("", true)) // devMode
	admin.POST("/products", adminH.Create)
	admin.PATCH("/products/by-id/:id", adminH.Patch)
	admin.DELETE("/products/by-id/:id", adminH.Delete)
	admin.POST("/products/by-id/:id/images", adminH.AddImage)
	admin.POST("/products/import", adminH.Import)

	r.GET("/api/v1/products", productH.List)
	r.GET("/api/v1/products/:slug", productH.GetBySlug)
	return r
}

func adminReq(r *gin.Engine, method, path, body string, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if admin {
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("X-User-Id", "admin-test")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// jsonField extrai um campo string do corpo JSON de resposta.
func jsonField(t *testing.T, body []byte, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("resposta não é JSON: %s", body)
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func TestAdmin_RequiresAdminRole(t *testing.T) {
	db := setupTestDB(t)
	r := adminRouterDB(db)

	// Sem header de admin → 401.
	w := adminReq(r, http.MethodPost, "/api/v1/admin/products", `{"name":"x","category":"ferramentas","price":1}`, false)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sem auth: esperado 401, veio %d", w.Code)
	}
}

func TestAdmin_CreatePublishFlow(t *testing.T) {
	db := setupTestDB(t)
	cleanupTestProducts(t, db)
	defer cleanupTestProducts(t, db)
	r := adminRouterDB(db)

	// 1) cria rascunho
	body := `{"name":"Furadeira Integ Teste","category":"ferramentas","price":299.90,"stock":15,"sku":"` + testSKUPrefix + `DRILL","description":"integ"}`
	w := adminReq(r, http.MethodPost, "/api/v1/admin/products", body, true)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: esperado 201, veio %d (%s)", w.Code, w.Body)
	}
	id := jsonField(t, w.Body.Bytes(), "id")
	slug := jsonField(t, w.Body.Bytes(), "slug")
	if id == "" || slug == "" {
		t.Fatal("create não devolveu id/slug")
	}
	if st := jsonField(t, w.Body.Bytes(), "status"); st != "draft" {
		t.Errorf("novo produto deveria nascer draft, veio %q", st)
	}

	// 2) rascunho NÃO aparece na vitrine pública
	w = adminReq(r, http.MethodGet, "/api/v1/products?q=Furadeira+Integ+Teste", "", false)
	if strings.Contains(w.Body.String(), slug) {
		t.Error("rascunho não deveria aparecer na listagem pública")
	}

	// 3) publica
	w = adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"status":"published"}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("publish: esperado 200, veio %d (%s)", w.Code, w.Body)
	}

	// 4) agora aparece
	w = adminReq(r, http.MethodGet, "/api/v1/products?q=Furadeira+Integ+Teste", "", false)
	if !strings.Contains(w.Body.String(), slug) {
		t.Errorf("produto publicado deveria aparecer na vitrine; corpo=%s", w.Body.String()[:min(200, w.Body.Len())])
	}

	// 5) ajusta preço + estoque
	w = adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"price":249.90,"stock":8}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("patch price: %d (%s)", w.Code, w.Body)
	}
	w = adminReq(r, http.MethodGet, "/api/v1/products/"+slug, "", false)
	var p map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p["price"].(float64) != 249.90 {
		t.Errorf("preço esperado 249.90, veio %v", p["price"])
	}
	if int(p["stock"].(float64)) != 8 {
		t.Errorf("estoque esperado 8, veio %v", p["stock"])
	}

	// 6) imagem por URL
	w = adminReq(r, http.MethodPost, "/api/v1/admin/products/by-id/"+id+"/images",
		`{"url":"https://example.com/img.jpg","alt":"teste"}`, true)
	if w.Code != http.StatusCreated {
		t.Errorf("add image: esperado 201, veio %d (%s)", w.Code, w.Body)
	}

	// 7) arquiva → some da vitrine
	w = adminReq(r, http.MethodDelete, "/api/v1/admin/products/by-id/"+id, "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("archive: %d", w.Code)
	}
	w = adminReq(r, http.MethodGet, "/api/v1/products?q=Furadeira+Integ+Teste", "", false)
	if strings.Contains(w.Body.String(), slug) {
		t.Error("produto arquivado não deveria aparecer na vitrine")
	}
}

func TestAdmin_CSVImportUpsert(t *testing.T) {
	db := setupTestDB(t)
	cleanupTestProducts(t, db)
	defer cleanupTestProducts(t, db)
	r := adminRouterDB(db)

	csv := "sku,name,category,price,stock,brand,status\n" +
		testSKUPrefix + "PARF,Parafuso Integ,fixacao,\"R$ 29,90\",200,Ciser,published\n" +
		testSKUPrefix + "CIM,Cimento Integ,construcao,42.50,120,Votoran,published\n"

	// 1ª carga → 2 criados
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products/import", strings.NewReader(csv))
	req.Header.Set("Content-Type", "text/csv")
	req.Header.Set("X-User-Role", "admin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d (%s)", w.Code, w.Body)
	}
	var res struct{ Total, Created, Updated, Failed int }
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 2 || res.Failed != 0 {
		t.Fatalf("1ª carga: esperado created=2 failed=0, veio %+v", res)
	}

	// preço BR "R$ 29,90" deve ter virado 29.90 na vitrine
	w = adminReq(r, http.MethodGet, "/api/v1/products?q=Parafuso+Integ", "", false)
	if !strings.Contains(w.Body.String(), `"price":29.9`) {
		t.Errorf("preço BR não parseado corretamente; corpo=%s", w.Body.String()[:min(300, w.Body.Len())])
	}

	// 2ª carga (mesmo CSV) → 2 atualizados (upsert por SKU)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/products/import", strings.NewReader(csv))
	req.Header.Set("Content-Type", "text/csv")
	req.Header.Set("X-User-Role", "admin")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Updated != 2 || res.Created != 0 {
		t.Fatalf("2ª carga: esperado updated=2 created=0, veio %+v", res)
	}
}
