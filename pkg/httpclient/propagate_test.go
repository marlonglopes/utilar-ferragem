package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/utilar/pkg/httpclient"
	"github.com/utilar/pkg/requestid"
)

// REGRESSÃO: sem isto, um checkout que atravessa payment→order→catalog vira
// três traços desconexos nos logs e não dá pra seguir o pedido ponta a ponta.
func TestPropagacaoDeRequestIDEntreServicos(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get(requestid.HeaderName)
	}))
	defer srv.Close()

	c := httpclient.New(2 * time.Second)
	ctx := requestid.NewContext(context.Background(), "01JABCDEF0000000000000000")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if h := <-got; h != "01JABCDEF0000000000000000" {
		t.Fatalf("X-Request-Id não propagou: %q", h)
	}
}

func TestPropagacaoNaoInventaIDQuandoContextNaoTem(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get(requestid.HeaderName)
	}))
	defer srv.Close()

	resp, err := httpclient.New(2 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if h := <-got; h != "" {
		t.Fatalf("inventou um id (%q) que não existe em log nenhum a montante", h)
	}
}

func TestPropagacaoNaoSobrescreveHeaderExplicito(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get(requestid.HeaderName)
	}))
	defer srv.Close()

	ctx := requestid.NewContext(context.Background(), "DO-CONTEXT")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	req.Header.Set(requestid.HeaderName, "EXPLICITO")
	resp, err := httpclient.New(2 * time.Second).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if h := <-got; h != "EXPLICITO" {
		t.Fatalf("sobrescreveu o header explícito do chamador: %q", h)
	}
}

// Contrato do net/http: RoundTripper não pode mutar a request recebida.
func TestPropagacaoNaoMutaRequestOriginal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	ctx := requestid.NewContext(context.Background(), "ABC")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := httpclient.New(2 * time.Second).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if req.Header.Get(requestid.HeaderName) != "" {
		t.Fatal("o transport mutou a request original — quebra retry e é data race")
	}
}

func TestFromContextOrNewNuncaVazio(t *testing.T) {
	if requestid.FromContextOrNew(context.Background()) == "" {
		t.Fatal("job de background ficaria sem correlação")
	}
	if got := requestid.FromContextOrNew(requestid.NewContext(context.Background(), "X")); got != "X" {
		t.Fatalf("preferiu gerar novo em vez de reusar o do context: %q", got)
	}
}
