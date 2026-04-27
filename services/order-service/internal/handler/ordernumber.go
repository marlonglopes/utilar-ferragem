package handler

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
)

// generateOrderNumber retorna um número de pedido com prefixo de ano e
// 8 chars base32 (40 bits) de entropia. Não enumerável.
// Formato: "YYYY-XXXXXXXX". Colisão prática em ~1M pedidos/ano: ~negligible.
func generateOrderNumber(year int) string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand failed: %w", err))
	}
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return fmt.Sprintf("%d-%s", year, suffix)
}
