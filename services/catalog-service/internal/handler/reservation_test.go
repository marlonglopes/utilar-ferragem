package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/catalog-service/internal/reservation"
)

// Testes de reserva de estoque — exigem Postgres :5436 com as migrations
// aplicadas. Skipam se o banco não responde.
//
// PORQUÊ estes testes precisam de banco de verdade: o que está sendo verificado
// É a atomicidade do `UPDATE ... WHERE stock >= n` sob concorrência real. Um
// mock provaria apenas que o mock funciona.

func reservationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CATALOG_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM stock_reservations`).Scan(&n); err != nil {
		t.Skipf("stock_reservations not ready (run migrations): %v", err)
	}
	return db
}

// seedProduct cria um produto de teste com o estoque pedido.
func seedProduct(t *testing.T, db *sql.DB, stock int) string {
	t.Helper()

	var categoryID, sellerID string
	if err := db.QueryRow(`SELECT id FROM categories LIMIT 1`).Scan(&categoryID); err != nil {
		t.Skipf("no categories seeded: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM sellers LIMIT 1`).Scan(&sellerID); err != nil {
		t.Skipf("no sellers seeded: %v", err)
	}

	slug := fmt.Sprintf("zzz-teste-reserva-%d", randSuffix(t, db))
	var id string
	err := db.QueryRow(`
		INSERT INTO products (slug, name, category_id, seller_id, price, icon, stock, status)
		VALUES ($1, 'Produto de teste (reserva)', $2, $3, 99.90, '🔧', $4, 'published')
		RETURNING id
	`, slug, categoryID, sellerID, stock).Scan(&id)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_reservations WHERE product_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM products WHERE id = $1`, id)
	})
	return id
}

func randSuffix(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(`SELECT (random() * 1000000000)::bigint`).Scan(&n); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return n
}

func reservationRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewReservationHandler(db)
	// devMode=true + X-User-Role: service — mesmo caminho de auth das rotas reais.
	g := r.Group("/api/v1/internal", handler.RequireRole("test-secret", true, "service", "admin"))
	g.POST("/reservations", h.Reserve)
	g.POST("/reservations/:orderId/commit", h.Commit)
	g.POST("/reservations/:orderId/release", h.Release)
	return r
}

func doReserve(r *gin.Engine, orderID, productID string, qty int) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{
		"orderId": orderID,
		"items":   []map[string]any{{"productId": productID, "quantity": qty}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "service")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doSettle(r *gin.Engine, orderID, action string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/internal/reservations/"+orderID+"/"+action, nil)
	req.Header.Set("X-User-Role", "service")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// stockOf devolve float64 desde que `products.stock` virou NUMERIC (migration
// 005) — o driver entrega NUMERIC como texto e um *int falha ao converter
// "6.000". As asserções seguem comparando com literais inteiros: as reservas
// continuam sendo de unidades inteiras.
func stockOf(t *testing.T, db *sql.DB, productID string) float64 {
	t.Helper()
	var s float64
	if err := db.QueryRow(`SELECT stock FROM products WHERE id=$1`, productID).Scan(&s); err != nil {
		t.Fatalf("read stock: %v", err)
	}
	return s
}

func cleanupOrders(t *testing.T, db *sql.DB, prefix string) {
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_reservations WHERE order_id LIKE $1`, prefix+"%")
	})
}

// ============================================================================
// TESTE OBRIGATÓRIO: N goroutines disputando a ÚLTIMA unidade.
// Exatamente 1 pode ganhar. Se dois ganharem, a loja vendeu o que não tem.
// ============================================================================
func TestReserve_ConcurrentRaceForLastUnit(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 1) // estoque: exatamente 1
	r := reservationRouter(db)
	cleanupOrders(t, db, "race-")

	const goroutines = 50

	var wg sync.WaitGroup
	results := make([]int, goroutines)
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // largada simultânea, pra maximizar a sobreposição real
			w := doReserve(r, fmt.Sprintf("race-%d", idx), productID, 1)
			results[idx] = w.Code
		}(i)
	}

	close(start)
	wg.Wait()

	won, lost, other := 0, 0, 0
	for _, code := range results {
		switch code {
		case http.StatusCreated:
			won++
		case http.StatusConflict:
			lost++
		default:
			other++
			t.Logf("status inesperado: %d", code)
		}
	}

	if won != 1 {
		t.Errorf("EXATAMENTE 1 goroutine deveria ganhar a última unidade, ganharam %d", won)
	}
	if lost != goroutines-1 {
		t.Errorf("esperava %d perdedoras com 409, veio %d (outros: %d)", goroutines-1, lost, other)
	}
	if s := stockOf(t, db, productID); s != 0 {
		t.Errorf("estoque final = %g; queria 0 (nunca negativo, nunca sobrando)", s)
	}

	// E exatamente uma reserva ativa foi criada.
	var active int
	_ = db.QueryRow(
		`SELECT count(*) FROM stock_reservations WHERE product_id=$1 AND status='active'`, productID,
	).Scan(&active)
	if active != 1 {
		t.Errorf("esperava 1 reserva ativa, veio %d", active)
	}
}

// Concorrência com estoque maior: a soma reservada nunca ultrapassa o estoque.
func TestReserve_ConcurrentNeverOversells(t *testing.T) {
	db := reservationDB(t)
	const stock = 10
	const goroutines = 40
	productID := seedProduct(t, db, stock)
	r := reservationRouter(db)
	cleanupOrders(t, db, "multi-")

	var wg sync.WaitGroup
	codes := make([]int, goroutines)
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			codes[idx] = doReserve(r, fmt.Sprintf("multi-%d", idx), productID, 1).Code
		}(i)
	}
	close(start)
	wg.Wait()

	won := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			won++
		}
	}
	if won != stock {
		t.Errorf("com estoque %d e %d tentativas de 1 unidade, esperava %d sucessos, veio %d",
			stock, goroutines, stock, won)
	}
	if s := stockOf(t, db, productID); s != 0 {
		t.Errorf("estoque final = %g; queria 0", s)
	}
}

func TestReserve_DecrementsStock(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "simple-")

	if w := doReserve(r, "simple-1", productID, 3); w.Code != http.StatusCreated {
		t.Fatalf("reserva deveria dar 201, veio %d: %s", w.Code, w.Body.String())
	}
	if s := stockOf(t, db, productID); s != 7 {
		t.Errorf("estoque = %g; queria 7", s)
	}
}

// REGRESSÃO: pedir mais do que existe tem que dar 409 com o detalhe do que
// faltou — não um erro genérico que obriga o cliente a adivinhar.
func TestReserve_InsufficientStockReturnsActionableDetail(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 2)
	r := reservationRouter(db)
	cleanupOrders(t, db, "short-")

	// 999 é o teto do binding (lte=999); acima disso o request nem chega ao
	// handler, e o que interessa aqui é o caminho de falta de saldo.
	w := doReserve(r, "short-1", productID, 999)
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		Code    string `json:"code"`
		Details struct {
			ProductID string `json:"productId"`
			Requested int    `json:"requested"`
			Available int    `json:"available"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Code != "insufficient_stock" {
		t.Errorf("code = %q; queria insufficient_stock", env.Code)
	}
	if env.Details.Requested != 999 || env.Details.Available != 2 {
		t.Errorf("details deveria dizer pedido/disponível, veio %+v", env.Details)
	}
	// E nada pode ter sido decrementado.
	if s := stockOf(t, db, productID); s != 2 {
		t.Errorf("reserva falha não pode mexer no estoque: %g", s)
	}
}

// All-or-nothing: se UM item do pedido falta, nenhum é reservado.
func TestReserve_AllOrNothing(t *testing.T) {
	db := reservationDB(t)
	okProduct := seedProduct(t, db, 100)
	shortProduct := seedProduct(t, db, 1)
	r := reservationRouter(db)
	cleanupOrders(t, db, "atomic-")

	body, _ := json.Marshal(map[string]any{
		"orderId": "atomic-1",
		"items": []map[string]any{
			{"productId": okProduct, "quantity": 5},
			{"productId": shortProduct, "quantity": 50},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "service")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}
	// O item que TINHA saldo não pode ter sido debitado.
	if s := stockOf(t, db, okProduct); s != 100 {
		t.Errorf("item com saldo foi debitado numa reserva que falhou: estoque %g, queria 100", s)
	}
}

// REGRESSÃO: retry da mesma reserva (timeout de rede, redelivery) não pode
// decrementar o estoque duas vezes.
func TestReserve_IsIdempotentPerOrder(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "idem-")

	for i := 0; i < 5; i++ {
		if w := doReserve(r, "idem-1", productID, 2); w.Code != http.StatusCreated {
			t.Fatalf("tentativa %d: esperava 201, veio %d: %s", i+1, w.Code, w.Body.String())
		}
	}
	if s := stockOf(t, db, productID); s != 8 {
		t.Errorf("5 reservas idênticas deveriam debitar 2 no total: estoque %g, queria 8", s)
	}
}

// Quantidades duplicadas do mesmo produto no payload são somadas — senão o
// unique index transformaria a segunda linha em no-op e reservaríamos menos do
// que o pedido precisa.
func TestReserve_SumsDuplicateLines(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "dup-")

	body, _ := json.Marshal(map[string]any{
		"orderId": "dup-1",
		"items": []map[string]any{
			{"productId": productID, "quantity": 3},
			{"productId": productID, "quantity": 4},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "service")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("esperava 201, veio %d: %s", w.Code, w.Body.String())
	}
	if s := stockOf(t, db, productID); s != 3 {
		t.Errorf("3+4 unidades deveriam debitar 7: estoque %g, queria 3", s)
	}
}

func TestRelease_ReturnsStock(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "rel-")

	doReserve(r, "rel-1", productID, 4)
	if s := stockOf(t, db, productID); s != 6 {
		t.Fatalf("setup: estoque %g, queria 6", s)
	}

	if w := doSettle(r, "rel-1", "release"); w.Code != http.StatusOK {
		t.Fatalf("release: %d %s", w.Code, w.Body.String())
	}
	if s := stockOf(t, db, productID); s != 10 {
		t.Errorf("release deveria devolver o saldo: estoque %g, queria 10", s)
	}
}

// REGRESSÃO: release duplicado não pode devolver estoque duas vezes — isso
// criaria unidades do nada.
func TestRelease_IsIdempotent(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "relx-")

	doReserve(r, "relx-1", productID, 4)
	for i := 0; i < 3; i++ {
		doSettle(r, "relx-1", "release")
	}
	if s := stockOf(t, db, productID); s != 10 {
		t.Errorf("3 releases deveriam devolver 4 no total: estoque %g, queria 10", s)
	}
}

// Commit é baixa definitiva: o estoque JÁ foi debitado na reserva e não volta.
func TestCommit_KeepsStockDebited(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "cmt-")

	doReserve(r, "cmt-1", productID, 4)
	if w := doSettle(r, "cmt-1", "commit"); w.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", w.Code, w.Body.String())
	}
	if s := stockOf(t, db, productID); s != 6 {
		t.Errorf("commit não devolve estoque: %g, queria 6", s)
	}

	// E um release posterior (bug de fluxo) não pode ressuscitar o estoque.
	doSettle(r, "cmt-1", "release")
	if s := stockOf(t, db, productID); s != 6 {
		t.Errorf("release após commit não pode devolver estoque: %g, queria 6", s)
	}
}

// O sweeper devolve o estoque de reservas vencidas — carrinho abandonado não
// pode prender estoque pra sempre.
func TestSweeper_ExpiredReservationReturnsStock(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "exp-")

	doReserve(r, "exp-1", productID, 4)
	if s := stockOf(t, db, productID); s != 6 {
		t.Fatalf("setup: estoque %g, queria 6", s)
	}

	// Envelhece a reserva por fora em vez de esperar 30 minutos.
	if _, err := db.Exec(
		`UPDATE stock_reservations SET expires_at = now() - interval '1 minute' WHERE order_id='exp-1'`,
	); err != nil {
		t.Fatalf("expire: %v", err)
	}

	n, err := reservation.NewSweeper(db).SweepOnce(t.Context())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n < 1 {
		t.Errorf("sweeper deveria ter liberado ao menos 1 reserva, liberou %d", n)
	}
	if s := stockOf(t, db, productID); s != 10 {
		t.Errorf("estoque não voltou após expiração: %g, queria 10", s)
	}

	// Rodar de novo não devolve outra vez.
	_, _ = reservation.NewSweeper(db).SweepOnce(t.Context())
	if s := stockOf(t, db, productID); s != 10 {
		t.Errorf("segundo sweep duplicou a devolução: %g, queria 10", s)
	}
}

// Reserva confirmada (committed) não é varrida pelo sweeper mesmo depois de
// vencida — a venda já aconteceu.
func TestSweeper_IgnoresCommittedReservations(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 10)
	r := reservationRouter(db)
	cleanupOrders(t, db, "expc-")

	doReserve(r, "expc-1", productID, 4)
	doSettle(r, "expc-1", "commit")
	_, _ = db.Exec(`UPDATE stock_reservations SET expires_at = now() - interval '1 hour' WHERE order_id='expc-1'`)

	if _, err := reservation.NewSweeper(db).SweepOnce(t.Context()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if s := stockOf(t, db, productID); s != 6 {
		t.Errorf("reserva confirmada não deveria ser devolvida: %g, queria 6", s)
	}
}

// As rotas internas não podem ser abertas: sem role, 401.
func TestReservationRoutes_RequireServiceRole(t *testing.T) {
	db := reservationDB(t)
	r := reservationRouter(db)

	body, _ := json.Marshal(map[string]any{
		"orderId": "unauth-1",
		"items":   []map[string]any{{"productId": "x", "quantity": 1}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("sem role deveria dar 401, veio %d", w.Code)
	}
}
