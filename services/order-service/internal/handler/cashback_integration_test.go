package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/cashback"
	"github.com/utilar/order-service/internal/handler"
)

// Caminho do dinheiro do cashback contra o banco: acúmulo idempotente, saldo,
// resgate com teto/saldo, estorno e histórico. As regras puras já têm unit test
// (cashback_test.go); aqui é o SQL (lotes, FIFO, idempotência).
func TestCashback_MoneyPath(t *testing.T) {
	db := dashDB(t)
	defer db.Close()

	const cust = "cb-cust-moneypath"
	cleanup := func() {
		db.Exec(`DELETE FROM cashback_entries WHERE customer_id=$1`, cust)
		db.Exec(`DELETE FROM cashback_lots WHERE customer_id=$1`, cust)
	}
	cleanup()
	t.Cleanup(cleanup)

	ctx := context.Background()
	cfg := cashback.Config{Active: true, EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90}

	// Acúmulo: 5% de 100 = 5.
	got, err := cashback.CreditEarn(ctx, db, cfg, cust, "cb-order-1", 100)
	if err != nil || got != 5 {
		t.Fatalf("CreditEarn = %v (err=%v), quero 5", got, err)
	}
	if bal := balanceOf(t, db, cust); bal != 5 {
		t.Fatalf("saldo = %v, quero 5", bal)
	}

	// IDEMPOTÊNCIA: mesmo pedido não credita de novo (replay do evento de pagamento).
	if got, _ := cashback.CreditEarn(ctx, db, cfg, cust, "cb-order-1", 100); got != 0 {
		t.Fatalf("CreditEarn duplicado = %v, quero 0", got)
	}
	if bal := balanceOf(t, db, cust); bal != 5 {
		t.Fatalf("saldo após replay = %v, quero 5 (não pode dobrar)", bal)
	}

	// Acúmulo maior: 5% de 2000 = 100. Saldo 105.
	if got, _ := cashback.CreditEarn(ctx, db, cfg, cust, "cb-order-2", 2000); got != 100 {
		t.Fatalf("CreditEarn 2000 = %v, quero 100", got)
	}

	// Resgate: pede 80, saldo 105, teto 50% de 200 = 100 → clamp 80; consome 80.
	want := cashback.ClampRedeem(cfg, 80, balanceOf(t, db, cust), 200)
	if want != 80 {
		t.Fatalf("ClampRedeem = %v, quero 80", want)
	}
	used, err := cashback.Redeem(ctx, db, cust, "cb-order-3", want)
	if err != nil || used != 80 {
		t.Fatalf("Redeem = %v (err=%v), quero 80", used, err)
	}
	if bal := balanceOf(t, db, cust); bal != 25 {
		t.Fatalf("saldo após resgate = %v, quero 25 (105-80)", bal)
	}

	// Estorno do pedido-2 (devolução total): zera o que sobrou daquele lote.
	// Após o resgate FIFO (vence primeiro), parte do lote-2 pode ter sido gasta;
	// Reverse zera o remaining atual do lote-2, seja qual for.
	var lot2Remaining float64
	db.QueryRow(`SELECT remaining FROM cashback_lots WHERE order_id='cb-order-2'`).Scan(&lot2Remaining)
	balBefore := balanceOf(t, db, cust)
	rev, err := cashback.Reverse(ctx, db, "cb-order-2")
	if err != nil || rev != lot2Remaining {
		t.Fatalf("Reverse = %v (err=%v), quero %v", rev, err, lot2Remaining)
	}
	if bal := balanceOf(t, db, cust); bal != round2test(balBefore-lot2Remaining) {
		t.Fatalf("saldo após estorno = %v, quero %v", bal, balBefore-lot2Remaining)
	}

	// Histórico tem earn + redeem + reverse.
	hist, err := cashback.History(ctx, db, cust, 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	kinds := map[string]bool{}
	for _, e := range hist {
		kinds[e.Kind] = true
	}
	for _, k := range []string{"earn", "redeem", "reverse"} {
		if !kinds[k] {
			t.Errorf("histórico sem entrada '%s': %+v", k, hist)
		}
	}
}

func balanceOf(t *testing.T, db *sql.DB, cust string) float64 {
	t.Helper()
	bal, err := cashback.BalanceFor(context.Background(), db, cust)
	if err != nil {
		t.Fatalf("BalanceFor: %v", err)
	}
	return bal
}

// GET /me/cashback devolve saldo + taxa vigente, escopado pelo user_id do JWT.
func TestCashback_MeEndpoint(t *testing.T) {
	db := dashDB(t)
	defer db.Close()

	const cust = "cb-cust-me"
	db.Exec(`DELETE FROM cashback_entries WHERE customer_id=$1`, cust)
	db.Exec(`DELETE FROM cashback_lots WHERE customer_id=$1`, cust)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM cashback_entries WHERE customer_id=$1`, cust)
		db.Exec(`DELETE FROM cashback_lots WHERE customer_id=$1`, cust)
	})

	cfg := cashback.Config{Active: true, EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90}
	cashback.CreditEarn(context.Background(), db, cfg, cust, "cb-me-order", 200) // 10

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewCashbackHandler(db)
	r.GET("/api/v1/me/cashback", func(c *gin.Context) { c.Set("user_id", cust); h.Me(c) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/me/cashback", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /me/cashback = %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Balance     float64 `json:"balance"`
		EarnRatePct float64 `json:"earnRatePct"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Balance != 10 {
		t.Fatalf("saldo no /me = %v, quero 10", out.Balance)
	}
}

// Config do cashback: admin lê e grava (liga/desliga + taxas).
func TestCashback_AdminConfig(t *testing.T) {
	db := dashDB(t)
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewCashbackHandler(db)
	r.GET("/api/v1/admin/cashback", h.GetConfig)
	r.PUT("/api/v1/admin/cashback", func(c *gin.Context) { c.Set("user_id", "admin-x"); h.UpdateConfig(c) })

	// Restaura o singleton ao padrão no fim (banco compartilhado).
	t.Cleanup(func() {
		db.Exec(`UPDATE cashback_config SET active=true, earn_rate_pct=5, redeem_max_pct=50, validity_days=90 WHERE id=1`)
	})

	// PUT muda a taxa.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/cashback",
		strings.NewReader(`{"active":true,"earnRatePct":8,"redeemMaxPct":40,"validityDays":60}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT config = %d %s", w.Code, w.Body.String())
	}

	// GET reflete.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/cashback", nil))
	var cfg struct {
		EarnRatePct  float64 `json:"earnRatePct"`
		RedeemMaxPct float64 `json:"redeemMaxPct"`
	}
	json.Unmarshal(w.Body.Bytes(), &cfg)
	if cfg.EarnRatePct != 8 || cfg.RedeemMaxPct != 40 {
		t.Fatalf("config após PUT = %+v, quero earn 8 / redeem 40", cfg)
	}

	// Percentual inválido (>100) → 400.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/cashback",
		strings.NewReader(`{"active":true,"earnRatePct":150,"redeemMaxPct":40,"validityDays":60}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("earnRatePct>100 = %d, quero 400", w.Code)
	}
}
