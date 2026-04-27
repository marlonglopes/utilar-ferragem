// O2-H5: OrderHandler.Create deve sobrescrever unitPrice tampered com o
// preço autoritativo retornado pelo catalog-service.
//
// Estes testes precisam de DB pra escrever o pedido (mesmo padrão dos outros
// integration tests). Skipam se DB não está disponível.
package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/utilar/order-service/internal/catalogclient"
)

// stubCatalog devolve um produto fixo (price autoritativo) ou um erro fixo.
type stubCatalog struct {
	product *catalogclient.Product
	err     error
	calls   int
}

func (s *stubCatalog) GetByID(ctx context.Context, productID string) (*catalogclient.Product, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.product == nil {
		return nil, catalogclient.ErrNotFound
	}
	// Devolve cópia com o ID solicitado pra não vazar mismatch.
	cp := *s.product
	cp.ID = productID
	return &cp, nil
}

const tamperedProductID = "00000000-0000-0000-0000-000000000777"

func tamperPayload(unitPrice float64) map[string]any {
	return map[string]any{
		"paymentMethod": "pix",
		"shippingCost":  0.0,
		"items": []map[string]any{
			{
				"productId":  tamperedProductID,
				"name":       "Produto Tamper",
				"icon":       "⚒",
				"sellerId":   "s1",
				"sellerName": "Seller 1",
				"quantity":   1,
				"unitPrice":  unitPrice, // cliente tenta enviar preço falso
			},
		},
		"address": map[string]any{
			"street": "Rua Teste", "number": "42", "neighborhood": "Centro",
			"city": "SP", "state": "SP", "cep": "01000-000",
		},
	}
}

// O2-H5: cliente envia unitPrice 0.01 num produto de 599.90 → o pedido fica
// gravado com 599.90 (autoritativo do catalog).
func TestCreate_PriceTamperBlocked_UsesCatalogPrice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	const authoritative = 599.90
	cat := &stubCatalog{product: &catalogclient.Product{
		Name:  "Produto Tamper",
		Price: authoritative,
		Stock: 10,
	}}
	r := setupRouterWithCatalog(db, cat)

	user := fmt.Sprintf("user-tamper-%d", 1001)
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, tamperPayload(0.01))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		ID    string  `json:"id"`
		Total float64 `json:"total"`
		Items []struct {
			UnitPrice float64 `json:"unitPrice"`
			ProductID string  `json:"productId"`
		} `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)

	if cat.calls != 1 {
		t.Errorf("catalog.GetByID chamado %d vezes, esperado 1", cat.calls)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, esperado 1", len(got.Items))
	}
	if got.Items[0].UnitPrice != authoritative {
		t.Errorf("unitPrice gravado = %.2f, esperado %.2f (catalog)", got.Items[0].UnitPrice, authoritative)
	}
	if got.Total != authoritative {
		t.Errorf("total = %.2f, esperado %.2f", got.Total, authoritative)
	}
}

// O2-H5: produto não existe → 400 e nada é gravado.
func TestCreate_ProductNotFound_Returns400(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cat := &stubCatalog{err: catalogclient.ErrNotFound}
	r := setupRouterWithCatalog(db, cat)

	w := do(r, http.MethodPost, "/api/v1/orders", "user-nf-1", tamperPayload(10.00))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, esperado 400", w.Code, w.Body.String())
	}
}

// O2-H5: catalog upstream error → 502.
func TestCreate_CatalogUpstreamErr_Returns502(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cat := &stubCatalog{err: errors.New("connection refused: " + catalogclient.ErrUpstream.Error())}
	r := setupRouterWithCatalog(db, cat)

	w := do(r, http.MethodPost, "/api/v1/orders", "user-up-1", tamperPayload(10.00))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s, esperado 502", w.Code, w.Body.String())
	}
}

// O2-H5: preço dentro de tolerância (1 centavo) NÃO é considerado tamper —
// passa adiante usando o do catalog mesmo assim.
func TestCreate_PriceWithinTolerance_NoWarn(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cat := &stubCatalog{product: &catalogclient.Product{Price: 100.00, Stock: 10, Name: "X"}}
	r := setupRouterWithCatalog(db, cat)

	user := "user-tol-1"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, tamperPayload(100.005)) // sub-centavo
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d", w.Code)
	}
}
