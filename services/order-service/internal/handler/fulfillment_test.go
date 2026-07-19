// Testes dos endpoints de operação (separar / despachar / entregar).
// Precisam de DB. Skipam sem banco.
package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/handler"
)

func opsRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewOrderHandler(db, nil, true)
	ops := r.Group("/api/v1/admin", handler.RequireRole("test-secret", true, "admin", "operator"))
	ops.PATCH("/orders/:id/picking", h.MarkPicking)
	ops.PATCH("/orders/:id/shipped", h.MarkShipped)
	ops.PATCH("/orders/:id/delivered", h.MarkDelivered)
	ops.PATCH("/orders/:id/cancel", h.AdminCancel)
	return r
}

func doOps(r *gin.Engine, orderID, action, role string, body map[string]any) *httptest.ResponseRecorder {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/admin/orders/"+orderID+"/"+action, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("X-User-Role", role)
		req.Header.Set("X-User-Id", "operator-1")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// seedPaidOrder cria um pedido já pago, ponto de partida do fluxo de operação.
func seedOrderWithStatus(t *testing.T, db *sql.DB, status string) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO orders (number, user_id, status, payment_method, subtotal, shipping_cost, total, paid_at)
		VALUES ('OPS-' || substr(md5(random()::text), 1, 12), 'ops-test-user', $1::order_status, 'pix', 100, 20, 120,
		        CASE WHEN $1 = 'pending_payment' THEN NULL ELSE now() END)
		RETURNING id
	`, status).Scan(&id)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orders WHERE id = $1`, id) })
	return id
}

func orderStatus(t *testing.T, db *sql.DB, id string) (string, *time.Time, *time.Time, *string) {
	t.Helper()
	var status string
	var picked, shipped *time.Time
	var tracking *string
	err := db.QueryRow(
		`SELECT status, picked_at, shipped_at, tracking_code FROM orders WHERE id=$1`, id,
	).Scan(&status, &picked, &shipped, &tracking)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	return status, picked, shipped, tracking
}

// O fluxo completo de operação, na ordem certa, preenchendo os timestamps que
// até aqui eram colunas mortas.
func TestFulfillment_HappyPathFillsTimestamps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)
	id := seedOrderWithStatus(t, db, "paid")

	if w := doOps(r, id, "picking", "operator", nil); w.Code != http.StatusOK {
		t.Fatalf("picking: %d %s", w.Code, w.Body.String())
	}
	status, picked, _, _ := orderStatus(t, db, id)
	if status != "picking" || picked == nil {
		t.Fatalf("após picking: status=%q picked_at=%v", status, picked)
	}

	if w := doOps(r, id, "shipped", "operator", map[string]any{
		"trackingCode": "BR123456789BR", "location": "CD Guarulhos",
	}); w.Code != http.StatusOK {
		t.Fatalf("shipped: %d %s", w.Code, w.Body.String())
	}
	status, _, shipped, tracking := orderStatus(t, db, id)
	if status != "shipped" || shipped == nil {
		t.Fatalf("após shipped: status=%q shipped_at=%v", status, shipped)
	}
	if tracking == nil || *tracking != "BR123456789BR" {
		t.Errorf("tracking_code = %v; queria BR123456789BR", tracking)
	}

	if w := doOps(r, id, "delivered", "operator", nil); w.Code != http.StatusOK {
		t.Fatalf("delivered: %d %s", w.Code, w.Body.String())
	}
	status, _, _, _ = orderStatus(t, db, id)
	if status != "delivered" {
		t.Errorf("status final = %q; queria delivered", status)
	}

	// Cada etapa gravou um tracking event.
	var n int
	db.QueryRow(`SELECT count(*) FROM tracking_events WHERE order_id=$1`, id).Scan(&n)
	if n != 3 {
		t.Errorf("esperava 3 tracking events (picking/shipped/delivered), veio %d", n)
	}
}

// REGRESSÃO: pular etapa é 409, não sucesso silencioso.
func TestFulfillment_RejectsSkippingSteps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)

	// paid → shipped pula a separação.
	id := seedOrderWithStatus(t, db, "paid")
	w := doOps(r, id, "shipped", "operator", map[string]any{"trackingCode": "BR1"})
	if w.Code != http.StatusConflict {
		t.Errorf("paid → shipped deveria dar 409, veio %d: %s", w.Code, w.Body.String())
	}

	// pending_payment → picking: pedido não pago não se separa.
	id2 := seedOrderWithStatus(t, db, "pending_payment")
	if w := doOps(r, id2, "picking", "operator", nil); w.Code != http.StatusConflict {
		t.Errorf("pending_payment → picking deveria dar 409, veio %d", w.Code)
	}
}

// REGRESSÃO: pedido cancelado é terminal — nenhuma operação o move.
func TestFulfillment_RejectsOperationsOnCancelled(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)
	id := seedOrderWithStatus(t, db, "cancelled")

	for _, action := range []string{"picking", "shipped", "delivered"} {
		body := map[string]any{"trackingCode": "BR1"}
		if w := doOps(r, id, action, "operator", body); w.Code != http.StatusConflict {
			t.Errorf("%s em pedido cancelado deveria dar 409, veio %d", action, w.Code)
		}
	}
}

// REGRESSÃO: um pedido "enviado" sem rastreio é uma reclamação garantida.
func TestFulfillment_ShippedRequiresTrackingCode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)
	id := seedOrderWithStatus(t, db, "picking")

	if w := doOps(r, id, "shipped", "operator", nil); w.Code != http.StatusBadRequest {
		t.Errorf("shipped sem trackingCode deveria dar 400, veio %d", w.Code)
	}
	if w := doOps(r, id, "shipped", "operator", map[string]any{"trackingCode": ""}); w.Code != http.StatusBadRequest {
		t.Errorf("shipped com trackingCode vazio deveria dar 400, veio %d", w.Code)
	}

	// O pedido não pode ter mudado.
	status, _, _, _ := orderStatus(t, db, id)
	if status != "picking" {
		t.Errorf("status = %q; deveria continuar picking", status)
	}
}

// Rotas de operação não são públicas.
func TestFulfillment_RequiresOperatorRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)
	id := seedOrderWithStatus(t, db, "paid")

	if w := doOps(r, id, "picking", "", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("sem role deveria dar 401, veio %d", w.Code)
	}
	// Um cliente comum (role=user) não pode operar o fulfillment.
	if w := doOps(r, id, "picking", "user", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("role=user deveria ser recusado, veio %d", w.Code)
	}
	// admin também opera.
	if w := doOps(r, id, "picking", "admin", nil); w.Code != http.StatusOK {
		t.Errorf("role=admin deveria poder operar, veio %d: %s", w.Code, w.Body.String())
	}
}

func TestFulfillment_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)

	var ghost string
	db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&ghost)
	if w := doOps(r, ghost, "picking", "operator", nil); w.Code != http.StatusNotFound {
		t.Errorf("pedido inexistente deveria dar 404, veio %d", w.Code)
	}
}

// Cancelamento pelo operador funciona nos estados pré-despacho.
func TestAdminCancel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := opsRouter(db)

	id := seedOrderWithStatus(t, db, "paid")
	if w := doOps(r, id, "cancel", "operator", map[string]any{"note": "Item avariado no estoque."}); w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body.String())
	}
	status, _, _, _ := orderStatus(t, db, id)
	if status != "cancelled" {
		t.Errorf("status = %q; queria cancelled", status)
	}

	// Depois de despachado, nem o operador cancela pelo fluxo normal.
	id2 := seedOrderWithStatus(t, db, "shipped")
	if w := doOps(r, id2, "cancel", "operator", nil); w.Code != http.StatusConflict {
		t.Errorf("cancelar pedido despachado deveria dar 409, veio %d", w.Code)
	}
}
