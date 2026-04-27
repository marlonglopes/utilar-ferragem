package handler

import (
	"testing"
	"time"
)

// A9-H5: padToMinElapsed garante piso mínimo de tempo de resposta.

func TestPadToMinElapsed_BloqueiaQuandoRapido(t *testing.T) {
	min := 50 * time.Millisecond
	start := time.Now()
	padToMinElapsed(start, min)
	elapsed := time.Since(start)
	if elapsed < min {
		t.Fatalf("pad falhou: elapsed=%v < min=%v", elapsed, min)
	}
	if elapsed > min+50*time.Millisecond {
		t.Fatalf("pad demorou demais: elapsed=%v vs min=%v", elapsed, min)
	}
}

func TestPadToMinElapsed_NaoBloqueiaQuandoLento(t *testing.T) {
	min := 5 * time.Millisecond
	start := time.Now().Add(-100 * time.Millisecond)
	t0 := time.Now()
	padToMinElapsed(start, min)
	if d := time.Since(t0); d > 5*time.Millisecond {
		t.Fatalf("pad deveria retornar imediatamente, mas dormiu %v", d)
	}
}

// O piso configurado precisa ser grande o suficiente pra englobar uma query
// + insert (~ tens de ms) — caso contrário a normalização não tem efeito.
func TestForgotPasswordMinElapsed_Razoavel(t *testing.T) {
	if forgotPasswordMinElapsed < 100*time.Millisecond {
		t.Fatalf("forgotPasswordMinElapsed=%v abaixo do recomendado (>=100ms)", forgotPasswordMinElapsed)
	}
}
