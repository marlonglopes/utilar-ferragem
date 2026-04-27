package handler

import (
	"testing"
	"time"
)

// CT1-H4: padToMinElapsed bloqueia até `min` se elapsed for menor;
// retorna imediatamente caso já tenha passado.

func TestPadToMinElapsed_BloqueiaQuandoRapido(t *testing.T) {
	min := 50 * time.Millisecond
	start := time.Now()
	padToMinElapsed(start, min)
	elapsed := time.Since(start)
	if elapsed < min {
		t.Fatalf("pad falhou: elapsed=%v < min=%v", elapsed, min)
	}
	// Tolerância generosa pra schedulers compartilhados (CI).
	if elapsed > min+50*time.Millisecond {
		t.Fatalf("pad demorou demais: elapsed=%v vs min=%v", elapsed, min)
	}
}

func TestPadToMinElapsed_NaoBloqueiaQuandoLento(t *testing.T) {
	min := 5 * time.Millisecond
	start := time.Now().Add(-100 * time.Millisecond) // já passou bem mais que min
	t0 := time.Now()
	padToMinElapsed(start, min)
	if d := time.Since(t0); d > 5*time.Millisecond {
		t.Fatalf("pad deveria retornar imediatamente, mas dormiu %v", d)
	}
}

// Verifica que o limiar configurado é razoável: pelo menos 25ms (suficiente
// pra ofuscar latência local de DB miss vs hit).
func TestSlugLookupMinElapsed_Razoavel(t *testing.T) {
	if slugLookupMinElapsed < 25*time.Millisecond {
		t.Fatalf("slugLookupMinElapsed=%v abaixo do recomendado (>=25ms)", slugLookupMinElapsed)
	}
}
