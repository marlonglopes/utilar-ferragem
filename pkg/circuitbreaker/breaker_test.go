package circuitbreaker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/utilar/pkg/circuitbreaker"
)

// clock é um relógio manual. Sem ele, testar cooldown exigiria time.Sleep de
// verdade e o teste passaria a demorar segundos — teste lento é teste que
// alguém desativa, e este aqui protege o checkout.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Unix(1_700_000_000, 0)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

var errBoom = errors.New("serviço remoto fora")

func fail() error { return errBoom }
func ok() error   { return nil }

// TestDisjuntorAbreDepoisDoLimiar — o comportamento central: N falhas
// consecutivas param de ir à rede.
func TestDisjuntorAbreDepoisDoLimiar(t *testing.T) {
	clk := newClock()
	calls := 0
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 3,
		Cooldown:         10 * time.Second,
		Now:              clk.Now,
	})

	// As 3 primeiras VÃO à rede e falham.
	for i := 1; i <= 3; i++ {
		err := b.Do(context.Background(), func() error { calls++; return fail() })
		if !errors.Is(err, errBoom) {
			t.Fatalf("tentativa %d: err = %v, esperado o erro do serviço", i, err)
		}
	}
	if calls != 3 {
		t.Fatalf("chamadas = %d, esperado 3", calls)
	}
	if got := b.State(); got != circuitbreaker.StateOpen {
		t.Fatalf("estado = %v, esperado aberto após 3 falhas", got)
	}

	// A 4ª nem sai daqui. É este o ponto do disjuntor: a requisição não fica
	// pendurada 5s esperando um timeout que já se sabe que vai acontecer.
	err := b.Do(context.Background(), func() error { calls++; return fail() })
	if !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("err = %v, esperado ErrOpen", err)
	}
	if calls != 3 {
		t.Fatalf("chamadas = %d — o disjuntor aberto DEIXOU a chamada ir à rede", calls)
	}
}

// TestSucessoZeraContadorDeFalhas — o limiar é de falhas CONSECUTIVAS. Duas
// falhas espalhadas ao longo do dia não podem abrir o circuito.
func TestSucessoZeraContadorDeFalhas(t *testing.T) {
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 3, Now: newClock().Now,
	})

	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), ok) // zera
	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)

	if got := b.State(); got != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v, esperado fechado: as falhas não foram consecutivas", got)
	}
}

// TestDisjuntorFechaDepoisDeProvaBemSucedida — o ciclo completo
// fechado → aberto → meio-aberto → fechado. Sem este caminho, o disjuntor
// protege uma vez e derruba o checkout para sempre.
func TestDisjuntorFechaDepoisDeProvaBemSucedida(t *testing.T) {
	clk := newClock()
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 2,
		Cooldown:         10 * time.Second,
		SuccessesToClose: 1,
		Now:              clk.Now,
	})

	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)
	if b.State() != circuitbreaker.StateOpen {
		t.Fatal("esperado aberto")
	}

	// Antes do cooldown vencer, continua recusando.
	clk.advance(9 * time.Second)
	if err := b.Do(context.Background(), ok); !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("err = %v, esperado ErrOpen antes do cooldown vencer", err)
	}

	// Vencido o cooldown, uma prova é admitida.
	clk.advance(2 * time.Second)
	called := false
	if err := b.Do(context.Background(), func() error { called = true; return nil }); err != nil {
		t.Fatalf("prova devolveu %v", err)
	}
	if !called {
		t.Fatal("a prova de meio-aberto não foi executada")
	}
	if got := b.State(); got != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v, esperado fechado após prova bem-sucedida", got)
	}

	// E volta a passar tudo normalmente.
	if err := b.Do(context.Background(), ok); err != nil {
		t.Fatalf("após fechar, err = %v", err)
	}
}

// TestProvaQueFalhaReabreImediatamente — meio-aberto não dá segunda chance.
func TestProvaQueFalhaReabreImediatamente(t *testing.T) {
	clk := newClock()
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 1, Cooldown: 5 * time.Second, Now: clk.Now,
	})

	_ = b.Do(context.Background(), fail)
	clk.advance(6 * time.Second)

	if err := b.Do(context.Background(), fail); !errors.Is(err, errBoom) {
		t.Fatalf("prova: err = %v", err)
	}
	if got := b.State(); got != circuitbreaker.StateOpen {
		t.Fatalf("estado = %v, esperado aberto: a prova falhou", got)
	}
	// E o cooldown recomeça do zero — não herda o tempo já decorrido.
	clk.advance(4 * time.Second)
	if err := b.Do(context.Background(), ok); !errors.Is(err, circuitbreaker.ErrOpen) {
		t.Fatalf("err = %v, esperado ErrOpen: o cooldown recomeçou", err)
	}
}

// TestMeioAbertoLimitaProvasSimultaneas — se o serviço remoto está afogado,
// mandar cem provas de uma vez o derruba de novo justo quando se recuperava.
func TestMeioAbertoLimitaProvasSimultaneas(t *testing.T) {
	clk := newClock()
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 1, Cooldown: time.Second, HalfOpenMaxCalls: 1, Now: clk.Now,
	})
	_ = b.Do(context.Background(), fail)
	clk.advance(2 * time.Second)

	// Segura a primeira prova dentro de fn e tenta uma segunda em paralelo.
	release := make(chan struct{})
	entered := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Do(context.Background(), func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	second := b.Do(context.Background(), ok)
	if !errors.Is(second, circuitbreaker.ErrOpen) {
		t.Fatalf("segunda prova simultânea: err = %v, esperado ErrOpen", second)
	}
	close(release)
	wg.Wait()
}

// TestErroDeNegocioNaoAbreCircuito — 404 do catálogo ("produto não existe") é
// resposta CORRETA. Se contasse como falha, um carrinho com produto removido
// abriria o disjuntor e derrubaria o checkout de todo mundo.
func TestErroDeNegocioNaoAbreCircuito(t *testing.T) {
	errNotFound := errors.New("product not found")
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 2,
		Now:              newClock().Now,
		IsFailure: func(err error) bool {
			return err != nil && !errors.Is(err, errNotFound)
		},
	})

	for i := 0; i < 10; i++ {
		_ = b.Do(context.Background(), func() error { return errNotFound })
	}
	if got := b.State(); got != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v — erro de negócio abriu o circuito", got)
	}
}

// TestTransicoesNotificamObservador — a métrica só existe se o callback for
// chamado em toda transição. Disjuntor aberto sem alerta é falha silenciosa.
func TestTransicoesNotificamObservador(t *testing.T) {
	clk := newClock()
	var mu sync.Mutex
	var seen []string
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 1, Cooldown: time.Second, Now: clk.Now,
		OnStateChange: func(_ string, from, to circuitbreaker.State) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, from.String()+"->"+to.String())
		},
	})

	_ = b.Do(context.Background(), fail)
	clk.advance(2 * time.Second)
	_ = b.Do(context.Background(), ok)

	want := []string{"closed->open", "open->half_open", "half_open->closed"}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != len(want) {
		t.Fatalf("transições = %v, esperado %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("transições = %v, esperado %v", seen, want)
		}
	}
}

// TestContadorDeFalhasFicaVisivel — o painel precisa do número real.
func TestContadorDeFalhasFicaVisivel(t *testing.T) {
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 10, Now: newClock().Now,
	})
	for i := 0; i < 4; i++ {
		_ = b.Do(context.Background(), fail)
	}
	if got := b.ConsecutiveFailures(); got != 4 {
		t.Fatalf("falhas = %d, esperado 4", got)
	}
}

// TestConcorrenciaNaoCorrompeEstado — roda sob -race. O disjuntor é
// compartilhado por todas as goroutines de request; é esse compartilhamento que
// faz a proteção existir, e é ele que precisa ser seguro.
func TestConcorrenciaNaoCorrompeEstado(t *testing.T) {
	clk := newClock()
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 5, Cooldown: time.Millisecond, Now: clk.Now,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = b.Do(context.Background(), func() error {
					if (i+j)%3 == 0 {
						return errBoom
					}
					return nil
				})
				_ = b.State()
				_ = b.ConsecutiveFailures()
			}
		}(i)
	}
	wg.Wait()
}

// TestContextoCanceladoNaoGastaProva — contabilizar como falha algo que nem
// saiu daqui poluiria o contador e abriria o circuito por engano.
func TestContextoCanceladoNaoGastaProva(t *testing.T) {
	b := circuitbreaker.New("catalog", circuitbreaker.Config{
		FailureThreshold: 1, Now: newClock().Now,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	err := b.Do(ctx, func() error { called = true; return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, esperado context.Canceled", err)
	}
	if called {
		t.Fatal("fn foi executada com contexto já cancelado")
	}
	if got := b.State(); got != circuitbreaker.StateClosed {
		t.Fatalf("estado = %v — contexto cancelado contou como falha", got)
	}
}

// TestMetricasRegistramEstadoETrips — prova que a série existe e muda.
func TestMetricasRegistramEstadoETrips(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := circuitbreaker.NewMetrics(reg, "order-service")

	clk := newClock()
	b := circuitbreaker.New("catalog", m.Instrument(circuitbreaker.Config{
		FailureThreshold: 2, Cooldown: time.Second, Now: clk.Now,
	}))

	_ = b.Do(context.Background(), fail)
	_ = b.Do(context.Background(), fail)
	m.Rejected("catalog")

	got := gather(t, reg)
	if got["utilar_circuit_breaker_state"] != float64(circuitbreaker.StateOpen) {
		t.Fatalf("state = %v, esperado %v", got["utilar_circuit_breaker_state"], float64(circuitbreaker.StateOpen))
	}
	if got["utilar_circuit_breaker_trips_total"] != 1 {
		t.Fatalf("trips = %v, esperado 1", got["utilar_circuit_breaker_trips_total"])
	}
	if got["utilar_circuit_breaker_failures_total"] != 2 {
		t.Fatalf("failures = %v, esperado 2", got["utilar_circuit_breaker_failures_total"])
	}
	if got["utilar_circuit_breaker_rejected_total"] != 1 {
		t.Fatalf("rejected = %v, esperado 1", got["utilar_circuit_breaker_rejected_total"])
	}
}

// TestInstrumentPreservaCallbackDoChamador — sobrescrever silenciosamente um
// OnStateChange já definido seria uma armadilha.
func TestInstrumentPreservaCallbackDoChamador(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := circuitbreaker.NewMetrics(reg, "order-service")

	called := false
	cfg := circuitbreaker.Config{
		FailureThreshold: 1, Now: newClock().Now,
		OnStateChange: func(string, circuitbreaker.State, circuitbreaker.State) { called = true },
	}
	b := circuitbreaker.New("catalog", m.Instrument(cfg))
	_ = b.Do(context.Background(), fail)

	if !called {
		t.Fatal("Instrument descartou o OnStateChange do chamador")
	}
}

// gather achata o registry em nome → valor do primeiro sample. Suficiente aqui:
// só existe um label `breaker`.
func gather(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]float64{}
	for _, f := range fams {
		for _, m := range f.GetMetric() {
			switch {
			case m.GetGauge() != nil:
				out[f.GetName()] = m.GetGauge().GetValue()
			case m.GetCounter() != nil:
				out[f.GetName()] = m.GetCounter().GetValue()
			}
		}
	}
	return out
}
