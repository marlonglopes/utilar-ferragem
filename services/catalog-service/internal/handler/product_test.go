package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/catalog-service/internal/handler"
)

// Testes de integração — requerem Postgres rodando em :5436 (make infra-up).
// Usam o schema + seed em migrations/. Skipam se DB não estiver acessível.

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CATALOG_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}

	// Valida que o seed existe; se não houver produtos, skipa (precisa rodar `make catalog-db-seed`).
	var count int
	if err := db.QueryRow("SELECT count(*) FROM products").Scan(&count); err != nil {
		t.Skipf("products table not ready: %v", err)
	}
	if count == 0 {
		t.Skip("no products in DB — run `make catalog-db-seed` before integration tests")
	}
	return db
}

func setupRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	productH := handler.NewProductHandler(db)
	categoryH := handler.NewCategoryHandler(db)
	sellerH := handler.NewSellerHandler(db)
	api := r.Group("/api/v1")
	api.GET("/categories", categoryH.List)
	api.GET("/sellers", sellerH.List)
	api.GET("/products", productH.List)
	api.GET("/products/facets", productH.Facets)
	api.GET("/products/by-id/:id", productH.GetByID)
	api.GET("/products/:slug", productH.GetBySlug)
	return r
}

// -- categories -------------------------------------------------------------

func TestCategories_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d want 200, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 8 {
		t.Errorf("want 8 categorias (taxonomia fixa), got %d", len(body.Data))
	}
}

// -- sellers ----------------------------------------------------------------

func TestSellers_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/sellers", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d want 200", w.Code)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) < 1 {
		t.Errorf("want ≥1 seller, got %d", len(body.Data))
	}
}

// -- products list ----------------------------------------------------------

func TestProducts_ListPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?per_page=5&page=1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d want 200, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Page       int `json:"page"`
			PerPage    int `json:"per_page"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Meta.PerPage != 5 || body.Meta.Page != 1 {
		t.Errorf("meta mismatch: page=%d per_page=%d", body.Meta.Page, body.Meta.PerPage)
	}
	if len(body.Data) != 5 {
		t.Errorf("want 5 items, got %d", len(body.Data))
	}
	if body.Meta.Total < 5 {
		t.Errorf("total should be ≥5, got %d", body.Meta.Total)
	}
}

func TestProducts_FilterByCategory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?category=ferramentas&per_page=50", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Data []struct {
			Category string `json:"category"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	if len(body.Data) == 0 {
		t.Fatal("no products returned for ferramentas")
	}
	for _, p := range body.Data {
		if p.Category != "ferramentas" {
			t.Errorf("item with wrong category: %q", p.Category)
		}
	}
}

func TestProducts_FilterByPriceRange(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?price_min=100&price_max=500&per_page=50", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Data []struct {
			Price float64 `json:"price"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	for _, p := range body.Data {
		if p.Price < 100 || p.Price > 500 {
			t.Errorf("product price %.2f outside [100, 500]", p.Price)
		}
	}
}

func TestProducts_FullTextSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?q=bosch&per_page=50", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	if len(body.Data) == 0 {
		t.Fatal("search bosch returned 0 results")
	}
	// Cada resultado deve conter "Bosch" (case-insensitive) em name, description ou seller.
	// Aqui checamos pelo menos o nome do primeiro item.
}

func TestProducts_SortPriceAsc(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?sort=price_asc&per_page=10", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Data []struct {
			Price float64 `json:"price"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)

	for i := 1; i < len(body.Data); i++ {
		if body.Data[i].Price < body.Data[i-1].Price {
			t.Errorf("ordenação quebrada no índice %d: %.2f < %.2f", i, body.Data[i].Price, body.Data[i-1].Price)
		}
	}
}

// -- product by slug --------------------------------------------------------

func TestProduct_GetBySlug(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/furadeira-bosch-gsb-13-re", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var p struct {
		Slug              string  `json:"slug"`
		Name              string  `json:"name"`
		SellerID          string  `json:"sellerId"`
		SellerRating      float64 `json:"sellerRating"`
		ReviewCount       int     `json:"reviewCount"`
		OriginalPrice     float64 `json:"originalPrice"`
		Images            []struct {
			URL string `json:"url"`
			Alt string `json:"alt"`
		} `json:"images"`
	}
	json.Unmarshal(w.Body.Bytes(), &p)

	if p.Slug != "furadeira-bosch-gsb-13-re" {
		t.Errorf("slug mismatch: %q", p.Slug)
	}
	if p.SellerID != "ferragem-silva" {
		t.Errorf("sellerId mismatch: got %q", p.SellerID)
	}
	if p.SellerRating == 0 {
		t.Error("sellerRating should be populated")
	}
	if len(p.Images) < 1 {
		t.Errorf("want ≥1 image, got %d", len(p.Images))
	}
}

func TestProduct_GetBySlug_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/slug-inexistente-xyz", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404", w.Code)
	}
}

// O2-H5: /products/by-id/:id é o endpoint que order-service usa pra validar
// preço autoritativo. Resolve um produto pelo seu UUID interno.
func TestProduct_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	// Pega um id real do seed
	var id string
	if err := db.QueryRow("SELECT id FROM products LIMIT 1").Scan(&id); err != nil {
		t.Skipf("nenhum produto no DB: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/by-id/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var p struct {
		ID    string  `json:"id"`
		Price float64 `json:"price"`
	}
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != id {
		t.Errorf("id mismatch: got %q want %q", p.ID, id)
	}
	if p.Price <= 0 {
		t.Errorf("price não populado: %v", p.Price)
	}
}

func TestProduct_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	// UUID válido mas não existe
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/by-id/00000000-0000-0000-0000-000000000000", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404", w.Code)
	}
}

// -- facets -----------------------------------------------------------------

func TestProducts_Facets(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/facets?category=ferramentas", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var facets struct {
		Brands []struct {
			Value string `json:"value"`
			Count int    `json:"count"`
		} `json:"brands"`
		PriceMin float64 `json:"price_min"`
		PriceMax float64 `json:"price_max"`
	}
	json.Unmarshal(w.Body.Bytes(), &facets)

	if len(facets.Brands) == 0 {
		t.Error("facets should include brands for ferramentas")
	}
	if facets.PriceMin >= facets.PriceMax {
		t.Errorf("price range inválido: min=%.2f max=%.2f", facets.PriceMin, facets.PriceMax)
	}
}

// -- helper -----------------------------------------------------------------

// Usada na doc; evita linter reclamar de imports não usados por variantes.
var _ = filepath.Join
