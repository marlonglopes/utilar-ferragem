package handler_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
)

// Bloqueio de conta após N tentativas (anti brute-force). Exige Postgres :5438
// com a migration 006. Usa um usuário DEDICADO por teste — nunca o seed — para
// não deixar contas travadas para os outros testes.

// Espelha handler.maxFailedLogins (não exportado; o pacote de teste é externo).
const maxLoginAttemptsForTest = 5

// freshUser devolve email + senha para um usuário dedicado (limpa um resquício
// anterior de mesmo nome, se houver).
func freshUser(t *testing.T, db *sql.DB, label string) (string, string) {
	t.Helper()
	email := fmt.Sprintf("lockout-%s@utilar.com.br", label)
	_, _ = db.Exec("DELETE FROM users WHERE email = $1", email)
	return email, "secret-pass-1"
}

func TestLogin_LockoutAposNFalhas(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email, pass := freshUser(t, db, "n-falhas")
	defer db.Exec("DELETE FROM users WHERE email = $1", email)
	if w := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": pass, "name": "Lock Test",
	}); w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}

	// 5 senhas erradas: cada uma 401. A 5ª bloqueia a conta.
	for i := 1; i <= maxLoginAttemptsForTest; i++ {
		w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"email": email, "password": "errada",
		})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("tentativa %d: esperava 401, veio %d", i, w.Code)
		}
	}

	// Agora, MESMO com a senha CERTA, a conta está bloqueada → 429 (não confirma
	// que a senha estava certa; nem chega a verificar).
	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": email, "password": pass,
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("conta bloqueada: esperava 429, veio %d %s", w.Code, w.Body.String())
	}

	// E o lock foi persistido na linha do usuário.
	var lockedUntil sql.NullTime
	if err := db.QueryRow("SELECT locked_until FROM users WHERE email=$1", email).Scan(&lockedUntil); err != nil {
		t.Fatalf("ler locked_until: %v", err)
	}
	if !lockedUntil.Valid {
		t.Fatal("locked_until não foi gravado após N falhas")
	}
}

func TestLogin_ResetContadorNoSucesso(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email, pass := freshUser(t, db, "reset")
	defer db.Exec("DELETE FROM users WHERE email = $1", email)
	do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": pass, "name": "Reset Test",
	})

	// 3 falhas (abaixo do teto → não bloqueia).
	for i := 0; i < 3; i++ {
		do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{"email": email, "password": "errada"})
	}
	// Senha certa: entra E zera o contador.
	if w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": email, "password": pass,
	}); w.Code != http.StatusOK {
		t.Fatalf("login ok esperado 200, veio %d %s", w.Code, w.Body.String())
	}
	var attempts int
	db.QueryRow("SELECT failed_login_attempts FROM users WHERE email=$1", email).Scan(&attempts)
	if attempts != 0 {
		t.Fatalf("contador não zerou no sucesso: %d", attempts)
	}
}

func TestLogin_LockExpiraPermiteNovaTentativa(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email, pass := freshUser(t, db, "expira")
	defer db.Exec("DELETE FROM users WHERE email = $1", email)
	do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": pass, "name": "Expira Test",
	})

	// Bloqueia manualmente com um lock JÁ EXPIRADO (no passado).
	if _, err := db.Exec(`UPDATE users SET failed_login_attempts=5, locked_until = now() - interval '1 minute' WHERE email=$1`, email); err != nil {
		t.Fatalf("forçar lock expirado: %v", err)
	}

	// Lock expirado → a senha certa entra normalmente (janela recomeça).
	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": email, "password": pass,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("lock expirado devia permitir login: veio %d %s", w.Code, w.Body.String())
	}
}
