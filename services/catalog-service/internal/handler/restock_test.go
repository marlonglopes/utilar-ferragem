package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// Reposição de estoque por devolução — exige Postgres :5436 com as migrations.
// O que se verifica é dinheiro/mercadoria: repor o saldo certo, UMA vez.

func restockRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewReservationHandler(db)
	g := r.Group("/api/v1/internal", handler.RequireRole("test-secret", true, "service", "admin"))
	g.POST("/restock", h.Restock)
	return r
}

func doRestock(r *gin.Engine, returnID string, items ...map[string]any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{
		"returnId": returnID,
		"reason":   "customer_return",
		"items":    items,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/restock", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "service")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func cleanupRestocks(t *testing.T, db *sql.DB, prefix string) {
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_restocks WHERE return_id LIKE $1`, prefix+"%")
	})
}

func movementCount(t *testing.T, db *sql.DB, productID, reason string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM stock_movements WHERE product_id=$1 AND reason=$2`,
		productID, reason,
	).Scan(&n); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	return n
}

// Repor incrementa o saldo e deixa um movimento de estoque (o almoxarife vê a
// devolução no histórico).
func TestRestock_IncrementaSaldoERegistraMovimento(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 3)
	cleanupRestocks(t, db, "ret-inc-")
	r := restockRouter(db)

	w := doRestock(r, "ret-inc-1", map[string]any{"productId": productID, "quantity": 2})
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body.String())
	}
	if got := stockOf(t, db, productID); got != 5 {
		t.Fatalf("estoque esperado 5 (3+2), veio %v", got)
	}
	if n := movementCount(t, db, productID, "customer_return"); n != 1 {
		t.Fatalf("esperava 1 movimento customer_return, veio %d", n)
	}
}

// REGRESSÃO (o bug que este item conserta): a rota não existia → o estoque
// devolvido nunca voltava (404). Agora existe e repõe.
func TestRegression_RestockRotaExiste(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 0)
	cleanupRestocks(t, db, "ret-exist-")
	r := restockRouter(db)

	w := doRestock(r, "ret-exist-1", map[string]any{"productId": productID, "quantity": 1})
	if w.Code == http.StatusNotFound {
		t.Fatalf("rota de restock 404 — o bug voltou (estoque de devolução não volta)")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", w.Code, w.Body.String())
	}
	if got := stockOf(t, db, productID); got != 1 {
		t.Fatalf("estoque esperado 1 (0+1), veio %v", got)
	}
}

// IDEMPOTÊNCIA por returnId: repor a MESMA devolução duas vezes não sobe o saldo
// duas vezes (repor em dobro = vender o que não existe).
func TestRestock_IdempotentePorReturnId(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 3)
	cleanupRestocks(t, db, "ret-idem-")
	r := restockRouter(db)

	item := map[string]any{"productId": productID, "quantity": 2}
	if w := doRestock(r, "ret-idem-1", item); w.Code != http.StatusOK {
		t.Fatalf("1ª reposição: %d %s", w.Code, w.Body.String())
	}
	w2 := doRestock(r, "ret-idem-1", item) // MESMO returnId → duplicata
	if w2.Code != http.StatusOK {
		t.Fatalf("2ª reposição (duplicata) devia ser 200, veio %d", w2.Code)
	}
	if got := stockOf(t, db, productID); got != 5 {
		t.Fatalf("idempotência falhou: estoque %v (esperado 5, não 7)", got)
	}
	if n := movementCount(t, db, productID, "customer_return"); n != 1 {
		t.Fatalf("duplicata não podia gerar 2º movimento; veio %d", n)
	}
}

// Devoluções parciais DIFERENTES (returnIds diferentes) repõem cada uma — a
// chave é a devolução, não o pedido.
func TestRestock_ReturnIdsDiferentesSomam(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 0)
	cleanupRestocks(t, db, "ret-multi-")
	r := restockRouter(db)

	doRestock(r, "ret-multi-1", map[string]any{"productId": productID, "quantity": 1})
	doRestock(r, "ret-multi-2", map[string]any{"productId": productID, "quantity": 3})
	if got := stockOf(t, db, productID); got != 4 {
		t.Fatalf("esperado 4 (1+3), veio %v", got)
	}
}

// Produto inexistente → 404 e NADA reposto (all-or-nothing): o item válido no
// mesmo pedido também não sobe, e o guarda de idempotência não é gravado.
func TestRestock_ProdutoInexistenteFazRollbackTotal(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 2)
	cleanupRestocks(t, db, "ret-miss-")
	r := restockRouter(db)

	w := doRestock(r, "ret-miss-1",
		map[string]any{"productId": productID, "quantity": 5},
		map[string]any{"productId": "00000000-0000-0000-0000-000000000000", "quantity": 1},
	)
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, veio %d: %s", w.Code, w.Body.String())
	}
	if got := stockOf(t, db, productID); got != 2 {
		t.Fatalf("rollback falhou: item válido subiu (estoque %v, esperado 2)", got)
	}
	// Guarda não gravado → um retry corrigido pode repor.
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM stock_restocks WHERE return_id=$1`, "ret-miss-1").Scan(&n)
	if n != 0 {
		t.Fatalf("guarda de idempotência não podia ter sido gravado no rollback (veio %d)", n)
	}
}

// CONCORRÊNCIA: N chamadas simultâneas da MESMA devolução → o saldo sobe UMA vez.
func TestRestock_ConcorrenteReponUmaVez(t *testing.T) {
	db := reservationDB(t)
	productID := seedProduct(t, db, 0)
	cleanupRestocks(t, db, "ret-conc-")
	r := restockRouter(db)

	const goroutines = 30
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			doRestock(r, "ret-conc-1", map[string]any{"productId": productID, "quantity": 2})
		}()
	}
	close(start)
	wg.Wait()

	if got := stockOf(t, db, productID); got != 2 {
		t.Fatalf("corrida repôs em dobro: estoque %v (esperado 2)", got)
	}
	if n := movementCount(t, db, productID, "customer_return"); n != 1 {
		t.Fatalf("corrida gerou %d movimentos (esperado 1)", n)
	}
}
