package alice_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/utilar/assistant-service/internal/alice"
	"github.com/utilar/assistant-service/internal/catalog"
	"github.com/utilar/assistant-service/internal/knowledge"
	"github.com/utilar/assistant-service/internal/llm"
)

// mustKB carrega a base de conhecimento real — se ela não validar, nenhum teste
// da Alice faz sentido.
func mustKB(t *testing.T) *knowledge.KB {
	t.Helper()
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatalf("base de conhecimento não carregou: %v", err)
	}
	return kb
}

// catálogo fake que devolve produtos pra busca.
func fakeCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/products") && r.URL.Query().Get("q") != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "1", "slug": "furadeira-bosch", "name": "Furadeira Bosch GSB 13", "price": 299.9, "stock": 15, "category": "ferramentas", "rating": 4.6},
					{"id": "2", "slug": "furadeira-makita", "name": "Furadeira Makita", "price": 459.0, "stock": 4, "category": "ferramentas", "rating": 4.8},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
}

func TestChat_MockUsesToolAndSurfacesProducts(t *testing.T) {
	srv := fakeCatalog(t)
	defer srv.Close()

	eng := alice.New(llm.NewMock(), catalog.New(srv.URL), mustKB(t), alice.Opts{})
	res, err := eng.Chat(context.Background(), alice.ModeCliente, nil, "procuro uma furadeira")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(res.Products) != 2 {
		t.Fatalf("esperava 2 produtos citados, veio %d", len(res.Products))
	}
	if res.Products[0].Slug != "furadeira-bosch" {
		t.Errorf("produto inesperado: %+v", res.Products[0])
	}
	if res.Reply == "" {
		t.Error("Alice deveria responder um texto")
	}
	if res.Model != "mock" {
		t.Errorf("model esperado mock, veio %q", res.Model)
	}
}

func TestChat_GreetingNoTool(t *testing.T) {
	srv := fakeCatalog(t)
	defer srv.Close()

	eng := alice.New(llm.NewMock(), catalog.New(srv.URL), mustKB(t), alice.Opts{})
	res, err := eng.Chat(context.Background(), alice.ModeCliente, nil, "oi, quem é você?")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(res.Products) != 0 {
		t.Errorf("saudação não deveria buscar produtos, veio %d", len(res.Products))
	}
	if !strings.Contains(res.Reply, "Alice") {
		t.Errorf("saudação deveria se apresentar como Alice; veio %q", res.Reply)
	}
}

func TestChat_NoResults(t *testing.T) {
	// catálogo que sempre devolve vazio
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	eng := alice.New(llm.NewMock(), catalog.New(srv.URL), mustKB(t), alice.Opts{})
	res, err := eng.Chat(context.Background(), alice.ModeCliente, nil, "procuro um item-que-nao-existe-zzz")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if len(res.Products) != 0 {
		t.Errorf("esperava 0 produtos, veio %d", len(res.Products))
	}
	if !strings.Contains(strings.ToLower(res.Reply), "não encontrei") {
		t.Errorf("esperava resposta honesta de 'não encontrei'; veio %q", res.Reply)
	}
}
