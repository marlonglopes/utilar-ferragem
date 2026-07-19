package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/utilar/pkg/retry"
)

var errUpstream = errors.New("502 do serviço remoto")

// recorder captura as esperas sem dormir de verdade.
func recorder() (func(context.Context, time.Duration) error, *[]time.Duration) {
	var slept []time.Duration
	return func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}, &slept
}

// ============================================================================
// A REGRA DO PACOTE — retry em operação não idempotente cobra duas vezes
// ============================================================================

// TestRegression_NaoRetentaOperacaoNaoIdempotente é O teste deste pacote.
//
// Modo de falha que ele previne: uma criação de cobrança no PSP dá timeout,
// o cliente HTTP repete, a Appmax (que NÃO tem chave de idempotência) processa
// as duas — e o cliente vê duas cobranças no extrato do cartão. Mesmo motivo
// pelo qual appmaxv1.isFinancialRoute desliga o retry em /v1/payments/*.
func TestRegression_NaoRetentaOperacaoNaoIdempotente(t *testing.T) {
	sleep, slept := recorder()
	calls := 0

	p := retry.Policy{
		Safety:      retry.NonIdempotent,
		MaxAttempts: 5, // configurado alto DE PROPÓSITO: tem que ser ignorado
		Base:        time.Millisecond,
	}.WithSleep(sleep)

	err := retry.Do(context.Background(), p, func(int) error {
		calls++
		return errUpstream
	})

	if calls != 1 {
		t.Fatalf("chamadas = %d, esperado 1 — operação não idempotente foi REPETIDA "+
			"(este é o caminho da cobrança em duplicidade)", calls)
	}
	if len(*slept) != 0 {
		t.Fatalf("dormiu %v vezes numa operação que não pode repetir", len(*slept))
	}
	if !errors.Is(err, errUpstream) {
		t.Fatalf("err = %v, esperado o erro do serviço", err)
	}
}

// TestRegression_ZeroValueDaPolicyNaoRetenta — quem esquecer de declarar
// Safety ganha o comportamento SEGURO. É por isso que NonIdempotent é o zero
// value do enum: o erro de distração custa uma requisição perdida, nunca uma
// cobrança dupla.
func TestRegression_ZeroValueDaPolicyNaoRetenta(t *testing.T) {
	calls := 0
	// Policy zerada, a não ser pelo MaxAttempts — o caso do desenvolvedor
	// distraído que só configurou "quantas vezes".
	p := retry.Policy{MaxAttempts: 4}
	_ = retry.Do(context.Background(), p, func(int) error { calls++; return errUpstream })

	if calls != 1 {
		t.Fatalf("chamadas = %d, esperado 1 — Policy sem Safety declarada RETENTOU", calls)
	}
	if got := (retry.Policy{MaxAttempts: 9}).Attempts(); got != 1 {
		t.Fatalf("Attempts() = %d, esperado 1", got)
	}
}

// ============================================================================
// Caminho idempotente
// ============================================================================

func TestRetentaOperacaoIdempotente(t *testing.T) {
	sleep, slept := recorder()
	calls := 0

	p := retry.Policy{
		Safety: retry.Idempotent, MaxAttempts: 3, Base: 100 * time.Millisecond,
	}.WithSleep(sleep)

	err := retry.Do(context.Background(), p, func(int) error { calls++; return errUpstream })
	if err == nil {
		t.Fatal("esperado erro após esgotar as tentativas")
	}
	if calls != 3 {
		t.Fatalf("chamadas = %d, esperado 3", calls)
	}
	// 3 tentativas = 2 esperas.
	if len(*slept) != 2 {
		t.Fatalf("esperas = %v, esperado 2", *slept)
	}
}

func TestParaNaPrimeiraTentativaBemSucedida(t *testing.T) {
	sleep, slept := recorder()
	calls := 0
	p := retry.Policy{Safety: retry.Idempotent, MaxAttempts: 5, Base: time.Millisecond}.
		WithSleep(sleep)

	if err := retry.Do(context.Background(), p, func(int) error {
		calls++
		if calls < 3 {
			return errUpstream
		}
		return nil
	}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 3 {
		t.Fatalf("chamadas = %d, esperado 3 (parou ao dar certo)", calls)
	}
	if len(*slept) != 2 {
		t.Fatalf("esperas = %v, esperado 2", *slept)
	}
}

// TestRecuoEExponencialComTeto — sem teto, seis tentativas com base de 300ms
// já esperam mais que o timeout do checkout inteiro.
func TestRecuoEExponencialComTeto(t *testing.T) {
	sleep, slept := recorder()
	p := retry.Policy{
		Safety: retry.Idempotent, MaxAttempts: 6,
		Base: 100 * time.Millisecond, Max: 400 * time.Millisecond,
	}.WithSleep(sleep)

	_ = retry.Do(context.Background(), p, func(int) error { return errUpstream })

	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond,
		400 * time.Millisecond, 400 * time.Millisecond, 400 * time.Millisecond}
	if len(*slept) != len(want) {
		t.Fatalf("esperas = %v, esperado %v", *slept, want)
	}
	for i := range want {
		if (*slept)[i] != want[i] {
			t.Fatalf("espera[%d] = %v, esperado %v (exponencial com teto)", i, (*slept)[i], want[i])
		}
	}
}

// TestJitterFicaDentroDaFaixa — o jitter espalha ±25%; fora dessa faixa
// significaria espera maior que o previsto (ou zero, que anula o recuo).
func TestJitterFicaDentroDaFaixa(t *testing.T) {
	sleep, slept := recorder()
	p := retry.Policy{
		Safety: retry.Idempotent, MaxAttempts: 30,
		Base: time.Second, Max: time.Second, Jitter: true,
	}.WithSleep(sleep)

	_ = retry.Do(context.Background(), p, func(int) error { return errUpstream })

	lo, hi := 750*time.Millisecond, 1250*time.Millisecond
	distinct := map[time.Duration]bool{}
	for _, d := range *slept {
		if d < lo || d > hi {
			t.Fatalf("espera %v fora da faixa [%v, %v]", d, lo, hi)
		}
		distinct[d] = true
	}
	if len(distinct) < 2 {
		t.Fatal("todas as esperas idênticas — o jitter não está espalhando " +
			"(cem clientes voltariam juntos e afogariam o serviço de novo)")
	}
}

// TestErroNaoRetentavelParaNaHora — 404 e 400 são respostas corretas.
// Repeti-las só gasta tempo e entrega o mesmo erro mais tarde.
func TestErroNaoRetentavelParaNaHora(t *testing.T) {
	errNotFound := errors.New("404 not found")
	calls := 0
	sleep, _ := recorder()

	p := retry.Policy{
		Safety: retry.Idempotent, MaxAttempts: 5, Base: time.Millisecond,
		Retryable: func(err error) bool { return !errors.Is(err, errNotFound) },
	}.WithSleep(sleep)

	err := retry.Do(context.Background(), p, func(int) error { calls++; return errNotFound })
	if calls != 1 {
		t.Fatalf("chamadas = %d, esperado 1 (erro não retentável)", calls)
	}
	if !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// TestContextoCanceladoInterrompe — o cliente já desistiu; segurar a goroutine
// por mais alguns segundos de recuo só consome recurso do servidor.
func TestContextoCanceladoInterrompe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	p := retry.Policy{Safety: retry.Idempotent, MaxAttempts: 5, Base: time.Millisecond}.
		WithSleep(func(context.Context, time.Duration) error { return context.Canceled })

	err := retry.Do(ctx, p, func(int) error {
		calls++
		cancel()
		return errUpstream
	})
	if calls != 1 {
		t.Fatalf("chamadas = %d, esperado 1", calls)
	}
	// Devolve o erro do SERVIÇO, não o do contexto: é o que interessa no log.
	if !errors.Is(err, errUpstream) {
		t.Fatalf("err = %v, esperado o erro do serviço remoto", err)
	}
}

// TestNumeroDaTentativaEPassadoParaFn — usado para logar "tentativa 2 de 3".
func TestNumeroDaTentativaEPassadoParaFn(t *testing.T) {
	sleep, _ := recorder()
	var seen []int
	p := retry.Policy{Safety: retry.Idempotent, MaxAttempts: 3, Base: time.Millisecond}.
		WithSleep(sleep)

	_ = retry.Do(context.Background(), p, func(a int) error { seen = append(seen, a); return errUpstream })
	if len(seen) != 3 || seen[0] != 1 || seen[2] != 3 {
		t.Fatalf("tentativas = %v, esperado [1 2 3] (1-based)", seen)
	}
}
