package orders_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utilar/assistant-service/internal/orders"
)

// Sem URL configurada, a co-compra fica DESLIGADA em vez de falhar a cada
// pergunta. A Alice então não oferece sugestão por co-compra — e, sobretudo,
// não inventa uma.
func TestDisponivel_DesligadoSemURL(t *testing.T) {
	if orders.New("", "").Disponivel() {
		t.Error("sem URL a co-compra deveria estar desligada")
	}
	if !orders.New("http://x", "").Disponivel() {
		t.Error("com URL deveria estar disponível")
	}

	// Desligado, devolve vazio SEM erro: é ausência de dado, não falha.
	pares, err := orders.New("", "").CoCompras(context.Background(), "cimento", 5)
	if err != nil {
		t.Errorf("cliente desligado não deveria dar erro, veio %v", err)
	}
	if len(pares) != 0 {
		t.Error("cliente desligado não pode devolver pares")
	}
}

// LGPD — a garantia central deste package.
//
// O piso de k-anonimato é REAPLICADO localmente, mesmo já sendo pedido ao
// order-service. Confiar num filtro remoto para proteger dado pessoal é confiar
// demais: basta uma regressão do outro lado para a Alice passar a expor um par
// que só existe porque UMA pessoa comprou aquela combinação — o que é o
// histórico de compra dela, não estatística.
func TestCoCompras_ReaplicaPisoDeKAnonimato(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Servidor "regredido": ignora o parâmetro min e devolve pares raros.
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"slug": "comum", "nome": "Item comum", "ocorrencias": 40},
			{"slug": "raro", "nome": "Item raro", "ocorrencias": 1},
			{"slug": "no-limite", "nome": "No limite", "ocorrencias": orders.MinOcorrenciasPadrao},
			{"slug": "abaixo", "nome": "Abaixo do piso", "ocorrencias": orders.MinOcorrenciasPadrao - 1},
			{"slug": "", "nome": "Sem slug", "ocorrencias": 99},
		}})
	}))
	defer srv.Close()

	pares, err := orders.New(srv.URL, "tok").CoCompras(context.Background(), "cimento", 5)
	if err != nil {
		t.Fatal(err)
	}

	permitidos := map[string]bool{"comum": true, "no-limite": true}
	for _, p := range pares {
		if !permitidos[p.Slug] {
			t.Errorf("VAZAMENTO LGPD: par %q com %d ocorrências passou o piso de %d",
				p.Slug, p.Ocorrencias, orders.MinOcorrenciasPadrao)
		}
	}
	if len(pares) != 2 {
		t.Errorf("esperava 2 pares acima do piso, veio %d: %+v", len(pares), pares)
	}
}

// O piso e o slug pedido têm que chegar ao order-service, para a agregação
// acontecer NO BANCO. Trazer itens de pedido crus para cá seria justamente o
// que a regra de LGPD proíbe.
func TestCoCompras_EnviaPisoESlugAoServidor(t *testing.T) {
	var gotMin, gotSlug, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMin = r.URL.Query().Get("min")
		gotSlug = r.URL.Query().Get("slug")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	if _, err := orders.New(srv.URL, "tok-servico").CoCompras(context.Background(), "cimento-cp2", 5); err != nil {
		t.Fatal(err)
	}
	if gotSlug != "cimento-cp2" {
		t.Errorf("slug enviado = %q", gotSlug)
	}
	if gotMin == "" || gotMin == "0" {
		t.Errorf("o piso de k-anonimato tem que ser enviado, veio %q", gotMin)
	}
	if gotAuth != "Bearer tok-servico" {
		t.Errorf("endpoint interno exige credencial de serviço, veio %q", gotAuth)
	}
}

func TestCoCompras_EntradaInvalidaEErroDoServidor(t *testing.T) {
	if _, err := orders.New("http://x", "").CoCompras(context.Background(), "", 5); err == nil {
		t.Error("slug vazio deveria dar erro")
	}

	ruim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ruim.Close()
	if _, err := orders.New(ruim.URL, "").CoCompras(context.Background(), "cimento", 5); err == nil {
		t.Error("erro do order-service deveria propagar — a Alice precisa saber que não tem o dado")
	}
}
