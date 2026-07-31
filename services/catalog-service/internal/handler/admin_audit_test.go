package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// A trilha do catálogo (catalog_audit_log) era gravada mas invisível — nenhuma
// tela lia. Este endpoint expõe "quem fez o quê": lista e filtra por ação.
func TestAuditList_LeTrilhaDoCatalogEFiltra(t *testing.T) {
	db := setupTestDB(t) // skipa sem banco/seed
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewAuditHandler(db)
	r.GET("/api/v1/admin/audit", h.AuditList)

	do := func(query string) map[string]any {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("json: %v", err)
		}
		return out
	}

	// Lista: 200 + estrutura data/meta.
	out := do("per_page=5")
	meta, _ := out["meta"].(map[string]any)
	if meta == nil {
		t.Fatal("resposta sem meta")
	}
	total := int(meta["total"].(float64))
	if total == 0 {
		t.Skip("catalog_audit_log vazio neste ambiente")
	}
	if len(out["data"].([]any)) == 0 {
		t.Error("meta.total > 0 mas data veio vazio")
	}

	// Filtro por ação: todo item devolvido tem a ação pedida.
	out = do("action=product.update&per_page=50")
	for _, it := range out["data"].([]any) {
		e := it.(map[string]any)
		if e["action"] != "product.update" {
			t.Errorf("filtro action=product.update devolveu action=%v", e["action"])
		}
	}
}
