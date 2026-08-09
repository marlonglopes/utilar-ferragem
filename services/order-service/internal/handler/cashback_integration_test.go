package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// Regras extras (mínimos + campanha) fazem round-trip pelo banco, e a taxa
// efetiva respeita a janela da campanha.
func TestCashback_RulesRoundTrip(t *testing.T) {
	db := dashDB(t)
	defer db.Close()
	ctx := context.Background()

	// Restaura o singleton ao padrão no fim (banco compartilhado).
	t.Cleanup(func() {
		db.Exec(`UPDATE cashback_config SET active=true, earn_rate_pct=5, redeem_max_pct=50,
			validity_days=90, min_earn_subtotal=0, min_redeem_subtotal=0,
			campaign_rate_pct=NULL, campaign_starts_at=NULL, campaign_ends_at=NULL WHERE id=1`)
	})

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	in := cashback.Config{
		Active: true, EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90,
		MinEarnSubtotal: 100, MinRedeemSubtotal: 50,
		CampaignRatePct: 12, CampaignStartsAt: &start, CampaignEndsAt: &end,
	}
	if err := cashback.SaveConfig(ctx, db, in, "admin-test"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := cashback.LoadConfig(ctx, db)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.MinEarnSubtotal != 100 || got.MinRedeemSubtotal != 50 || got.CampaignRatePct != 12 {
		t.Fatalf("regras não persistiram: %+v", got)
	}
	if got.CampaignStartsAt == nil || got.CampaignEndsAt == nil {
		t.Fatalf("datas da campanha não persistiram: %+v", got)
	}
	// Campanha ativa agora → taxa efetiva 12 (não a base 5).
	if r := cashback.EffectiveEarnRate(got, time.Now()); r != 12 {
		t.Fatalf("taxa efetiva na campanha = %v, quero 12", r)
	}
	// Abaixo do mínimo de acúmulo não credita, mesmo na campanha.
	if a := cashback.Earn(got, 99, time.Now()); a != 0 {
		t.Fatalf("abaixo do mín. de acúmulo: Earn=%v, quero 0", a)
	}
}

// Cashback por categoria: taxa de override persiste e o acúmulo por item aplica
// a taxa certa por categoria (distribuindo o pago proporcionalmente).
func TestCashback_CategoryEarn(t *testing.T) {
	db := dashDB(t)
	defer db.Close()
	ctx := context.Background()

	const cust = "cb-cust-category"
	cleanup := func() {
		db.Exec(`DELETE FROM cashback_entries WHERE customer_id=$1`, cust)
		db.Exec(`DELETE FROM cashback_lots WHERE customer_id=$1`, cust)
		db.Exec(`DELETE FROM cashback_category_rates WHERE category_id IN ('cbt-ferramentas','cbt-tintas')`)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Override: ferramentas 10%; tintas sem override (usa a base 5%).
	if err := cashback.SaveCategoryRate(ctx, db, "cbt-ferramentas", 10, "admin-test"); err != nil {
		t.Fatalf("SaveCategoryRate: %v", err)
	}
	rates, err := cashback.LoadCategoryRates(ctx, db)
	if err != nil {
		t.Fatalf("LoadCategoryRates: %v", err)
	}
	if rates["cbt-ferramentas"] != 10 {
		t.Fatalf("override não persistiu: %+v", rates)
	}

	cfg := cashback.Config{Active: true, EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90}
	items := []cashback.ItemLine{
		{CategoryID: "cbt-ferramentas", LineTotal: 100},
		{CategoryID: "cbt-tintas", LineTotal: 100},
	}
	// 10% de 100 + 5% de 100 = 15 (sem desconto: basisNet == gross == 200).
	got, err := cashback.CreditEarnItems(ctx, db, cfg, cust, "cb-cat-order", items, 200, rates)
	if err != nil || got != 15 {
		t.Fatalf("CreditEarnItems = %v (err=%v), quero 15", got, err)
	}
	if bal := balanceOf(t, db, cust); bal != 15 {
		t.Fatalf("saldo = %v, quero 15", bal)
	}

	// Remover o override → volta pra base.
	if err := cashback.DeleteCategoryRate(ctx, db, "cbt-ferramentas"); err != nil {
		t.Fatalf("DeleteCategoryRate: %v", err)
	}
	rates, _ = cashback.LoadCategoryRates(ctx, db)
	if _, ok := rates["cbt-ferramentas"]; ok {
		t.Fatalf("override não foi removido: %+v", rates)
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
