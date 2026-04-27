package handler

import (
	"testing"
)

// A7-H3: hashToken é determinístico e produz SHA-256 hex de 64 chars.

func TestHashToken_DeterministicAndShape(t *testing.T) {
	a := hashToken("abc")
	b := hashToken("abc")
	if a != b {
		t.Fatalf("hashToken não-determinístico: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("hashToken length = %d, esperado 64", len(a))
	}
	for _, ch := range a {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			t.Fatalf("hashToken não é hex: %q", a)
		}
	}
}

func TestHashToken_DifferentInputsDifferentHashes(t *testing.T) {
	if hashToken("a") == hashToken("b") {
		t.Fatal("colisão trivial entre inputs distintos")
	}
}

// Sanity: SHA-256 conhecido de "abc" é
// "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad".
func TestHashToken_KnownVector(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	got := hashToken("abc")
	if got != want {
		t.Fatalf("hash de %q = %q, esperado %q", "abc", got, want)
	}
}
