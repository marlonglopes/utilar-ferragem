package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Regressão: a tela /admin/produtos abria com "Produto não encontrado" porque
// GET /api/v1/admin/products NÃO EXISTIA — só havia busca por id. A listagem
// pública não serve: ela esconde `cost` e só mostra `published`, e é justamente
// em rascunho que o operador trabalha depois de importar uma planilha.
func TestAdminList_DevolveProdutosComCustoEMargem(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := adminRouterDB(db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?pageSize=5", nil)
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-User-Id", "teste")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Page, PageSize, Total, TotalPages int
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Meta.Total == 0 {
		t.Skip("catálogo vazio")
	}
	if len(resp.Data) == 0 {
		t.Fatal("meta.total > 0 mas data veio vazio")
	}
	if resp.Meta.TotalPages < 1 {
		t.Errorf("totalPages = %d — a paginação da tela quebra", resp.Meta.TotalPages)
	}

	// `cost` PRECISA estar aqui: é a rota de admin, e é o número que evita
	// cadastrar produto no prejuízo. A pública é que não pode tê-lo.
	temCusto := false
	for _, p := range resp.Data {
		if _, ok := p["cost"]; ok {
			temCusto = true
		}
		for _, campo := range []string{"id", "name", "price", "status"} {
			if _, ok := p[campo]; !ok {
				t.Errorf("produto sem campo %q — a tabela da tela não renderiza", campo)
			}
		}
	}
	if !temCusto {
		t.Error("nenhum produto trouxe `cost`: a coluna de margem do admin fica vazia")
	}
}

// O admin precisa enxergar rascunho e arquivado — é neles que ele trabalha
// depois de importar planilha (a importação entra como rascunho de propósito).
// Se esta rota filtrasse por `published` como a pública, o operador importaria
// 400 produtos e veria uma lista vazia.
func TestAdminList_NaoFiltraPorPublicado(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := adminRouterDB(db)

	var comStatus int
	_ = db.QueryRow(`SELECT count(*) FROM products WHERE status <> 'published'`).Scan(&comStatus)
	if comStatus == 0 {
		t.Skip("não há produto fora de 'published' para verificar")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?status=draft&pageSize=5", nil)
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-User-Id", "teste")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	for _, p := range resp.Data {
		if p["status"] != "draft" {
			t.Errorf("filtro status=draft devolveu %v", p["status"])
		}
	}
}

// `sort` e `dir` vêm da URL e NUNCA podem entrar no SQL. Valor desconhecido
// tem que cair no padrão, não virar erro nem injeção.
func TestAdminList_OrdenacaoEhWhitelist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := adminRouterDB(db)

	for _, sort := range []string{
		"name", "price", "cost", "stock", "margin", "status",
		"desconhecido",
		"p.price; DROP TABLE products--",
		"(SELECT 1)",
	} {
		// Codificar na query: o valor malicioso tem espaço, e concatenar cru
		// quebraria o httptest.NewRequest antes de chegar no handler — o teste
		// falharia sem exercitar nada.
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)
		vals := req.URL.Query()
		vals.Set("pageSize", "2")
		vals.Set("sort", sort)
		req.URL.RawQuery = vals.Encode()
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("X-User-Id", "teste")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("sort=%q devolveu %d — valor não reconhecido deve cair no padrão, não falhar",
				sort, w.Code)
		}
	}

	// A tabela continua existindo depois do sort malicioso.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM products`).Scan(&n); err != nil {
		t.Fatalf("tabela products não sobreviveu ao teste de injeção: %v", err)
	}
}

// Custo é o dado mais sensível do negócio: quem o vê sabe até onde a loja pode
// baixar o preço. A rota tem que recusar quem não é admin.
func TestAdminList_RecusaQuemNaoEhAdmin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := adminRouterDB(db)

	casos := []struct {
		nome, papel string
		querStatus  []int
	}{
		{"anônimo", "", []int{http.StatusUnauthorized}},
		{"customer", "customer", []int{http.StatusUnauthorized, http.StatusForbidden}},
		{"seller", "seller", []int{http.StatusUnauthorized, http.StatusForbidden}},
		{"store_operator", "store_operator", []int{http.StatusUnauthorized, http.StatusForbidden}},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)
			if tc.papel != "" {
				req.Header.Set("X-User-Role", tc.papel)
				req.Header.Set("X-User-Id", "u")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			for _, ok := range tc.querStatus {
				if w.Code == ok {
					return
				}
			}
			t.Fatalf("%s recebeu %d — custo e margem vazariam para quem não é admin",
				tc.nome, w.Code)
		})
	}
}

// Termo de busca hostil não pode derrubar a rota nem forçar CPU no pg_trgm
// (audit CT1-C1: `%_%_%_%_%` sem escape leva o servidor a 100%).
func TestAdminList_BuscaHostilNaoQuebra(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := adminRouterDB(db)

	for _, q := range []string{
		"%_%_%_%_%_%_%", "'; DROP TABLE products; --", "\\", "%", "_",
		"<script>", "\x00",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?pageSize=2", nil)
		vals := req.URL.Query()
		vals.Set("q", q)
		req.URL.RawQuery = vals.Encode()
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("X-User-Id", "teste")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("q=%q devolveu %d", q, w.Code)
		}
	}
}
