package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/authclient"
	"github.com/utilar/order-service/internal/handler"
)

// ============================================================================
// Testes de integração da venda de balcão.
//
// As REGRAS de autorização já têm teste puro em internal/balcao/authz_test.go
// (que roda sempre, sem infra). Estes aqui verificam a OUTRA metade: que o
// handler realmente chama aquelas regras, que o pedido de balcão grava sem
// endereço e que o desconto sai recalculado pelo servidor. Skipam sem banco.
// ============================================================================

// stubOperators substitui o auth-service. É por ele que entra o teto de
// desconto — o número que este serviço nunca deve aceitar do cliente.
type stubOperators struct {
	ops map[string]*authclient.Operator
}

func (s *stubOperators) GetOperator(_ context.Context, userID string) (*authclient.Operator, error) {
	op, ok := s.ops[userID]
	if !ok {
		return nil, authclient.ErrNotOperator
	}
	return op, nil
}

const (
	balcaoStoreA = "11111111-1111-4111-8111-111111111111"
	balcaoStoreB = "22222222-2222-4222-8222-222222222222"
)

func balcaoOperators() *stubOperators {
	return &stubOperators{ops: map[string]*authclient.Operator{
		"op-a": {UserID: "op-a", StoreID: balcaoStoreA, Level: "operator", DiscountCeilingPct: 12, Active: true},
		"op-b": {UserID: "op-b", StoreID: balcaoStoreB, Level: "operator", DiscountCeilingPct: 12, Active: true},
		"mgr-a": {UserID: "mgr-a", StoreID: balcaoStoreA, Level: "manager",
			DiscountCeilingPct: 100, CanApproveDiscount: true, Active: true},
	}}
}

func setupBalcaoRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	orderH := handler.NewOrderHandler(db, nil, true).WithOperators(balcaoOperators())
	r.Use(handler.RequestID())

	api := r.Group("/api/v1", handler.RequireUser("test-secret", true))
	api.POST("/orders", orderH.Create)
	api.GET("/orders/:id", orderH.Get)

	bal := r.Group("/api/v1/balcao", handler.RequireUser("test-secret", true))
	bal.GET("/approvals", orderH.ListPendingApprovals)
	bal.PATCH("/orders/:id/approve", orderH.Approve)
	bal.PATCH("/orders/:id/reject", orderH.Reject)
	return r
}

// actor é a identidade do request no fallback de dev (X-User-*).
type actor struct{ userID, role, storeID string }

var (
	operatorA = actor{"op-a", "store_operator", balcaoStoreA}
	operatorB = actor{"op-b", "store_operator", balcaoStoreB}
	managerA  = actor{"mgr-a", "store_operator", balcaoStoreA}
	plainUser = actor{"cli-1", "customer", ""}
)

func doAs(r *gin.Engine, method, path string, a actor, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)

	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-User-Id", a.userID)
	req.Header.Set("X-User-Role", a.role)
	if a.storeID != "" {
		req.Header.Set("X-Store-Id", a.storeID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func balcaoPayload(discountPct float64) map[string]any {
	return map[string]any{
		"channel":       "balcao",
		"paymentMethod": "pix",
		"discountPct":   discountPct,
		"customerName":  "Cliente Balcão",
		"customerPhone": "(11) 98888-7777",
		"items": []map[string]any{{
			"productId": "550e8400-e29b-41d4-a716-446655440000",
			"name":      "Furadeira", "icon": "🔧",
			"sellerId": "balcao", "sellerName": "Loja física",
			"quantity": 2, "unitPrice": 100.0,
		}},
	}
}

func decodeOrder(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta não é JSON: %v — body=%s", err, w.Body.String())
	}
	return out
}

// ---------------------------------------------------------------------------
// Pedido de balcão não tem endereço
// ---------------------------------------------------------------------------

func TestRegression_BalcaoOrderCreatedWithoutAddress(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(0))
	if w.Code != http.StatusCreated {
		t.Fatalf("venda de balcão sem endereço deveria ser criada, veio %d: %s", w.Code, w.Body.String())
	}
	order := decodeOrder(t, w)

	if order["channel"] != "balcao" {
		t.Errorf("channel = %v, esperado balcao", order["channel"])
	}
	// O endereço falso "Retirada no balcão" que o PDV mandava não existe mais:
	// o campo simplesmente não vem.
	if _, present := order["address"]; present {
		t.Errorf("pedido de balcão não deveria ter endereço: %v", order["address"])
	}
	if order["shippingCost"] != 0.0 {
		t.Errorf("retirada no balcão não tem frete, veio %v", order["shippingCost"])
	}
	if order["storeId"] != balcaoStoreA {
		t.Errorf("storeId = %v, esperado a loja do vínculo", order["storeId"])
	}
	if order["operatorId"] != "op-a" {
		t.Errorf("operatorId = %v, esperado op-a", order["operatorId"])
	}
}

func TestRegression_WebOrderStillRequiresAddress(t *testing.T) {
	// A obrigatoriedade saiu do `binding:"required"` para o handler. Se alguém
	// esquecer de recolocá-la lá, um pedido web nasceria sem endereço de entrega.
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	payload := balcaoPayload(0)
	payload["channel"] = "web"
	w := doAs(r, http.MethodPost, "/api/v1/orders", plainUser, payload)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pedido web sem endereço deveria dar 400, veio %d: %s", w.Code, w.Body.String())
	}
}

func TestRegression_BalcaoOrderRejectsShippingAddress(t *testing.T) {
	// Endereço em venda de balcão é recusado, não ignorado: aceitar em silêncio
	// deixaria o endereço falso do PDV chegar numa etiqueta de entrega.
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	payload := balcaoPayload(0)
	payload["address"] = map[string]any{
		"street": "Retirada no balcão", "number": "S/N", "neighborhood": "Loja física",
		"city": "São Paulo", "state": "SP", "cep": "00000-000",
	}
	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, payload)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("balcão com endereço deveria dar 400, veio %d: %s", w.Code, w.Body.String())
	}
}

func TestRegression_BalcaoOrderRequiresCustomerPhone(t *testing.T) {
	// A Appmax recusa a cobrança sem celular do pagador — falhar aqui é mais
	// barato que falhar na maquininha com o cliente no caixa.
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	payload := balcaoPayload(0)
	delete(payload, "customerPhone")
	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, payload)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("balcão sem telefone deveria dar 400, veio %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Regra 1 (handler): operador só cria pedido da própria loja
// ---------------------------------------------------------------------------

func TestRegression_HandlerBlocksOrderForAnotherStore(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	payload := balcaoPayload(0)
	payload["storeId"] = balcaoStoreB // operador A tentando vender na loja B

	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, payload)
	if w.Code == http.StatusCreated {
		t.Fatalf("operador da loja A não pode criar pedido na loja B — foi criado: %s", w.Body.String())
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404 (não confirmar existência da loja alheia), veio %d: %s", w.Code, w.Body.String())
	}
}

func TestRegression_CustomerCannotCreateBalcaoOrder(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	w := doAs(r, http.MethodPost, "/api/v1/orders", plainUser, balcaoPayload(0))
	if w.Code != http.StatusForbidden {
		t.Fatalf("cliente comum não abre venda de balcão, veio %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Regra 2 (handler): operador só vê pedidos da própria loja
// ---------------------------------------------------------------------------

func TestRegression_HandlerBlocksReadingAnotherStoreOrder(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	created := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(0))
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: criação falhou %d: %s", created.Code, created.Body.String())
	}
	orderID, _ := decodeOrder(t, created)["id"].(string)

	// Operador da loja B lendo pedido da loja A.
	w := doAs(r, http.MethodGet, "/api/v1/orders/"+orderID, operatorB, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("operador de outra loja não pode ler o pedido, veio %d: %s", w.Code, w.Body.String())
	}

	// O operador da própria loja lê normalmente.
	if w := doAs(r, http.MethodGet, "/api/v1/orders/"+orderID, operatorA, nil); w.Code != http.StatusOK {
		t.Fatalf("operador da própria loja deveria ler, veio %d: %s", w.Code, w.Body.String())
	}

	// E um cliente comum não lê pedido de balcão de ninguém.
	if w := doAs(r, http.MethodGet, "/api/v1/orders/"+orderID, plainUser, nil); w.Code != http.StatusNotFound {
		t.Fatalf("cliente não pode ler pedido de balcão alheio, veio %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Desconto: servidor recalcula e roteia para aprovação
// ---------------------------------------------------------------------------

func TestRegression_DiscountIsRecalculatedServerSide(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	// 10% sobre 2 × R$100 = R$20 de desconto, total R$180. Dentro do teto (12%).
	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(10))
	if w.Code != http.StatusCreated {
		t.Fatalf("criação falhou %d: %s", w.Code, w.Body.String())
	}
	order := decodeOrder(t, w)

	if order["discountAmount"] != 20.0 {
		t.Errorf("discountAmount = %v, esperado 20 (calculado pelo servidor)", order["discountAmount"])
	}
	if order["total"] != 180.0 {
		t.Errorf("total = %v, esperado 180", order["total"])
	}
	if order["approvalStatus"] != "not_required" {
		t.Errorf("10%% dentro do teto de 12%% não pede aprovação, veio %v", order["approvalStatus"])
	}
}

func TestRegression_DiscountOverCeilingBecomesPending(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(30)) // teto 12%
	if w.Code != http.StatusCreated {
		t.Fatalf("desconto acima do teto NÃO bloqueia a venda; veio %d: %s", w.Code, w.Body.String())
	}
	order := decodeOrder(t, w)
	if order["approvalStatus"] != "pending" {
		t.Fatalf("30%% acima do teto de 12%% deveria ficar pendente, veio %v", order["approvalStatus"])
	}

	// E o pedido aparece na fila do gerente da MESMA loja.
	q := doAs(r, http.MethodGet, "/api/v1/balcao/approvals", managerA, nil)
	if q.Code != http.StatusOK {
		t.Fatalf("fila de aprovação falhou %d: %s", q.Code, q.Body.String())
	}
	if !bytes.Contains(q.Body.Bytes(), []byte(order["id"].(string))) {
		t.Errorf("pedido pendente não apareceu na fila do gerente da loja")
	}
}

// ---------------------------------------------------------------------------
// Regra 3 (handler): não aprovar o próprio desconto
// ---------------------------------------------------------------------------

func TestRegression_HandlerBlocksSelfApproval(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	// O operador dá 30% (teto dele é 12%), o pedido cai na fila — e ele tenta
	// homologar a si mesmo. É o caminho completo da regra 3 pelo handler.
	created := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(30))
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: criação falhou %d: %s", created.Code, created.Body.String())
	}
	orderID := decodeOrder(t, created)["id"].(string)

	// O próprio vendedor tentando homologar o desconto que ele deu.
	w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve", operatorA, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("operador aprovou o próprio desconto — falha crítica de segurança")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("esperado 403 na auto-aprovação, veio %d: %s", w.Code, w.Body.String())
	}

	// Operador de outra loja também não aprova.
	if w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve", operatorB, nil); w.Code == http.StatusOK {
		t.Fatalf("operador de outra loja aprovou o desconto")
	}

	// O gerente da loja aprova.
	ok := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve", managerA, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("gerente da loja deveria aprovar, veio %d: %s", ok.Code, ok.Body.String())
	}
	if decodeOrder(t, ok)["approvalStatus"] != "approved" {
		t.Errorf("pedido deveria ficar approved")
	}

	// Aprovar duas vezes é conflito, não uma segunda linha de auditoria.
	if again := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve", managerA, nil); again.Code != http.StatusConflict {
		t.Errorf("re-aprovação deveria dar 409, veio %d", again.Code)
	}
}

func TestRegression_RejectRequiresReason(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	created := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(30))
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: criação falhou %d", created.Code)
	}
	orderID := decodeOrder(t, created)["id"].(string)

	if w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/reject", managerA, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("recusa sem motivo deveria dar 400, veio %d", w.Code)
	}

	w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/reject", managerA,
		map[string]any{"note": "margem insuficiente"})
	if w.Code != http.StatusOK {
		t.Fatalf("recusa com motivo deveria funcionar, veio %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Auditoria — desconto é dinheiro saindo e precisa ser rastreável até a pessoa
// ---------------------------------------------------------------------------

func TestRegression_DiscountLeavesAuditTrail(t *testing.T) {
	db := setupTestDB(t)
	r := setupBalcaoRouter(db)

	created := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(30))
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: criação falhou %d: %s", created.Code, created.Body.String())
	}
	orderID := decodeOrder(t, created)["id"].(string)

	var actor, action string
	var amount float64
	err := db.QueryRow(`
		SELECT actor_id, action, amount FROM balcao_audit_events
		WHERE order_id = $1 AND action LIKE 'discount%'
	`, orderID).Scan(&actor, &action, &amount)
	if err != nil {
		t.Fatalf("desconto sem linha de auditoria: %v", err)
	}
	if actor != "op-a" {
		t.Errorf("auditoria deve amarrar o desconto à pessoa: actor=%q", actor)
	}
	if amount != 60 {
		t.Errorf("valor auditado = %v, esperado 60 (30%% de 200)", amount)
	}

	// A aprovação também deixa rastro, com o aprovador — que não é o vendedor.
	if w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve", managerA, nil); w.Code != http.StatusOK {
		t.Fatalf("aprovação falhou %d", w.Code)
	}
	var approver string
	if err := db.QueryRow(`
		SELECT actor_id FROM balcao_audit_events WHERE order_id = $1 AND action = 'discount.approved'
	`, orderID).Scan(&approver); err != nil {
		t.Fatalf("aprovação sem linha de auditoria: %v", err)
	}
	if approver != "mgr-a" {
		t.Errorf("aprovador auditado = %q, esperado mgr-a", approver)
	}
}
