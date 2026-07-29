package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Regressão: "a seção 'Outros produtos de X' não mostrava foto".
//
// O GET /products/:slug/related montava cada produto SEM o campo `images` — só a
// vitrine (List) trazia a capa, via loadThumbnails. Resultado: o ProductCard na
// seção de relacionados caía no ícone da categoria MESMO com o produto tendo
// foto. loadCovers passou a preencher a capa dos relacionados, como na vitrine.
func TestRegression_RelatedTrazCapaDoProduto(t *testing.T) {
	db := reviewDB(t)
	r := recoRouter(db)

	// Origem: um produto publicado cuja categoria tem ≥3 publicados COM imagem —
	// assim o related terá itens que DEVEM vir com capa.
	var srcSlug string
	err := db.QueryRow(`
		SELECT p.slug FROM products p
		WHERE p.status = 'published'
		  AND EXISTS (SELECT 1 FROM product_images i WHERE i.product_id = p.id)
		  AND (SELECT count(*) FROM products q
		         JOIN product_images qi ON qi.product_id = q.id
		        WHERE q.category_id = p.category_id AND q.status = 'published') >= 3
		LIMIT 1`).Scan(&srcSlug)
	if err == sql.ErrNoRows {
		t.Skip("sem categoria com produtos publicados e imagens suficientes")
	}
	if err != nil {
		t.Fatalf("buscar origem: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/"+srcSlug+"/related?limit=8", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Name   string `json:"name"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v — body=%s", err, w.Body.String())
	}
	if len(resp.Data) == 0 {
		t.Skip("related vazio para o produto escolhido")
	}

	comCapa := 0
	for _, p := range resp.Data {
		if len(p.Images) > 0 && p.Images[0].URL != "" {
			comCapa++
		}
	}
	if comCapa == 0 {
		t.Errorf("REGRESSÃO: /related não trouxe capa em NENHUM dos %d relacionados "+
			"(a seção 'Outros produtos' cai no ícone mesmo com o produto tendo foto)", len(resp.Data))
	}
}
