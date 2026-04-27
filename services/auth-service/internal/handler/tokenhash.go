package handler

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashToken transforma um token opaco (32 hex chars de randToken) em SHA-256
// hex pra armazenamento em DB (A7-H3).
//
// SHA-256 sem salt é apropriado aqui — o input já tem 128 bits de entropia,
// então rainbow table não compensa. Salt + bcrypt seria overkill (e custoso
// em latência de auth) sem benefício real.
//
// Constant-time comparison não é necessária na lookup: usamos `WHERE token_hash = $1`,
// e o atacante não controla a comparação individual (Postgres BTREE índice
// retorna direto). O hash em si é suficiente — sem ele, vazamento de DB =
// vazamento direto dos tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
