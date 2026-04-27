package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/order-service/internal/handler"
)

// Integration tests — precisam de Postgres em :5437 com schema aplicado.
// Skipam se DB não responde.

const testUserID = "user-001"

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORDER_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM orders").Scan(&n); err != nil {
		t.Skipf("orders table not ready: %v", err)
	}
	if n == 0 {
		t.Skip("no orders in DB — run `make order-db-seed`")
	}
	return db
}

func setupRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	orderH := handler.NewOrderHandler(db)
	r.Use(handler.RequestID())
	api := r.Group("/api/v1", handler.RequireUser("test-secret", true))
	api.POST("/orders", orderH.Create)
	api.GET("/orders", orderH.List)
	api.GET("/orders/:id", orderH.Get)
	api.PATCH("/orders/:id/cancel", orderH.Cancel)
	return r
}

func do(r *gin.Engine, method, path, userID string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// -- auth -------------------------------------------------------------------

func TestOrders_Unauthorized(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := do(r, http.MethodGet, "/api/v1/orders", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401", w.Code)
	}
}

// -- list -------------------------------------------------------------------

func TestOrders_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := do(r, http.MethodGet, "/api/v1/orders", testUserID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Meta.Total < 1 {
		t.Errorf("want ≥1 order for seeded user, got %d", body.Meta.Total)
	}
}

func TestOrders_List_FilterActive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := do(r, http.MethodGet, "/api/v1/orders?status=active", testUserID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Data []struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	for _, o := range body.Data {
		if o.Status == "delivered" || o.Status == "cancelled" {
			t.Errorf("filtro active retornou pedido %q", o.Status)
		}
	}
}

func TestOrders_List_FilterDone(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := do(r, http.MethodGet, "/api/v1/orders?status=done", testUserID, nil)
	var body struct {
		Data []struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	for _, o := range body.Data {
		if o.Status != "delivered" && o.Status != "cancelled" {
			t.Errorf("filtro done retornou pedido %q", o.Status)
		}
	}
}

// -- get --------------------------------------------------------------------

func TestOrders_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	w := do(r, http.MethodGet, "/api/v1/orders/00000000-0000-0000-0000-000000000000", testUserID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404", w.Code)
	}
}

func TestOrders_Get_WrongUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	// Pega qualquer id de outro user e tenta ler com testUserID
	var someID string
	db.QueryRow("SELECT id FROM orders WHERE user_id != $1 LIMIT 1", testUserID).Scan(&someID)
	if someID == "" {
		t.Skip("sem order de outro user no seed")
	}
	w := do(r, http.MethodGet, "/api/v1/orders/"+someID, testUserID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user read retornou %d, esperado 404", w.Code)
	}
}

// -- create + cancel --------------------------------------------------------

func TestOrders_CreateAndCancel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	newUser := fmt.Sprintf("user-test-%d", 9999)

	payload := map[string]any{
		"paymentMethod": "pix",
		"shippingCost":  19.90,
		"items": []map[string]any{
			{"productId": "00000000-0000-0000-0000-000000000001", "name": "Produto X", "icon": "⚒",
				"sellerId": "s1", "sellerName": "Seller 1", "quantity": 2, "unitPrice": 100.00},
		},
		"address": map[string]any{
			"street": "Rua Teste", "number": "42", "neighborhood": "Centro",
			"city": "SP", "state": "SP", "cep": "01000-000",
		},
	}

	// Create
	w := do(r, http.MethodPost, "/api/v1/orders", newUser, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Total  float64 `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Status != "pending_payment" {
		t.Errorf("status inicial = %q, esperado pending_payment", created.Status)
	}
	wantTotal := 2*100.00 + 19.90
	if created.Total != wantTotal {
		t.Errorf("total = %.2f, esperado %.2f", created.Total, wantTotal)
	}

	// Cancel
	w = do(r, http.MethodPatch, "/api/v1/orders/"+created.ID+"/cancel", newUser, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", w.Code, w.Body.String())
	}
	var cancelled struct {
		Status         string `json:"status"`
		TrackingEvents []any  `json:"trackingEvents"`
	}
	json.Unmarshal(w.Body.Bytes(), &cancelled)
	if cancelled.Status != "cancelled" {
		t.Errorf("status após cancel = %q", cancelled.Status)
	}
	if len(cancelled.TrackingEvents) < 2 {
		t.Errorf("want ≥ 2 tracking events, got %d", len(cancelled.TrackingEvents))
	}

	// Cancel novamente deve dar 409
	w = do(r, http.MethodPatch, "/api/v1/orders/"+created.ID+"/cancel", newUser, nil)
	if w.Code != http.StatusConflict {
		t.Errorf("cancel de cancelled status = %d, esperado 409", w.Code)
	}

	// Limpa teste data
	db.Exec("DELETE FROM orders WHERE user_id = $1", newUser)
}

func TestOrders_Create_BadRequest(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	// items vazios → validação falha
	w := do(r, http.MethodPost, "/api/v1/orders", testUserID, map[string]any{
		"paymentMethod": "pix",
		"items":         []any{},
		"address": map[string]any{"street": "X", "number": "1", "neighborhood": "N", "city": "C", "state": "SP", "cep": "00000-000"},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d want 400", w.Code)
	}

	// método inválido
	w = do(r, http.MethodPost, "/api/v1/orders", testUserID, map[string]any{
		"paymentMethod": "bitcoin",
		"items":         []map[string]any{{"productId": "x", "name": "X", "icon": "⚒", "sellerId": "s", "sellerName": "S", "quantity": 1, "unitPrice": 10.00}},
		"address":       map[string]any{"street": "X", "number": "1", "neighborhood": "N", "city": "C", "state": "SP", "cep": "00000-000"},
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("paymentMethod inválido status = %d want 400", w.Code)
	}
}
