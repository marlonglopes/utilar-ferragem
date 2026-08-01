package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// O uploader em lote casa a foto ao produto pelo SKU do nome do arquivo. Este
// teste prova o resolvedor: casa SKUs reais, diz se já tem imagem, ignora SKU
// inexistente, e NUNCA devolve custo.
func TestResolveBySKU_CasaProdutosSemVazarCusto(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/products/by-sku", handler.NewCatalogAdminHandler(db).ResolveBySKU)

	// Pega 2 SKUs reais do banco.
	rows, err := db.Query(`SELECT sku FROM products WHERE sku ~ '^[0-9]+$' AND sku IS NOT NULL LIMIT 2`)
	if err != nil {
		t.Skipf("sem banco: %v", err)
	}
	var skus []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		skus = append(skus, s)
	}
	_ = rows.Close()
	if len(skus) < 2 {
		t.Skip("banco sem SKUs numéricos suficientes")
	}

	get := func(q string) (int, string, []map[string]any) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/by-sku?skus="+q, nil))
		var resp struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, w.Body.String(), resp.Data
	}

	// Casa os dois SKUs reais + um inexistente.
	code, body, data := get(skus[0] + "," + skus[1] + ",SKU-QUE-NAO-EXISTE-999")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	if len(data) != 2 {
		t.Fatalf("esperava 2 casados (o inexistente não casa), veio %d", len(data))
	}
	for _, m := range data {
		for _, f := range []string{"sku", "id", "name", "hasImage"} {
			if _, ok := m[f]; !ok {
				t.Errorf("match sem campo %q: %v", f, m)
			}
		}
	}
	// Nunca custo.
	if strings.Contains(strings.ToLower(body), "cost") {
		t.Errorf("REGRESSÃO: by-sku vazou custo: %s", body)
	}

	// SKUs vazio → data vazio, sem erro.
	code, _, data = get("")
	if code != http.StatusOK || len(data) != 0 {
		t.Errorf("skus vazio deveria devolver 200 e lista vazia, veio %d/%d", code, len(data))
	}
}
