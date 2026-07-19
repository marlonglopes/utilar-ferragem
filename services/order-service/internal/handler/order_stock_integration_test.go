// Testes end-to-end de POST /orders com as duas defesas novas:
// validação de estoque e frete server-side.
//
// Precisam de DB (mesmo padrão dos outros integration tests). Skipam sem banco.
package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/order-service/internal/shipping"
)

// stubReserver registra as chamadas de reserva sem falar com o catalog.
type stubReserver struct {
	reserveErr error
	reserved   []string
	released   []string
	committed  []string
}

func (s *stubReserver) Reserve(ctx context.Context, orderID string, items []catalogclient.ReservationItem, ttl time.Duration) error {
	if s.reserveErr != nil {
		return s.reserveErr
	}
	s.reserved = append(s.reserved, orderID)
	return nil
}
func (s *stubReserver) Commit(ctx context.Context, orderID string) error {
	s.committed = append(s.committed, orderID)
	return nil
}
func (s *stubReserver) Release(ctx context.Context, orderID string) error {
	s.released = append(s.released, orderID)
	return nil
}

// fullRouter monta o handler com catálogo, reserva e frete ligados.
func fullRouter(db *sql.DB, cat handler.CatalogLookup, stock handler.StockReserver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())

	h := handler.NewOrderHandler(db, cat, true).WithShipping(shipping.NewStore(db))
	if stock != nil {
		h = h.WithStock(stock)
	}

	api := r.Group("/api/v1", handler.RequireUser("test-secret", true))
	api.POST("/orders", h.Create)
	api.POST("/shipping/quote", handler.NewShippingHandler(shipping.NewStore(db)).Quote)
	return r
}

func orderPayload(productID string, qty int, cep string, shippingCost float64) map[string]any {
	return map[string]any{
		"paymentMethod": "pix",
		"shippingCost":  shippingCost,
		"items": []map[string]any{
			{
				"productId": productID, "name": "Produto", "icon": "⚒",
				"sellerId": "s1", "sellerName": "Seller 1",
				"quantity": qty, "unitPrice": 100.00,
			},
		},
		"address": map[string]any{
			"street": "Rua Teste", "number": "42", "neighborhood": "Centro",
			"city": "São Paulo", "state": "SP", "cep": cep,
		},
	}
}

func ratesReady(t *testing.T, db *sql.DB) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM shipping_rates WHERE active`).Scan(&n); err != nil {
		t.Skipf("shipping_rates not ready (run migrations): %v", err)
	}
	if n == 0 {
		t.Skip("no shipping rates seeded — run migration 002")
	}
}

const stockProductID = "00000000-0000-0000-0000-000000000901"

// REGRESSÃO: pedido acima do estoque tem que ser recusado com 409 +
// code=insufficient_stock, dizendo qual item e quanto há.
func TestCreate_RejectsOrderAboveStock(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 1}}
	r := fullRouter(db, cat, nil)

	user := "user-stock-reject"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 999, "01310-100", 0))
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Code    string `json:"code"`
		Details struct {
			Requested int `json:"requested"`
			Available int `json:"available"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Code != "insufficient_stock" {
		t.Errorf("code = %q; queria insufficient_stock", env.Code)
	}
	if env.Details.Requested != 999 || env.Details.Available != 1 {
		t.Errorf("details = %+v; deveria dizer pedido 999 / disponível 1", env.Details)
	}

	// Nada pode ter sido gravado.
	var n int
	db.QueryRow("SELECT count(*) FROM orders WHERE user_id=$1", user).Scan(&n)
	if n != 0 {
		t.Errorf("pedido recusado não pode gravar nada, veio %d linhas", n)
	}
}

// REGRESSÃO CRÍTICA: mandar shippingCost:0 não pode mais dar frete grátis.
// O servidor recalcula a partir da tabela e do CEP.
func TestCreate_IgnoresClientShippingCost(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 50}}
	r := fullRouter(db, cat, nil)

	user := "user-ship-zero"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	// Capital SP, 1 item, subtotal 100 (abaixo do limiar de frete grátis de 299).
	// Tabela: standard = 19.90 + 2.50*1 = 22.40.
	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 1, "01310-100", 0))
	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		ShippingCost    float64 `json:"shippingCost"`
		ShippingService string  `json:"shippingService"`
		Subtotal        float64 `json:"subtotal"`
		Total           float64 `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)

	if got.ShippingCost == 0 {
		t.Fatal("mandar shippingCost:0 não pode resultar em frete zero — este era O buraco")
	}
	if got.ShippingCost != 22.40 {
		t.Errorf("shippingCost = %.2f; queria 22.40 (tabela: capital, 1 item)", got.ShippingCost)
	}
	if got.Total != got.Subtotal+got.ShippingCost {
		t.Errorf("total (%.2f) != subtotal (%.2f) + frete (%.2f)", got.Total, got.Subtotal, got.ShippingCost)
	}
	if got.ShippingService != "standard" {
		t.Errorf("shippingService = %q; queria standard", got.ShippingService)
	}
}

// Frete grátis acima do limiar continua valendo — e vem do servidor.
func TestCreate_AppliesFreeShippingFromTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 50}}
	r := fullRouter(db, cat, nil)

	user := "user-ship-free"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	// 4 × R$100 = R$400 > R$299 → standard grátis na capital.
	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 4, "01310-100", 999.00))
	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		ShippingCost float64 `json:"shippingCost"`
		Total        float64 `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)

	if got.ShippingCost != 0 {
		t.Errorf("acima de R$299 o frete deveria ser 0, veio %.2f", got.ShippingCost)
	}
	// E o valor inflado que o cliente mandou (999) foi ignorado.
	if got.Total != 400.00 {
		t.Errorf("total = %.2f; queria 400.00 (o shippingCost:999 do cliente deve ser ignorado)", got.Total)
	}
}

// CEP fora da área de entrega é recusado — nunca vira frete zero silencioso.
func TestCreate_RejectsUncoveredCEP(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 50}}
	r := fullRouter(db, cat, nil)

	user := "user-ship-nocover"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	// 00000-000 não cai em nenhuma faixa do seed (que começa em 01000-000).
	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 1, "00000-000", 0))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperava 422, veio %d: %s", w.Code, w.Body.String())
	}
}

// Quando a reserva atômica está ligada, um pedido criado com sucesso reserva o
// estoque e o pedido fica marcado como reservado.
func TestCreate_ReservesStock(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 50}}
	res := &stubReserver{}
	r := fullRouter(db, cat, res)

	user := "user-reserve-ok"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 2, "01310-100", 0))
	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body.String())
	}
	if len(res.reserved) != 1 {
		t.Errorf("esperava 1 reserva, veio %d", len(res.reserved))
	}
	if len(res.released) != 0 {
		t.Errorf("pedido criado com sucesso não deveria liberar estoque: %v", res.released)
	}

	var reserved bool
	var id string
	json.Unmarshal(w.Body.Bytes(), &struct {
		ID *string `json:"id"`
	}{&id})
	db.QueryRow("SELECT stock_reserved FROM orders WHERE id=$1", id).Scan(&reserved)
	if !reserved {
		t.Error("orders.stock_reserved deveria estar true")
	}
}

// Se o catalog recusar a reserva (corrida perdida entre a validação e a
// reserva), o pedido não é criado.
func TestCreate_ReservationConflictAbortsOrder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	cat := &stubCatalog{product: &catalogclient.Product{Name: "Produto", Price: 100.00, Stock: 50}}
	res := &stubReserver{reserveErr: &catalogclient.StockError{
		Shortage: catalogclient.Shortage{ProductID: stockProductID, Requested: 2, Available: 0},
	}}
	r := fullRouter(db, cat, res)

	user := "user-reserve-lost"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, orderPayload(stockProductID, 2, "01310-100", 0))
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}

	var n int
	db.QueryRow("SELECT count(*) FROM orders WHERE user_id=$1", user).Scan(&n)
	if n != 0 {
		t.Errorf("reserva recusada não pode deixar pedido gravado, veio %d", n)
	}
}

// Cotação de frete: o carrinho pede opções com CEP e recebe preço + prazo.
func TestShippingQuote_ReturnsOptions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	r := fullRouter(db, nil, nil)

	w := do(r, http.MethodPost, "/api/v1/shipping/quote", "user-quote", map[string]any{
		"cep": "01310-100", "subtotal": 150.00, "itemCount": 2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Options []struct {
			ServiceCode  string  `json:"serviceCode"`
			Cost         float64 `json:"cost"`
			DeliveryDays int     `json:"deliveryDays"`
		} `json:"options"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)

	if len(got.Options) < 2 {
		t.Fatalf("capital deveria ter standard + express, veio %d: %+v", len(got.Options), got.Options)
	}
	// Ordenado do mais barato pro mais caro.
	if got.Options[0].Cost > got.Options[1].Cost {
		t.Errorf("opções deveriam vir da mais barata pra mais cara: %+v", got.Options)
	}
	for _, o := range got.Options {
		if o.DeliveryDays <= 0 {
			t.Errorf("opção sem prazo: %+v", o)
		}
	}
}

func TestShippingQuote_RejectsInvalidCEP(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	r := fullRouter(db, nil, nil)
	w := do(r, http.MethodPost, "/api/v1/shipping/quote", "user-quote", map[string]any{
		"cep": "123", "subtotal": 100.00, "itemCount": 1,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("CEP inválido deveria dar 400, veio %d", w.Code)
	}
}

func TestShippingQuote_NoCoverage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ratesReady(t, db)

	r := fullRouter(db, nil, nil)
	w := do(r, http.MethodPost, "/api/v1/shipping/quote", "user-quote", map[string]any{
		"cep": "00000-000", "subtotal": 100.00, "itemCount": 1,
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("CEP sem cobertura deveria dar 422, veio %d", w.Code)
	}
}
