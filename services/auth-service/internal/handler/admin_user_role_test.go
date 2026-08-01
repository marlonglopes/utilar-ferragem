package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/auth-service/internal/handler"
)

// UpdateUserRole é como o dono atribui persona (contador/vendas/almoxarife).
// Regressões cobertas:
//   - papel desconhecido NÃO chega no enum do Postgres (400, não 500 vazando schema)
//   - só muda de fato e reporta changed corretamente
//   - usuário inexistente → 404
func TestUpdateUserRole_AtribuiPersonaEValida(t *testing.T) {
	db, _ := setupTestDB(t) // skipa sem banco
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Injeta um ator (o admin) como o JWTAuth faria, para a trilha ter autor.
	r.Use(func(c *gin.Context) { c.Set("user_id", "00000000-0000-4000-8000-0000000000ad") })
	h := handler.NewUserAdminHandler(db)
	r.PATCH("/api/v1/admin/users/:id/role", h.UpdateUserRole)

	// Cria um usuário efêmero para não tocar em ninguém do seed.
	var uid string
	err := db.QueryRow(`
		INSERT INTO users (email, password_hash, name)
		VALUES ('persona-test@utilar.local', 'x', 'Persona Teste')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&uid)
	if err != nil {
		t.Skipf("não consegui criar usuário de teste: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, uid) })

	patch := func(id, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+id+"/role", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// 1) papel válido novo → 200 changed=true, e o banco reflete.
	w := patch(uid, `{"role":"vendas"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("vendas: status %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["role"] != "vendas" || got["changed"] != true {
		t.Fatalf("esperava role=vendas changed=true, veio %v", got)
	}
	var dbRole string
	_ = db.QueryRow(`SELECT role::text FROM users WHERE id=$1`, uid).Scan(&dbRole)
	if dbRole != "vendas" {
		t.Fatalf("banco não gravou: role=%q", dbRole)
	}

	// 2) mesmo papel de novo → changed=false (idempotente, sem trilha à toa).
	w = patch(uid, `{"role":"vendas"}`)
	got = map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["changed"] != false {
		t.Errorf("repetir o mesmo papel deveria ser changed=false, veio %v", got)
	}

	// 3) papel inventado → 400 (NUNCA 500 vazando o enum do Postgres).
	w = patch(uid, `{"role":"bruxo"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("papel inválido: esperava 400, veio %d: %s", w.Code, w.Body.String())
	}

	// 4) `service` não é papel de usuário (A1) → 400.
	w = patch(uid, `{"role":"service"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("role=service deveria ser 400, veio %d", w.Code)
	}

	// 5) usuário inexistente → 404.
	w = patch("00000000-0000-4000-8000-0000000000ff", `{"role":"contador"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("uuid inexistente: esperava 404, veio %d", w.Code)
	}
}
