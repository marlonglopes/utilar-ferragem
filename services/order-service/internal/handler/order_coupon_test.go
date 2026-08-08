package handler_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"testing"

	"github.com/utilar/order-service/internal/catalogclient"
)

// Cupons no Create (web), server-authoritative. Exige Postgres :5437 com a
// migration 008. Asserções pelo discountAmount (independe do frete).

func seedCoupon(t *testing.T, db *sql.DB, code, ctype string, value, minSub float64, maxUses *int) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM coupons WHERE code = $1", code)
	if _, err := db.Exec(`
		INSERT INTO coupons (code, type, value, min_subtotal, max_uses, active)
		VALUES ($1,$2,$3,$4,$5,true)`, code, ctype, value, minSub, maxUses); err != nil {
		t.Skipf("coupons table not ready (migration 008?): %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM coupons WHERE code = $1", code) })
}

func webOrderPayload(unitPrice float64, coupon string) map[string]any {
	p := tamperPayload(unitPrice) // reusa: web + address + 1 item
	if coupon != "" {
		p["couponCode"] = coupon
	}
	return p
}

type orderResp struct {
	ID             string  `json:"id"`
	Total          float64 `json:"total"`
	Subtotal       float64 `json:"subtotal"`
	ShippingCost   float64 `json:"shippingCost"`
	DiscountAmount float64 `json:"discountAmount"`
}

func webCatalog() *stubCatalog {
	return &stubCatalog{product: &catalogclient.Product{Name: "P", Price: 200.00, Stock: 500}}
}

func TestCreate_CouponPercent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	seedCoupon(t, db, "OBRA10", "percent", 10, 0, nil)
	r := setupRouterWithCatalog(db, webCatalog())
	user := "cli-cup-pct"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, webOrderPayload(200, "obra10"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got orderResp
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.DiscountAmount != 20.0 { // 10% de 200
		t.Fatalf("desconto=%v, esperado 20.00", got.DiscountAmount)
	}
	// total = subtotal - desconto + frete (frete lido da resposta → robusto).
	want := round2test(got.Subtotal - got.DiscountAmount + got.ShippingCost)
	if got.Total != want {
		t.Fatalf("total=%v, esperado %v (subtotal %v - desc %v + frete %v)",
			got.Total, want, got.Subtotal, got.DiscountAmount, got.ShippingCost)
	}
}

func TestCreate_CouponFixed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	seedCoupon(t, db, "MENOS50", "fixed", 50, 0, nil)
	r := setupRouterWithCatalog(db, webCatalog())
	user := "cli-cup-fix"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	w := do(r, http.MethodPost, "/api/v1/orders", user, webOrderPayload(200, "MENOS50"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got orderResp
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.DiscountAmount != 50.0 {
		t.Fatalf("desconto=%v, esperado 50.00", got.DiscountAmount)
	}
}

func TestCreate_CouponInvalidOrMinNotMet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	// cupom exige pedido mínimo de 1000; subtotal é 200 → 422.
	seedCoupon(t, db, "SO1000", "percent", 10, 1000, nil)
	r := setupRouterWithCatalog(db, webCatalog())
	user := "cli-cup-min"
	defer db.Exec("DELETE FROM orders WHERE user_id = $1", user)

	// pedido mínimo não atingido → 422, nada gravado.
	if w := do(r, http.MethodPost, "/api/v1/orders", user, webOrderPayload(200, "SO1000")); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("min não atingido: status=%d, esperado 422", w.Code)
	}
	// cupom inexistente → 422.
	if w := do(r, http.MethodPost, "/api/v1/orders", user, webOrderPayload(200, "NAOEXISTE")); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cupom inexistente: status=%d, esperado 422", w.Code)
	}
}

// REGRESSÃO/corrida: cupom max_uses=1, N pedidos simultâneos → exatamente 1
// aplica; os outros são recusados (o UPDATE condicional na tx fecha a janela).
func TestCreate_CouponMaxUsesRace(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	one := 1
	seedCoupon(t, db, "UNICO1", "fixed", 10, 0, &one)
	r := setupRouterWithCatalog(db, webCatalog())
	const goroutines = 20
	defer db.Exec("DELETE FROM orders WHERE user_id LIKE $1", "cup-race-%")

	var wg sync.WaitGroup
	codes := make([]int, goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			w := do(r, http.MethodPost, "/api/v1/orders",
				fmt.Sprintf("cup-race-%d", idx), webOrderPayload(200, "UNICO1"))
			codes[idx] = w.Code
		}(i)
	}
	close(start)
	wg.Wait()

	won := 0
	for _, code := range codes {
		if code == http.StatusCreated {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("cupom max_uses=1 aplicado %d vezes (esperado exatamente 1) — corrida", won)
	}
	var uses int
	db.QueryRow("SELECT uses FROM coupons WHERE code='UNICO1'").Scan(&uses)
	if uses != 1 {
		t.Fatalf("uses=%d, esperado 1", uses)
	}
}

func round2test(v float64) float64 { return math.Round(v*100) / 100 }
