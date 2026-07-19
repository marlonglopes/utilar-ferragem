package catalogclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/pkg/circuitbreaker"
)

// ============================================================================
// Resiliência order → catalog
// ----------------------------------------------------------------------------
// O que estes testes travam (auditoria arquitetural 2026-07-18): uma lentidão
// do catálogo NÃO pode derrubar o checkout, e a degradação NÃO pode virar
// "confio no preço que o cliente mandou".
// ============================================================================

// resilientClient monta um cliente apontado pro stub, com disjuntor.
func resilientClient(t *testing.T, h http.HandlerFunc) (*catalogclient.Client, *catalogclient.Resilience, *int32) {
	t.Helper()
	var rejected int32
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	res := catalogclient.NewResilience(nil, func(string) { atomic.AddInt32(&rejected, 1) })
	c := catalogclient.NewWithSecret(srv.URL, "segredo-de-servico-de-teste").WithResilience(res)
	return c, res, &rejected
}

// TestDisjuntorAbreEDeixaDeIrAoCatalogo — o comportamento que evita o
// empilhamento: depois de N falhas, o order-service para de esperar timeout.
func TestDisjuntorAbreEDeixaDeIrAoCatalogo(t *testing.T) {
	var hits int32
	c, res, rejected := resilientClient(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	})

	// O limiar é 5 falhas; com retry de 2 tentativas, 3 chamadas já bastam.
	for i := 0; i < 5; i++ {
		_, _ = c.GetByID(context.Background(), "11111111-1111-1111-1111-111111111111")
	}
	if res.State() != circuitbreaker.StateOpen {
		t.Fatalf("estado = %v, esperado aberto após rajada de 502", res.State())
	}

	before := atomic.LoadInt32(&hits)
	_, err := c.GetByID(context.Background(), "11111111-1111-1111-1111-111111111111")

	if !errors.Is(err, catalogclient.ErrUnavailable) {
		t.Fatalf("err = %v, esperado ErrUnavailable (disjuntor aberto)", err)
	}
	if got := atomic.LoadInt32(&hits); got != before {
		t.Fatalf("o catálogo recebeu %d chamadas a mais com o disjuntor aberto", got-before)
	}
	if atomic.LoadInt32(rejected) == 0 {
		t.Fatal("a métrica de recusa não foi contabilizada — o painel não mostraria o estrago")
	}
}

// TestRetryAbsorveSoluco — um 502 isolado seguido de 200 tem que virar sucesso
// para o cliente. É este o caso que hoje derruba um checkout inteiro.
func TestRetryAbsorveSoluco(t *testing.T) {
	var n int32
	c, _, _ := resilientClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1","name":"Furadeira","price":249.9,"stock":10}`))
	})

	p, err := c.GetByID(context.Background(), "p1")
	if err != nil {
		t.Fatalf("err = %v — o retry não absorveu o 502 isolado", err)
	}
	if p.Price != 249.9 {
		t.Fatalf("price = %v", p.Price)
	}
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Fatalf("chamadas = %d, esperado 2 (original + 1 retry)", got)
	}
}

// TestRegression_ProdutoInexistenteNaoAbreDisjuntor.
//
// Modo de falha que previne: 404 é resposta CORRETA do catálogo. Se contasse
// como falha de infraestrutura, um único carrinho com produto arquivado abriria
// o disjuntor e derrubaria o checkout de TODO MUNDO — indisponibilidade
// auto-infligida a partir de comportamento normal.
func TestRegression_ProdutoInexistenteNaoAbreDisjuntor(t *testing.T) {
	var hits int32
	c, res, _ := resilientClient(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	})

	for i := 0; i < 20; i++ {
		_, err := c.GetByID(context.Background(), "sumiu")
		if !errors.Is(err, catalogclient.ErrNotFound) {
			t.Fatalf("err = %v, esperado ErrNotFound", err)
		}
	}
	if res.State() != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v — 404 abriu o disjuntor", res.State())
	}
	// E 404 também não é retentado: repetir devolveria o mesmo 404 mais tarde.
	if got := atomic.LoadInt32(&hits); got != 20 {
		t.Fatalf("chamadas = %d, esperado 20 (uma por GetByID, sem retry em 404)", got)
	}
}

// TestDisjuntorFechaQuandoOCatalogoVolta — sem este caminho o checkout ficaria
// recusando para sempre depois de uma piscada de 3 segundos.
func TestDisjuntorFechaQuandoOCatalogoVolta(t *testing.T) {
	var fora atomic.Bool
	fora.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fora.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"p1","name":"Furadeira","price":10,"stock":1}`))
	}))
	t.Cleanup(srv.Close)

	// Cooldown zero (via config própria) para não depender de relógio real: o
	// teste do cooldown em si vive em pkg/circuitbreaker com relógio injetado.
	// Aqui interessa só que a recuperação acontece de ponta a ponta.
	res := catalogclient.NewResilienceForTest(circuitbreaker.Config{
		FailureThreshold: 3,
		Cooldown:         1, // 1ns: já vencido na próxima chamada
	})
	c := catalogclient.New(srv.URL).WithResilience(res)

	for i := 0; i < 4; i++ {
		_, _ = c.GetByID(context.Background(), "p1")
	}

	fora.Store(false)
	if _, err := c.GetByID(context.Background(), "p1"); err != nil {
		t.Fatalf("após o catálogo voltar, err = %v", err)
	}
	if res.State() != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v, esperado fechado depois da prova bem-sucedida", res.State())
	}
}

// TestSemResilienciaComportamentoOriginal — WithResilience é opt-in; um cliente
// montado como antes não pode mudar de comportamento por acidente.
func TestSemResilienciaComportamentoOriginal(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	c := catalogclient.New(srv.URL)
	for i := 0; i < 10; i++ {
		_, err := c.GetByID(context.Background(), "p1")
		if !errors.Is(err, catalogclient.ErrUpstream) {
			t.Fatalf("err = %v, esperado ErrUpstream", err)
		}
	}
	if got := atomic.LoadInt32(&n); got != 10 {
		t.Fatalf("chamadas = %d, esperado 10 (sem retry, sem disjuntor)", got)
	}
}
