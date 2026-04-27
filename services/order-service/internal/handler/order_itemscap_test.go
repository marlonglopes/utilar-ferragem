// O3-M1: cap em items array (max=100). Regressão pra prevenir DoS via array
// gigante.
//
// L-ORDER-1: regressão — `status` query param fora do whitelist deve ser
// tratado como "all" sem invocar SQL injection ou comportamento surpresa.
package handler_test

import (
	"net/http"
	"net/url"
	"testing"
)

func makeItem(productID string) map[string]any {
	return map[string]any{
		"productId":  productID,
		"name":       "X",
		"icon":       "⚒",
		"sellerId":   "s1",
		"sellerName": "Seller",
		"quantity":   1,
		"unitPrice":  10.0,
	}
}

func TestOrders_List_StatusFilterArbitrario_Aceito(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	// L-ORDER-1: payload arbitrário no status, incluindo SQL injection attempt.
	// Esperado: handler trata como "all" (default) sem panicar nem injetar.
	for _, payload := range []string{
		"' OR 1=1 --",
		"unknown",
		"DROP TABLE orders",
	} {
		w := do(r, http.MethodGet, "/api/v1/orders?status="+url.QueryEscape(payload), testUserID, nil)
		if w.Code != http.StatusOK {
			t.Errorf("status=%q retornou %d, esperado 200", payload, w.Code)
		}
	}
}

func TestOrders_Create_TooManyItems_Returns400(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db)

	items := make([]map[string]any, 0, 101)
	for i := 0; i < 101; i++ {
		items = append(items, makeItem("p-x"))
	}
	w := do(r, http.MethodPost, "/api/v1/orders", "user-cap-1", map[string]any{
		"paymentMethod": "pix",
		"items":         items,
		"address": map[string]any{
			"street": "X", "number": "1", "neighborhood": "N",
			"city": "C", "state": "SP", "cep": "00000-000",
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, esperado 400 (101 items > max=100)", w.Code)
	}
}
