package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/payment-service/internal/handler"
)

// Integration test — requires PAYMENT_DB_URL env var pointing to a test DB.
// Run with: make test-integration

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	return db
}

func TestWebhookIdempotency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewWebhookHandler(db, "") // no secret in test
	r.POST("/webhooks/mp", h.HandleMercadoPago)

	payload := map[string]any{
		"type":   "payment",
		"action": "payment.updated",
		"data":   map[string]string{"id": "test-idempotency-001"},
	}
	body, _ := json.Marshal(payload)

	// First call — should succeed
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/webhooks/mp", bytes.NewReader(body)))
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: want 200, got %d", w1.Code)
	}

	// Second call with same psp_payment_id + event_type — must also return 200 (idempotent)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/webhooks/mp", bytes.NewReader(body)))
	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate call: want 200, got %d", w2.Code)
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewWebhookHandler(db, "my-secret")
	r.POST("/webhooks/mp", h.HandleMercadoPago)

	payload := []byte(`{"type":"payment","action":"payment.updated","data":{"id":"abc"}}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/mp", bytes.NewReader(payload))
	req.Header.Set("x-signature", "invalidsig")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}
