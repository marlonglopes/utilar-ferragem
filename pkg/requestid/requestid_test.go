package requestid_test

import (
	"sync"
	"testing"

	"github.com/utilar/pkg/requestid"
)

func TestNew_FormatoESize(t *testing.T) {
	id := requestid.New()
	if len(id) != 26 {
		t.Fatalf("esperado 26 chars (ULID base32), got %d: %q", len(id), id)
	}
	// Crockford base32: 0-9 e A-Z exceto I, L, O, U
	for _, ch := range id {
		ok := (ch >= '0' && ch <= '9') ||
			(ch >= 'A' && ch <= 'H') ||
			(ch == 'J') || (ch == 'K') ||
			(ch == 'M') || (ch == 'N') ||
			(ch >= 'P' && ch <= 'T') ||
			(ch >= 'V' && ch <= 'Z')
		if !ok {
			t.Fatalf("char %q fora do alfabeto Crockford: %q", ch, id)
		}
	}
}

func TestNew_Distinto(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		v := requestid.New()
		if _, dup := seen[v]; dup {
			t.Fatalf("colisão em %d gerações", n)
		}
		seen[v] = struct{}{}
	}
}

// Monotonic: dois IDs gerados em sequência devem ser ordenados.
func TestNew_KSortable(t *testing.T) {
	prev := requestid.New()
	for i := 0; i < 100; i++ {
		curr := requestid.New()
		if curr <= prev {
			t.Fatalf("ordenação quebrada: %q <= %q", curr, prev)
		}
		prev = curr
	}
}

// Concorrência: gerar de várias goroutines não duplica nem panic.
func TestNew_ConcorrenteSemPanic(t *testing.T) {
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	seen := map[string]struct{}{}
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				id := requestid.New()
				mu.Lock()
				if _, dup := seen[id]; dup {
					t.Errorf("colisão concorrente: %q", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}
