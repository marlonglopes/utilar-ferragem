package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
)

// A atividade unificada precisa das DUAS trilhas do order (PDV e devolução)
// numa lista só, no shape do catalog/auth. Este teste insere um evento em cada
// tabela e confirma: aparecem juntos, a entidade vem do prefixo da ação, o
// `amount` do dinheiro está lá, e o filtro por entidade separa as fontes.
func TestOrderAuditList_UneBalcaoEDevolucaoNoShapeUnificado(t *testing.T) {
	db := dashDB(t) // skipa sem banco
	defer db.Close()

	const actor = "audit-union-test-actor"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM balcao_audit_events WHERE actor_id = $1`, actor)
		_, _ = db.Exec(`DELETE FROM return_audit_events WHERE actor_id = $1`, actor)
	})

	// Evento de PDV: desconto aplicado (order_id NULL evita depender de um pedido).
	if _, err := db.Exec(`
		INSERT INTO balcao_audit_events (action, actor_id, actor_role, old_value, new_value, amount)
		VALUES ('discount.applied', $1, 'store_operator', '{"discountPct":0}', '{"discountPct":18}', 12.34)`,
		actor); err != nil {
		t.Skipf("sem tabela/banco: %v", err)
	}
	// Evento de devolução: estorno (dinheiro saindo).
	if _, err := db.Exec(`
		INSERT INTO return_audit_events (action, actor_id, actor_role, old_value, new_value, amount)
		VALUES ('return.refunded', $1, 'admin', '{"status":"received"}', '{"status":"refunded"}', 99.90)`,
		actor); err != nil {
		t.Fatalf("insert return audit: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/audit", handler.NewAuditHandler(db).AuditList)

	get := func(query string) []map[string]any {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.Data
	}

	// Sem filtro de entidade, mas filtrando pelo ator: as DUAS fontes aparecem.
	all := get("actor=" + actor + "&per_page=100")
	var temDiscount, temReturn bool
	for _, ev := range all {
		if ev["action"] == "discount.applied" {
			temDiscount = true
			if ev["entity"] != "discount" {
				t.Errorf("entity de discount.applied = %v, queria discount", ev["entity"])
			}
			if ev["amount"] != 12.34 {
				t.Errorf("amount do desconto = %v, queria 12.34", ev["amount"])
			}
		}
		if ev["action"] == "return.refunded" {
			temReturn = true
			if ev["entity"] != "return" {
				t.Errorf("entity de return.refunded = %v, queria return", ev["entity"])
			}
			ch, _ := ev["changes"].(map[string]any)
			status, _ := ch["status"].(map[string]any)
			if status["old"] != "received" || status["new"] != "refunded" {
				t.Errorf("diff do estorno errado: %v", status)
			}
		}
	}
	if !temDiscount || !temReturn {
		t.Fatalf("faltou fonte na união: discount=%v return=%v", temDiscount, temReturn)
	}

	// Filtro por entidade separa: entity=return traz só a devolução.
	onlyReturn := get("actor=" + actor + "&entity=return&per_page=100")
	for _, ev := range onlyReturn {
		if ev["entity"] != "return" {
			t.Errorf("filtro entity=return trouxe %v", ev["entity"])
		}
	}
}
