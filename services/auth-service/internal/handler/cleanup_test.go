// A14-M5: runTokenCleanup apaga tokens expirados, deixa válidos intactos.
package handler_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/utilar/auth-service/internal/handler"
)

func sha256HexCleanup(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestTokenCleanup_ApagaExpirados(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	// Cria token expirado e válido pra mesma user
	var userID string
	if err := db.QueryRow(`SELECT id FROM users WHERE email = 'test1@utilar.com.br'`).Scan(&userID); err != nil {
		t.Skipf("user de teste ausente: %v", err)
	}

	expHash := sha256HexCleanup("expired-test-token-cleanup-xyz")
	validHash := sha256HexCleanup("valid-test-token-cleanup-xyz")

	defer db.Exec(`DELETE FROM refresh_tokens WHERE token_hash IN ($1, $2)`, expHash, validHash)

	if _, err := db.Exec(`
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, expHash, userID, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, validHash, userID, time.Now().Add(1*time.Hour)); err != nil {
		t.Fatalf("insert valid: %v", err)
	}

	// Roda cleanup uma vez via boot do worker (intervalo grande pra não disparar 2x)
	ctx, cancel := context.WithCancel(context.Background())
	handler.StartTokenCleanup(ctx, db)

	// Espera execução inicial
	time.Sleep(100 * time.Millisecond)
	cancel()

	var nExp, nValid int
	db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, expHash).Scan(&nExp)
	db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, validHash).Scan(&nValid)

	if nExp != 0 {
		t.Errorf("expirado não foi apagado: %d ainda no DB", nExp)
	}
	if nValid != 1 {
		t.Errorf("válido foi apagado erroneamente: count=%d", nValid)
	}
}
