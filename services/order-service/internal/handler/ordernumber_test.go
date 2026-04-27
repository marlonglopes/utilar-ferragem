package handler

import (
	"regexp"
	"strings"
	"testing"
)

// O2-H4: order number deve ter alta entropia e não ser sequencial.
// Formato esperado: YYYY-XXXXXXXX onde X é base32 [A-Z2-7].
var orderNumberRe = regexp.MustCompile(`^\d{4}-[A-Z2-7]{8}$`)

func TestGenerateOrderNumber_Format(t *testing.T) {
	got := generateOrderNumber(2026)
	if !orderNumberRe.MatchString(got) {
		t.Fatalf("formato inesperado: %q", got)
	}
	if !strings.HasPrefix(got, "2026-") {
		t.Fatalf("prefixo de ano ausente: %q", got)
	}
}

func TestGenerateOrderNumber_NoCollisionsBatch(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		v := generateOrderNumber(2026)
		if _, dup := seen[v]; dup {
			t.Fatalf("colisão em %d gerações: %q", n, v)
		}
		seen[v] = struct{}{}
	}
}

// Sanity: 40 bits significa que sequência de 5 chamadas consecutivas não deve
// diferir só no último char (como UnixNano%100000 fazia).
func TestGenerateOrderNumber_NotSequential(t *testing.T) {
	a := generateOrderNumber(2026)
	b := generateOrderNumber(2026)
	if a == b {
		t.Fatalf("dois números iguais consecutivos: %q", a)
	}
	// Compara só os 8 chars do sufixo. Se diferir em pelo menos 2 caracteres,
	// é razoavelmente aleatório.
	suffixA, suffixB := a[5:], b[5:]
	diff := 0
	for i := 0; i < 8; i++ {
		if suffixA[i] != suffixB[i] {
			diff++
		}
	}
	if diff < 2 {
		t.Fatalf("entropia suspeita entre %q e %q (diff=%d)", a, b, diff)
	}
}
