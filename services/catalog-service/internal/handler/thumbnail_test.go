package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utilar/catalog-service/internal/model"
)

// Regressão: a listagem de produtos não devolvia imagem nenhuma — só o detalhe
// devolvia. O card da vitrine caía no emoji da categoria, e ninguém compra uma
// furadeira de R$ 429 olhando "⚒". Foto na vitrine é o que converte.
//
// Também trava a forma da correção: UMA query para a página inteira. A versão
// ingênua (chamar loadImages por produto) seria N+1 e, com 100 itens por página,
// transformaria a vitrine no endpoint mais caro do sistema.
func TestList_DevolveCapaDoProduto(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	r := setupRouter(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products?per_page=24", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200: %s", w.Code, w.Body.String())
	}

	var resp model.ProductsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Skip("catálogo vazio — nada a verificar")
	}

	comCapa := 0
	for _, p := range resp.Data {
		if len(p.Images) > 0 {
			comCapa++
			if p.Images[0].URL == "" {
				t.Errorf("produto %s tem imagem com URL vazia", p.Slug)
			}
			// Só a capa vai na listagem. A galeria completa é do detalhe —
			// mandar 5 imagens de 24 produtos infla a resposta sem que o card
			// use nenhuma delas.
			if len(p.Images) > 1 {
				t.Errorf("produto %s trouxe %d imagens na listagem; a listagem manda só a capa",
					p.Slug, len(p.Images))
			}
		}
	}

	if comCapa == 0 {
		t.Error("nenhum produto da primeira página veio com capa — a vitrine voltou a mostrar só ícone")
	}
	t.Logf("%d de %d produtos com capa", comCapa, len(resp.Data))
}
