// L-AUTH-1: confirmar que login_failure cria linha em auth_events.
package handler_test

import (
	"net/http"
	"testing"
)

func TestAuditEvent_LoginFailure(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	// Conta antes
	var before int
	db.QueryRow(`SELECT count(*) FROM auth_events WHERE event_type = 'login_failure'`).Scan(&before)

	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "naoexiste-audit@utilar.com.br",
		"password": "qualquercoisa",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}

	var after int
	db.QueryRow(`SELECT count(*) FROM auth_events WHERE event_type = 'login_failure'`).Scan(&after)
	if after != before+1 {
		t.Errorf("auth_events não incrementou: before=%d after=%d", before, after)
	}

	// Cleanup
	db.Exec(`DELETE FROM auth_events WHERE created_at > now() - interval '5 minutes' AND event_type = 'login_failure'`)
}
