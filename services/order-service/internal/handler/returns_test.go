package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/order-service/internal/paymentclient"
)

// ============================================================================
// Devolução — testes de integração.
//
// As REGRAS puras (prazo de 7 dias, base legal, autorização, quantidade, trava
// de split) têm teste sem infra em internal/returns, e é lá que mora a garantia
// que roda sempre. Aqui verificamos a OUTRA metade, que só o handler responde:
//
//   * que o handler realmente CHAMA aquelas regras (o invasor toma 404 de
//     verdade, atravessando o middleware);
//   * que o ESTOQUE só é reposto no RECEBIMENTO, nunca na solicitação;
//   * que o ESTORNO só sai depois do recebimento, e sai UMA vez;
//   * que a trilha de auditoria grava pessoa, valor e ação.
//
// Skipam sem banco.
// ============================================================================

// stubRefundLedger registra os estornos que o handler mandaria ao payment-service.
type stubRefundLedger struct {
	mu    sync.Mutex
	posts []paymentclient.ReturnRefund
	err   error
}

func (s *stubRefundLedger) PostReturnRefund(_ context.Context, in paymentclient.ReturnRefund) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.posts = append(s.posts, in)
	return nil
}

func (s *stubRefundLedger) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

// stubRestocker registra as reposições de estoque.
type stubRestocker struct {
	mu    sync.Mutex
	calls []string
	items []catalogclient.RestockItem
}

func (s *stubRestocker) Restock(_ context.Context, returnID string, items []catalogclient.RestockItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, returnID)
	s.items = append(s.items, items...)
	return nil
}

func (s *stubRestocker) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func returnsRouter(db *sql.DB, led handler.RefundPoster, stock handler.StockRestorer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())

	h := handler.NewReturnHandler(db)
	if led != nil {
		h = h.WithRefundLedger(led)
	}
	if stock != nil {
		h = h.WithRestock(stock)
	}

	// Middleware de teste: injeta identidade pelos headers, como os outros
	// testes de integração deste pacote fazem.
	ident := func(c *gin.Context) {
		c.Set("user_id", c.GetHeader("X-Test-User"))
		c.Set("user_role", c.GetHeader("X-Test-Role"))
		c.Next()
	}

	r.POST("/api/v1/orders/:id/returns", ident, h.Create)
	r.GET("/api/v1/orders/:id/returns", ident, h.ListForOrder)
	r.GET("/api/v1/returns/:rid", ident, h.Get)
	r.PATCH("/api/v1/admin/returns/:rid/approve", ident, h.Approve)
	r.PATCH("/api/v1/admin/returns/:rid/reject", ident, h.Reject)
	r.PATCH("/api/v1/admin/returns/:rid/receive", ident, h.Receive)
	r.PATCH("/api/v1/admin/returns/:rid/refund", ident, h.Refund)
	return r
}

// seedReturnableOrder cria um pedido ENTREGUE hoje com 2 itens, pronto para
// devolução. Devolve (orderID, itemAID, itemBID) e limpa no fim.
func seedReturnableOrder(t *testing.T, db *sql.DB, userID string) (string, string, string) {
	t.Helper()
	var orderID string
	num := fmt.Sprintf("RET-%d", time.Now().UnixNano())
	err := db.QueryRow(`
		INSERT INTO orders (number, user_id, status, payment_method, subtotal,
		                    shipping_cost, total, paid_at, delivered_at)
		VALUES ($1,$2,'delivered','card',500,25,525, now() - interval '3 days', now() - interval '1 day')
		RETURNING id`, num, userID).Scan(&orderID)
	if err != nil {
		t.Skipf("não foi possível criar pedido de teste: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM return_audit_events WHERE order_id = $1`, orderID)
		_, _ = db.Exec(`DELETE FROM order_return_items WHERE return_id IN
		                 (SELECT id FROM order_returns WHERE order_id = $1)`, orderID)
		_, _ = db.Exec(`DELETE FROM order_returns WHERE order_id = $1`, orderID)
		_, _ = db.Exec(`DELETE FROM order_items WHERE order_id = $1`, orderID)
		_, _ = db.Exec(`DELETE FROM tracking_events WHERE order_id = $1`, orderID)
		_, _ = db.Exec(`DELETE FROM orders WHERE id = $1`, orderID)
	})

	var itemA, itemB string
	insert := `INSERT INTO order_items (order_id, product_id, name, icon, seller_id,
	                                    seller_name, quantity, unit_price)
	           VALUES ($1, gen_random_uuid(), $2, '🔧', 's1', 'Vendedor', $3, $4) RETURNING id`
	if err := db.QueryRow(insert, orderID, "Furadeira", 10, 30.00).Scan(&itemA); err != nil {
		t.Skipf("seed item A: %v", err)
	}
	if err := db.QueryRow(insert, orderID, "Serra", 2, 100.00).Scan(&itemB); err != nil {
		t.Skipf("seed item B: %v", err)
	}
	return orderID, itemA, itemB
}

func doReturnReq(t *testing.T, r *gin.Engine, method, path, user, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	req.Header.Set("X-Test-Role", role)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func returnIDOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Kind   string `json:"kind"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta ilegível: %v — %s", err, w.Body.String())
	}
	return out.ID
}

// ============================================================================

// TestRegression_EstoqueVoltaSoNoRecebimentoNaoNaSolicitacao.
//
// Modo de falha que previne: repor o estoque quando a devolução é SOLICITADA
// coloca à venda um produto que ainda está na casa do cliente. O sistema vende
// o que não tem, e quem descobre é a SEGUNDA venda — com o cliente já
// esperando a entrega.
func TestRegression_EstoqueVoltaSoNoRecebimentoNaoNaSolicitacao(t *testing.T) {
	db := setupTestDB(t)
	stock := &stubRestocker{}
	led := &stubRefundLedger{}
	r := returnsRouter(db, led, stock)

	orderID, itemA, _ := seedReturnableOrder(t, db, "cli-estoque")

	// 1. Solicitação — arrependimento, deferido na hora pela lei.
	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-estoque", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 2}}})
	if w.Code != http.StatusCreated {
		t.Fatalf("solicitar devolução: status = %d, body = %s", w.Code, w.Body.String())
	}
	rid := returnIDOf(t, w)

	if stock.count() != 0 {
		t.Fatal("ESTOQUE FOI REPOSTO NA SOLICITAÇÃO — o produto ainda está com o cliente")
	}
	if led.count() != 0 {
		t.Fatal("estorno lançado na solicitação — dinheiro saiu antes da mercadoria voltar")
	}

	// 2. Em trânsito (via aprovação já feita automaticamente). Ainda nada.
	if stock.count() != 0 {
		t.Fatal("estoque reposto antes do recebimento")
	}

	// 3. RECEBIMENTO — agora sim.
	w = doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/receive", "op-1", "operator", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("receber: status = %d, body = %s", w.Code, w.Body.String())
	}
	if stock.count() != 1 {
		t.Fatalf("reposições = %d, esperado 1 no recebimento", stock.count())
	}
	// E o dinheiro ainda NÃO saiu: recebimento não é estorno.
	if led.count() != 0 {
		t.Fatalf("lançamentos = %d — o estorno saiu no recebimento, sem decisão de estorno", led.count())
	}
}

// TestRegression_DinheiroSoSaiDepoisDoRecebimento — o estorno é recusado
// enquanto a mercadoria não foi conferida.
func TestRegression_DinheiroSoSaiDepoisDoRecebimento(t *testing.T) {
	db := setupTestDB(t)
	led := &stubRefundLedger{}
	r := returnsRouter(db, led, &stubRestocker{})

	orderID, itemA, _ := seedReturnableOrder(t, db, "cli-dinheiro")

	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-dinheiro", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 1}}})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	rid := returnIDOf(t, w)

	// Estorno ANTES do recebimento: recusado pela máquina de estados.
	w = doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/refund", "op-1", "operator", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("estorno antes do recebimento: status = %d (esperado 409) — "+
			"produto E dinheiro iriam para a mesma pessoa", w.Code)
	}
	if led.count() != 0 {
		t.Fatal("lançamento contábil saiu num estorno recusado")
	}

	// Recebe e estorna.
	if w := doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/receive", "op-1", "operator", nil); w.Code != http.StatusOK {
		t.Fatalf("receber: status = %d, %s", w.Code, w.Body.String())
	}
	if w := doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/refund", "op-1", "operator", nil); w.Code != http.StatusOK {
		t.Fatalf("estornar: status = %d, %s", w.Code, w.Body.String())
	}
	if led.count() != 1 {
		t.Fatalf("lançamentos = %d, esperado 1", led.count())
	}
	if got := led.posts[0].AmountBRL; got != 30.00 {
		t.Fatalf("valor estornado = %v, esperado 30.00 (1 × 30,00)", got)
	}
	if !led.posts[0].Partial {
		t.Fatal("devolução de 1 de 12 unidades não foi marcada como parcial no lançamento")
	}
	if led.posts[0].ReturnID != rid {
		t.Fatalf("chave de idempotência = %q, esperado o id da DEVOLUÇÃO", led.posts[0].ReturnID)
	}
}

// TestRegression_EstornoNaoSaiDuasVezes.
//
// Modo de falha que previne: dois cliques no botão de estornar. Sem o FOR
// UPDATE e a máquina de estados, os dois leriam `received`, os dois passariam,
// e o dinheiro sairia em dobro.
func TestRegression_EstornoNaoSaiDuasVezes(t *testing.T) {
	db := setupTestDB(t)
	led := &stubRefundLedger{}
	r := returnsRouter(db, led, &stubRestocker{})

	orderID, _, itemB := seedReturnableOrder(t, db, "cli-duplo")

	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-duplo", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemB, "quantity": 1}}})
	rid := returnIDOf(t, w)
	doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/receive", "op-1", "operator", nil)

	first := doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/refund", "op-1", "operator", nil)
	second := doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/refund", "op-1", "operator", nil)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status = %d / %d, esperado 200 nos dois (o segundo é retry idempotente)",
			first.Code, second.Code)
	}

	// O lançamento é RETENTADO (é idempotente do outro lado, e é assim que se
	// recupera um estorno cujo lançamento falhou), mas o ESTADO só muda uma vez
	// e a trilha só ganha uma linha de estorno.
	var linhas int
	if err := db.QueryRow(`
		SELECT count(*) FROM return_audit_events
		 WHERE return_id = $1 AND action = 'return.refunded'`, rid).Scan(&linhas); err != nil {
		t.Fatalf("consulta à trilha: %v", err)
	}
	if linhas != 1 {
		t.Fatalf("linhas de 'return.refunded' na trilha = %d, esperado 1 — "+
			"o estorno foi registrado duas vezes", linhas)
	}
}

// TestRegression_ClienteNaoAbreDevolucaoDePedidoAlheio — IDOR ponta a ponta,
// atravessando o handler.
func TestRegression_ClienteNaoAbreDevolucaoDePedidoAlheio(t *testing.T) {
	db := setupTestDB(t)
	r := returnsRouter(db, &stubRefundLedger{}, &stubRestocker{})

	orderID, itemA, _ := seedReturnableOrder(t, db, "dono-legitimo")

	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "invasor", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 1}}})

	// 404 e não 403: 403 confirmaria que o pedido existe e transformaria a rota
	// num enumerador de pedidos alheios.
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404 — IDOR na rota de devolução (estorno para a conta errada)", w.Code)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM order_returns WHERE order_id = $1`, orderID).Scan(&n); err != nil {
		t.Fatalf("consulta: %v", err)
	}
	if n != 0 {
		t.Fatal("uma devolução foi criada por quem não é dono do pedido")
	}
}

// TestClienteNaoEstornaAPropriaDevolucao — aprovar/estornar a própria
// devolução é o equivalente de aprovar o próprio desconto no balcão.
func TestClienteNaoEstornaAPropriaDevolucao(t *testing.T) {
	db := setupTestDB(t)
	led := &stubRefundLedger{}
	r := returnsRouter(db, led, &stubRestocker{})

	orderID, itemA, _ := seedReturnableOrder(t, db, "cli-esperto")
	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-esperto", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 1}}})
	rid := returnIDOf(t, w)

	for _, rota := range []string{"receive", "refund", "approve"} {
		w := doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/"+rota,
			"cli-esperto", "customer", nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("rota %s com papel customer: status = %d, esperado 403", rota, w.Code)
		}
	}
	if led.count() != 0 {
		t.Fatal("o cliente conseguiu disparar o próprio estorno")
	}
}

// TestDevolucaoParcialEstornaSoOItemDevolvido — ponta a ponta do caso "compra
// 10, devolve 1".
func TestDevolucaoParcialEstornaSoOItemDevolvido(t *testing.T) {
	db := setupTestDB(t)
	led := &stubRefundLedger{}
	stock := &stubRestocker{}
	r := returnsRouter(db, led, stock)

	orderID, itemA, _ := seedReturnableOrder(t, db, "cli-parcial")

	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-parcial", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 3}}})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, %s", w.Code, w.Body.String())
	}

	var out struct {
		ID             string  `json:"id"`
		RefundAmount   float64 `json:"refundAmount"`
		RefundShipping float64 `json:"refundShipping"`
		RefundTotal    float64 `json:"refundTotal"`
		Kind           string  `json:"kind"`
		Status         string  `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)

	if out.RefundAmount != 90.00 {
		t.Fatalf("refundAmount = %v, esperado 90.00 (3 × 30,00)", out.RefundAmount)
	}
	// ⚠️ Frete NÃO volta na parcial: a entrega dos demais itens aconteceu.
	if out.RefundShipping != 0 {
		t.Fatalf("refundShipping = %v, esperado 0 na devolução parcial", out.RefundShipping)
	}
	if out.Kind != "regret" || out.Status != "approved" {
		t.Fatalf("kind/status = %s/%s, esperado regret/approved (art. 49, deferimento automático)",
			out.Kind, out.Status)
	}

	doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+out.ID+"/receive", "op-1", "operator", nil)

	// Só as 3 unidades voltam ao estoque, não as 10 compradas.
	if len(stock.items) != 1 || stock.items[0].Quantity != 3 {
		t.Fatalf("itens repostos = %+v, esperado 3 unidades do item devolvido", stock.items)
	}
}

// TestTrilhaDeAuditoriaGravaPessoaEValor — estorno é dinheiro saindo por
// decisão humana; sem rastro até a pessoa não pode acontecer.
func TestTrilhaDeAuditoriaGravaPessoaEValor(t *testing.T) {
	db := setupTestDB(t)
	r := returnsRouter(db, &stubRefundLedger{}, &stubRestocker{})

	orderID, _, itemB := seedReturnableOrder(t, db, "cli-trilha")
	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-trilha", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemB, "quantity": 1}}})
	rid := returnIDOf(t, w)

	doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/receive", "conferente-7", "operator", nil)
	doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/refund", "gerente-3", "admin", nil)

	var actor string
	var amount float64
	err := db.QueryRow(`
		SELECT actor_id, amount FROM return_audit_events
		 WHERE return_id = $1 AND action = 'return.refunded'`, rid).Scan(&actor, &amount)
	if err != nil {
		t.Fatalf("a trilha do estorno não foi gravada: %v", err)
	}
	if actor != "gerente-3" {
		t.Fatalf("actor = %q, esperado 'gerente-3' — o estorno ficou sem responsável", actor)
	}
	if amount != 100.00 {
		t.Fatalf("amount = %v, esperado 100.00", amount)
	}
}

// TestArrependimentoNaoPodeSerRecusadoPeloEndpoint — a regra do art. 49 tem
// que valer atravessando o handler, não só na função pura.
func TestArrependimentoNaoPodeSerRecusadoPeloEndpoint(t *testing.T) {
	db := setupTestDB(t)
	r := returnsRouter(db, &stubRefundLedger{}, &stubRestocker{})

	orderID, itemA, _ := seedReturnableOrder(t, db, "cli-arrep")
	w := doReturnReq(t, r, "POST", "/api/v1/orders/"+orderID+"/returns", "cli-arrep", "customer",
		map[string]any{"items": []map[string]any{{"orderItemId": itemA, "quantity": 1}}})
	rid := returnIDOf(t, w)

	w = doReturnReq(t, r, "PATCH", "/api/v1/admin/returns/"+rid+"/reject", "op-1", "operator",
		map[string]any{"note": "cliente usou o produto"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, esperado 422 — um arrependimento foi RECUSADO. "+
			"Isso é multa do Procon. Body: %s", w.Code, w.Body.String())
	}
}
