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

// A trilha de staff é a que responde "quem promoveu quem a admin/vendas". Este
// teste fecha o laço: muda o papel de alguém e confirma que a AÇÃO aparece na
// leitura, no MESMO shape do catalog (para a atividade unificada), com o
// old→new do papel.
func TestAuditList_MostraMudancaDePapelNoShapeUnificado(t *testing.T) {
	db, _ := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Sem ator (user_id vazio → actor_id NULL): store_audit_events.actor_id tem
	// FK para users(id), então um UUID de ator inventado violaria a FK e o
	// logStoreEvent (que falha aberto) engoliria a linha. Em produção o ator é
	// um admin real vindo do JWT; aqui o que importa é o alvo e o old→new.
	uh := handler.NewUserAdminHandler(db)
	ah := handler.NewAuditHandler(db)
	r.PATCH("/api/v1/admin/users/:id/role", uh.UpdateUserRole)
	r.GET("/api/v1/admin/audit", ah.AuditList)

	// Usuário efêmero.
	var uid string
	if err := db.QueryRow(`
		INSERT INTO users (email, password_hash, name)
		VALUES ('audit-persona@utilar.local','x','Audit Persona')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&uid); err != nil {
		t.Skipf("sem banco: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM store_audit_events WHERE target_id = $1`, uid)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, uid)
	})

	// Garante estado inicial conhecido e muda o papel (gera a linha de trilha).
	_, _ = db.Exec(`UPDATE users SET role='customer' WHERE id=$1`, uid)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+uid+"/role",
		strings.NewReader(`{"role":"contador"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("mudança de papel falhou: %d %s", w.Code, w.Body.String())
	}

	// Lê a trilha filtrando pelo alvo.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit?entityId="+uid, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("audit list: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Action   string          `json:"action"`
			Entity   string          `json:"entity"`
			EntityID *string         `json:"entityId"`
			Changes  json.RawMessage `json:"changes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("a mudança de papel não apareceu na trilha")
	}
	ev := resp.Data[0]
	if ev.Action != "user.role.update" {
		t.Errorf("action = %q, queria user.role.update", ev.Action)
	}
	if ev.Entity != "user" {
		t.Errorf("entity = %q, queria user (derivado do prefixo da ação)", ev.Entity)
	}
	// changes tem que trazer role: {old: customer, new: contador} — o formato
	// campo→{old,new} que a tela unificada renderiza igual ao do catalog.
	var ch map[string]map[string]any
	if err := json.Unmarshal(ev.Changes, &ch); err != nil {
		t.Fatalf("changes não é o shape esperado: %v (%s)", err, ev.Changes)
	}
	role, ok := ch["role"]
	if !ok {
		t.Fatalf("changes sem o campo role: %s", ev.Changes)
	}
	if role["old"] != "customer" || role["new"] != "contador" {
		t.Errorf("diff do papel errado: old=%v new=%v", role["old"], role["new"])
	}
}
