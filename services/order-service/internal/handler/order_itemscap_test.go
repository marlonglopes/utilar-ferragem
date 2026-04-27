// O3-M1: cap em items array (max=100). Regressão pra prevenir DoS via array
// gigante.
package handler_test

import (
	"net/http"
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
