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

// ListUsers é a base da gestão de staff (achar o userId pra promover a operador).
// Regressão: lista, filtra por papel, e NUNCA devolve password_hash.
func TestListUsers_ListaFiltraENaoVazaSenha(t *testing.T) {
	db, _ := setupTestDB(t) // skipa sem banco/seed
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewUserAdminHandler(db)
	r.GET("/api/v1/admin/users", h.ListUsers)

	do := func(query string) (string, map[string]any) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("json: %v", err)
		}
		return w.Body.String(), out
	}

	body, out := do("per_page=5")
	// A senha NÃO pode sair nunca — projeção explícita, sem SELECT *.
	if strings.Contains(strings.ToLower(body), "password") {
		t.Error("REGRESSÃO: resposta contém 'password' — a projeção vazou o hash")
	}
	meta := out["meta"].(map[string]any)
	if int(meta["total"].(float64)) == 0 {
		t.Skip("sem usuários no seed")
	}
	if len(out["data"].([]any)) == 0 {
		t.Error("total > 0 mas data veio vazio")
	}

	// Filtro por papel: role=admin devolve só admins.
	_, out = do("role=admin&per_page=50")
	for _, it := range out["data"].([]any) {
		u := it.(map[string]any)
		if u["role"] != "admin" {
			t.Errorf("filtro role=admin devolveu role=%v", u["role"])
		}
	}
}
