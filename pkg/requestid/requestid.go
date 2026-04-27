// Package requestid fornece geração padronizada de Request-IDs via ULID.
//
// ULID em vez de UUID v4 (M11, CT1-M4):
//   - 26 chars Crockford-base32 (vs 36 chars UUID); mais curto em logs
//   - Lexicograficamente ordenável por tempo (k-sortable) — agrupa requests
//     próximas na ordenação ASCII de logs
//   - Mesmo nível de unicidade prática que UUID v4 (80 bits aleatórios)
//
// Compartilhado entre os 4 serviços via go.work — substitui as 4 cópias do
// `newRequestID()` baseado em UnixNano.
package requestid

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

const HeaderName = "X-Request-Id"

// monotonicEntropy é o pool de entropia. ulid.Monotonic garante que IDs
// gerados na mesma ms sejam ordenados — útil pra log replay determinístico.
// Mutex pq monotonic.Read não é thread-safe.
var (
	mu              sync.Mutex
	monotonicEntropy = ulid.Monotonic(rand.Reader, 0)
)

// New retorna um novo ULID como string Crockford-base32 (26 chars uppercase).
//
// Falla aberto: se entropy.Read falha (impossível na prática com crypto/rand),
// retorna um ULID baseado em timestamp puro. Não panic — request_id é
// telemetria, não security primitive.
func New() string {
	mu.Lock()
	defer mu.Unlock()
	id, err := ulid.New(ulid.Timestamp(time.Now()), monotonicEntropy)
	if err != nil {
		// fallback: timestamp-only (preserva ordenação)
		return ulid.MustNew(ulid.Timestamp(time.Now()), nil).String()
	}
	return id.String()
}
