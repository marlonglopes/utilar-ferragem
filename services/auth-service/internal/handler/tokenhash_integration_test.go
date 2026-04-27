// A7-H3: integration test que valida invariante "DB nunca tem token plaintext".
//
// Após login, o refreshToken devolvido pelo endpoint deve ser DIFERENTE do
// valor armazenado em refresh_tokens.token_hash. O hash do plaintext deve
// bater com o token_hash.
package handler_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestRefreshToken_StoredAsHash(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	// Login pra emitir refresh token
	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "test1@utilar.com.br", "password": "utilar123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: status=%d body=%s", w.Code, w.Body.String())
	}
	var login struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.RefreshToken == "" {
		t.Fatal("refresh token vazio na response")
	}

	// O DB NÃO deve ter o plaintext.
	var hits int
	if err := db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, login.RefreshToken).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("plaintext refresh token encontrado em token_hash: regression A7-H3")
	}

	// O hash do plaintext bate com algum token_hash do DB.
	expectedHash := sha256Hex(login.RefreshToken)
	if err := db.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, expectedHash).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("hash esperado não encontrado em refresh_tokens (hits=%d)", hits)
	}

	// Limpa o token de teste pra não poluir DB.
	_, _ = db.Exec(`DELETE FROM refresh_tokens WHERE token_hash = $1`, expectedHash)
}

// Cobre password_reset_tokens.token_hash — usa ForgotPassword pra criar.
func TestPasswordResetToken_StoredAsHash(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email := "test1@utilar.com.br"
	w := do(r, http.MethodPost, "/api/v1/auth/forgot-password", "", map[string]any{"email": email})
	if w.Code != http.StatusOK {
		t.Fatalf("forgot status=%d body=%s", w.Code, w.Body.String())
	}

	// Sem capturar token (vai pro log dev), checamos só que o DB tem entrada
	// com token_hash de 64 chars hex e nada que se pareça com plaintext de
	// 32 chars hex (o output de randToken).
	rows, err := db.Query(`
		SELECT token_hash FROM password_reset_tokens
		WHERE user_id = (SELECT id FROM users WHERE email = $1)
		ORDER BY created_at DESC LIMIT 1
	`, email)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("nenhum token de reset criado")
	}
	var h string
	if err := rows.Scan(&h); err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Fatalf("token_hash length=%d, esperado 64 (sha256 hex)", len(h))
	}
	if !isHex(h) {
		t.Fatalf("token_hash não é hex: %q", h)
	}
	// Cleanup
	_, _ = db.Exec(`DELETE FROM password_reset_tokens WHERE token_hash = $1`, h)
}

func isHex(s string) bool {
	s = strings.ToLower(s)
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
