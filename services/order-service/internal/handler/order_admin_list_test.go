package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
)

func adminOrdersRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	orderH := handler.NewOrderHandler(db, nil, true) // devMode: aceita X-User-Role
	g := r.Group("/api/v1/admin", handler.RequireRole("test-secret", true, "admin", "operator"))
	g.GET("/orders", orderH.AdminList)
	return r
}

// AdminList é o endpoint que faltava para o painel de pedidos. Regressão: sem
// escopo de cliente (lista TUDO), meta.total bate com o banco, e o filtro de
// status devolve SÓ aquele status.
func TestAdminList_ListaTudoEFiltraStatus(t *testing.T) {
	db := dashDB(t) // skipa sem banco
	r := adminOrdersRouter(db)

	get := func(query string) (int, map[string]any) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?"+query, nil)
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("X-User-Id", "admin-test")
		r.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// 1) Sem filtro: 200 e meta.total == count(*) do banco (escopo = TODOS).
	code, out := get("per_page=5")
	if code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", code)
	}
	var dbTotal int
	if err := db.QueryRow("SELECT count(*) FROM orders").Scan(&dbTotal); err != nil {
		t.Fatalf("count: %v", err)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("resposta sem meta")
	}
	if got := int(meta["total"].(float64)); got != dbTotal {
		t.Errorf("meta.total = %d, banco tem %d (deve listar todos, sem escopo de cliente)", got, dbTotal)
	}

	// 2) Filtro status=paid: todo item devolvido é 'paid'.
	code, out = get("status=paid&per_page=100")
	if code != http.StatusOK {
		t.Fatalf("status=paid: %d", code)
	}
	for _, it := range out["data"].([]any) {
		o := it.(map[string]any)
		if o["status"] != "paid" {
			t.Errorf("filtro status=paid devolveu um pedido status=%v", o["status"])
		}
	}
}
