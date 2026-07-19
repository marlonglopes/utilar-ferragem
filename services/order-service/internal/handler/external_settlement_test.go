package handler_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/handler"
	"github.com/utilar/order-service/internal/paymentclient"
)

// ============================================================================
// Liquidação externa — testes de integração.
//
// As REGRAS puras (quem pode, NSU, idempotência) têm teste sem infra em
// internal/balcao/external_test.go, e é lá que mora a garantia que roda sempre.
// Aqui verificamos a OUTRA metade, que só o handler responde:
//   * que o handler realmente CHAMA aquelas regras (customer/anônimo tomam
//     403/401 de verdade, atravessando o middleware);
//   * que a trilha de auditoria é gravada com pessoa, NSU, loja e IP;
//   * que o lançamento contábil sai — e sai UMA vez por pedido;
//   * que a reserva de estoque vira baixa definitiva.
// Skipam sem banco.
// ============================================================================

// stubLedger registra os lançamentos que o handler mandaria ao payment-service.
type stubLedger struct {
	mu    sync.Mutex
	posts []paymentclient.ExternalSettlement
	err   error
}

func (s *stubLedger) PostExternalSettlement(_ context.Context, in paymentclient.ExternalSettlement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.posts = append(s.posts, in)
	return nil
}

func (s *stubLedger) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func settleRouter(db *sql.DB, led handler.LedgerPoster, stock handler.StockReserver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())

	h := handler.NewOrderHandler(db, nil, true).WithOperators(balcaoOperators())
	if led != nil {
		h = h.WithLedger(led)
	}
	if stock != nil {
		h = h.WithStock(stock)
	}

	api := r.Group("/api/v1", handler.RequireUser("test-secret", true))
	api.POST("/orders", h.Create)
	api.GET("/orders/:id", h.Get)

	bal := r.Group("/api/v1/balcao", handler.RequireUser("test-secret", true))
	bal.PATCH("/orders/:id/approve", h.Approve)
	bal.POST("/orders/:id/settle-external", h.SettleExternal)
	return r
}

// criaVendaBalcao cria uma venda de balcão e devolve o id.
func criaVendaBalcao(t *testing.T, r *gin.Engine, descontoPct float64) string {
	t.Helper()
	w := doAs(r, http.MethodPost, "/api/v1/orders", operatorA, balcaoPayload(descontoPct))
	if w.Code != http.StatusCreated {
		t.Fatalf("criar venda de balcão: status %d — %s", w.Code, w.Body.String())
	}
	order := decodeOrder(t, w)
	id, _ := order["id"].(string)
	if id == "" {
		t.Fatalf("pedido sem id: %s", w.Body.String())
	}
	return id
}

func settlePayload(nsu string) map[string]any {
	return map[string]any{"nsu": nsu, "brand": "visa", "authorizationCode": "A1B2C3"}
}

// nsuSeq gera NSUs distintos entre execuções.
//
// PORQUÊ não um valor fixo: o NSU é único por loja no banco (índice parcial), e
// o banco de teste é PERSISTENTE — um literal "004417" passaria na primeira
// execução e daria 409 em todas as seguintes. Teste que só passa em banco
// limpo é teste que vira flake e depois vira teste comentado.
var nsuSeq atomic.Int64

func novoNSU() string {
	return fmt.Sprintf("%d%04d", time.Now().UnixNano()%1_000_000_0, nsuSeq.Add(1))
}

// ---------------------------------------------------------------------------
// AUTORIZAÇÃO — o requisito central
// ---------------------------------------------------------------------------

func TestRegression_ClienteNaoLiquidaPedidoExternamente(t *testing.T) {
	// O risco central desta feature: um endpoint que diz "pagou" sem dinheiro
	// nenhum ter entrado. Se um customer conseguir chamá-lo, qualquer pessoa
	// com conta marca o próprio pedido como pago e leva mercadoria de graça.
	db := setupTestDB(t)
	led := &stubLedger{}
	r := settleRouter(db, led, nil)

	orderID := criaVendaBalcao(t, r, 0)

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		plainUser, settlePayload(novoNSU()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("customer liquidou pedido externamente: status %d — %s", w.Code, w.Body.String())
	}
	if led.count() != 0 {
		t.Fatalf("customer recusado mas lançamento contábil saiu: %d", led.count())
	}

	// E o pedido continua aguardando pagamento — nada mudou.
	got := decodeOrder(t, doAs(r, http.MethodGet, "/api/v1/orders/"+orderID, operatorA, nil))
	if got["status"] != "pending_payment" {
		t.Errorf("status = %v, esperado pending_payment intacto", got["status"])
	}
	if got["externalNsu"] != nil {
		t.Errorf("NSU gravado por um customer: %v", got["externalNsu"])
	}
}

func TestRegression_AnonimoNaoLiquidaPedidoExternamente(t *testing.T) {
	// Sem identidade nenhuma o middleware barra antes do handler. É a barreira
	// mais externa, e ela precisa existir de fato — não só na função pura.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)
	orderID := criaVendaBalcao(t, r, 0)

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		actor{}, settlePayload(novoNSU()))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anônimo liquidou pedido: status %d — %s", w.Code, w.Body.String())
	}
}

func TestRegression_OperadorDeOutraLojaNaoLiquida(t *testing.T) {
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)
	orderID := criaVendaBalcao(t, r, 0) // criado na loja A

	// 404 e não 403: confirmar que o pedido existe numa loja alheia já é
	// informação (dá para enumerar o parque de lojas e o volume de vendas).
	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorB, settlePayload(novoNSU()))
	if w.Code != http.StatusNotFound {
		t.Fatalf("operador da loja B liquidou venda da loja A: status %d — %s", w.Code, w.Body.String())
	}
}

func TestOperadorDaPropriaLojaLiquida(t *testing.T) {
	db := setupTestDB(t)
	led := &stubLedger{}
	r := settleRouter(db, led, nil)
	orderID := criaVendaBalcao(t, r, 0)

	// NSU digitado com separador, como vem no papel do comprovante.
	nsu := novoNSU()
	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(nsu[:4]+"-"+nsu[4:]))
	if w.Code != http.StatusOK {
		t.Fatalf("operador da própria loja não liquidou: status %d — %s", w.Code, w.Body.String())
	}

	order := decodeOrder(t, w)
	if order["status"] != "paid" {
		t.Errorf("status = %v, esperado paid", order["status"])
	}
	// O CONSERTO: a venda deixa de ser gravada como `card`.
	if order["paymentMethod"] != "external" {
		t.Errorf("paymentMethod = %v, esperado external — gravar `card` aqui é o bug de conciliação original",
			order["paymentMethod"])
	}
	// NSU normalizado: o separador digitado pelo operador não vai para o banco,
	// senão o financeiro não casa por igualdade exata com o extrato.
	if order["externalNsu"] != nsu {
		t.Errorf("externalNsu = %v, esperado %s normalizado", order["externalNsu"], nsu)
	}
	if order["externalSettledBy"] != "op-a" {
		t.Errorf("externalSettledBy = %v, esperado op-a", order["externalSettledBy"])
	}
	if order["externalSettledAt"] == nil {
		t.Error("externalSettledAt não gravado")
	}
}

func TestRegression_PedidoWebNaoEhLiquidadoPeloEndpoint(t *testing.T) {
	// Pedido do site nunca sai do PSP por este caminho, nem para um operador.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)

	var webOrderID string
	err := db.QueryRow(`SELECT id FROM orders WHERE channel = 'web' LIMIT 1`).Scan(&webOrderID)
	if err != nil {
		t.Skipf("sem pedido web no banco: %v", err)
	}

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+webOrderID+"/settle-external",
		operatorA, settlePayload(novoNSU()))
	if w.Code != http.StatusConflict {
		t.Fatalf("pedido WEB liquidado por fora: status %d — %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// IDEMPOTÊNCIA
// ---------------------------------------------------------------------------

func TestLiquidarDuasVezesNaoGeraDoisLancamentos(t *testing.T) {
	db := setupTestDB(t)
	led := &stubLedger{}
	r := settleRouter(db, led, nil)
	orderID := criaVendaBalcao(t, r, 0)

	path := "/api/v1/balcao/orders/" + orderID + "/settle-external"

	nsu := novoNSU()
	w1 := doAs(r, http.MethodPost, path, operatorA, settlePayload(nsu))
	if w1.Code != http.StatusOK {
		t.Fatalf("primeira liquidação: status %d — %s", w1.Code, w1.Body.String())
	}
	// Segunda chamada com o MESMO NSU: retry legítimo (rede caiu, operador
	// clicou duas vezes). Precisa devolver sucesso sem duplicar nada.
	w2 := doAs(r, http.MethodPost, path, operatorA, settlePayload(nsu))
	if w2.Code != http.StatusOK {
		t.Fatalf("retry com mesmo NSU: status %d — %s", w2.Code, w2.Body.String())
	}

	// Só UMA linha de auditoria de liquidação. Duas significariam duas
	// liquidações no histórico do pedido para uma venda só.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM balcao_audit_events
		WHERE order_id = $1 AND action = 'payment.settled_external'`, orderID).Scan(&n); err != nil {
		t.Fatalf("contar auditoria: %v", err)
	}
	if n != 1 {
		t.Errorf("linhas de auditoria de liquidação = %d, esperado 1", n)
	}

	// E só UM tracking event de pagamento — a máquina de estados não foi
	// atravessada duas vezes.
	if err := db.QueryRow(`SELECT count(*) FROM tracking_events
		WHERE order_id = $1 AND status = 'paid'`, orderID).Scan(&n); err != nil {
		t.Fatalf("contar tracking: %v", err)
	}
	if n != 1 {
		t.Errorf("tracking events 'paid' = %d, esperado 1", n)
	}
}

func TestSegundoNSUNoMesmoPedidoEhRecusado(t *testing.T) {
	// Dois comprovantes diferentes para a mesma venda: pode ser cobrança em
	// duplicidade no cartão do cliente. Recusa, e o NSU original (a prova do
	// primeiro comprovante) nunca é sobrescrito.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)
	orderID := criaVendaBalcao(t, r, 0)
	path := "/api/v1/balcao/orders/" + orderID + "/settle-external"

	nsu := novoNSU()
	if w := doAs(r, http.MethodPost, path, operatorA, settlePayload(nsu)); w.Code != http.StatusOK {
		t.Fatalf("primeira liquidação: %d — %s", w.Code, w.Body.String())
	}
	w := doAs(r, http.MethodPost, path, operatorA, settlePayload(novoNSU()))
	if w.Code != http.StatusConflict {
		t.Fatalf("segundo NSU no mesmo pedido: status %d — %s", w.Code, w.Body.String())
	}

	var gravado string
	if err := db.QueryRow(`SELECT external_nsu FROM orders WHERE id = $1`, orderID).Scan(&gravado); err != nil {
		t.Fatalf("ler nsu: %v", err)
	}
	if gravado != nsu {
		t.Errorf("NSU original foi sobrescrito: %q — é a prova do primeiro comprovante", nsu)
	}
}

func TestMesmoNSUEmDoisPedidosEhRecusado(t *testing.T) {
	// O mesmo comprovante liquidando dois pedidos significa uma venda cobrada
	// uma vez e baixada duas — a metade que "sobra" sai como mercadoria sem
	// contrapartida. Barrado pelo índice único parcial do banco.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)

	primeiro := criaVendaBalcao(t, r, 0)
	segundo := criaVendaBalcao(t, r, 0)
	nsu := novoNSU()

	if w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+primeiro+"/settle-external",
		operatorA, settlePayload(nsu)); w.Code != http.StatusOK {
		t.Fatalf("primeira liquidação: %d — %s", w.Code, w.Body.String())
	}
	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+segundo+"/settle-external",
		operatorA, settlePayload(nsu))
	if w.Code != http.StatusConflict {
		t.Fatalf("mesmo NSU em outro pedido: status %d — %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// AUDITORIA — obrigatória e fail-closed
// ---------------------------------------------------------------------------

func TestAuditoriaDaLiquidacaoGravaPessoaNSULojaEIP(t *testing.T) {
	// Liquidação externa sem rastro até a pessoa é o caminho natural de fraude
	// interna: alguém "liquida" um pedido, a mercadoria sai e não há a quem
	// perguntar. Cada campo aqui responde uma pergunta do dono da loja.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)
	orderID := criaVendaBalcao(t, r, 0)
	nsu := novoNSU()

	if w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(nsu)); w.Code != http.StatusOK {
		t.Fatalf("liquidar: %d — %s", w.Code, w.Body.String())
	}

	var actorID, actorRole, storeID, ip, requestID string
	var amount float64
	var newValue []byte
	err := db.QueryRow(`
		SELECT actor_id, actor_role, store_id, ip, request_id, amount, new_value
		FROM balcao_audit_events
		WHERE order_id = $1 AND action = 'payment.settled_external'`, orderID).
		Scan(&actorID, &actorRole, &storeID, &ip, &requestID, &amount, &newValue)
	if err != nil {
		t.Fatalf("trilha de liquidação não foi gravada: %v", err)
	}

	if actorID != "op-a" {
		t.Errorf("actor_id = %q, esperado quem liquidou", actorID)
	}
	if actorRole != "store_operator" {
		t.Errorf("actor_role = %q", actorRole)
	}
	if storeID != balcaoStoreA {
		t.Errorf("store_id = %q, esperado a loja da venda", storeID)
	}
	if ip == "" {
		t.Error("ip vazio: de onde partiu a liquidação é parte do rastro")
	}
	if requestID == "" {
		t.Error("request_id vazio: sem ele não dá para correlacionar com o lançamento contábil")
	}
	if amount <= 0 {
		t.Errorf("amount = %v: a trilha precisa registrar quanto foi declarado como recebido", amount)
	}
	if !bytesContains(newValue, nsu) {
		t.Errorf("new_value sem o NSU: %s", newValue)
	}
	if !bytesContains(newValue, "external") {
		t.Errorf("new_value sem o método: %s", newValue)
	}
}

func bytesContains(b []byte, sub string) bool {
	return len(b) > 0 && stringContains(string(b), sub)
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// LANÇAMENTO CONTÁBIL
// ---------------------------------------------------------------------------

func TestLiquidacaoLancaNoLivroComNSUeValorDoServidor(t *testing.T) {
	db := setupTestDB(t)
	led := &stubLedger{}
	r := settleRouter(db, led, nil)
	orderID := criaVendaBalcao(t, r, 0)
	nsu := novoNSU()

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(nsu))
	if w.Code != http.StatusOK {
		t.Fatalf("liquidar: %d — %s", w.Code, w.Body.String())
	}
	if led.count() != 1 {
		t.Fatalf("lançamentos contábeis = %d, esperado 1", led.count())
	}

	post := led.posts[0]
	if post.NSU != nsu {
		t.Errorf("NSU no lançamento = %q", post.NSU)
	}
	if post.SettledBy != "op-a" {
		t.Errorf("settledBy = %q — lançamento sem pessoa não serve de rastro", post.SettledBy)
	}
	if post.StoreID != balcaoStoreA {
		t.Errorf("storeId = %q", post.StoreID)
	}

	// O VALOR é o total do servidor, nunca um número que o cliente mandou —
	// não existe campo de valor no payload de liquidação, de propósito.
	order := decodeOrder(t, doAs(r, http.MethodGet, "/api/v1/orders/"+orderID, operatorA, nil))
	if total, ok := order["total"].(float64); !ok || post.AmountBRL != total {
		t.Errorf("valor lançado = %v, esperado o total do pedido (%v)", post.AmountBRL, order["total"])
	}
}

func TestFalhaNoLancamentoNaoDesfazLiquidacaoMasDeixaRastro(t *testing.T) {
	// O cliente pagou na maquininha e levou a mercadoria. Desfazer a
	// liquidação porque o serviço contábil está fora seria mentir sobre o que
	// aconteceu na loja. O que não pode é a falha passar despercebida.
	db := setupTestDB(t)
	led := &stubLedger{err: paymentclient.ErrUpstream}
	r := settleRouter(db, led, nil)
	orderID := criaVendaBalcao(t, r, 0)

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(novoNSU()))
	if w.Code != http.StatusOK {
		t.Fatalf("liquidação deveria concluir mesmo com o livro fora: %d — %s", w.Code, w.Body.String())
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM balcao_audit_events
		WHERE order_id = $1 AND action = 'payment.ledger_post_failed'`, orderID).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Errorf("falha do lançamento não deixou rastro (%d linhas): a receita fica subestimada e ninguém saberia o que replicar", n)
	}
}

// ---------------------------------------------------------------------------
// ESTOQUE
// ---------------------------------------------------------------------------

func TestLiquidacaoBaixaOEstoqueReservado(t *testing.T) {
	// No fluxo pago normal quem dá baixa é o consumer de pagamento
	// (payment.confirmed → stock.Commit). A liquidação externa não passa por
	// Kafka nenhum, então a baixa precisa acontecer no handler. Sem isso, a
	// mercadoria sai da loja e o sweeper devolve a reserva ao estoque em 30
	// minutos — o sistema passa a acreditar que tem um item que já foi embora.
	db := setupTestDB(t)
	stock := &stubReserver{}
	r := settleRouter(db, &stubLedger{}, stock)
	orderID := criaVendaBalcao(t, r, 0)

	if w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(novoNSU())); w.Code != http.StatusOK {
		t.Fatalf("liquidar: %d — %s", w.Code, w.Body.String())
	}

	if len(stock.committed) != 1 || stock.committed[0] != orderID {
		t.Fatalf("reserva não virou baixa definitiva: committed = %v", stock.committed)
	}
	if len(stock.released) != 0 {
		t.Errorf("estoque devolvido numa venda liquidada: %v", stock.released)
	}
}

// ---------------------------------------------------------------------------
// DESCONTO PENDENTE
// ---------------------------------------------------------------------------

func TestDescontoPendenteNaoEhLiquidado(t *testing.T) {
	// Sem esta trava a fila de aprovação vira decoração: bastaria dar um
	// desconto acima do teto e cobrar na maquininha antes de o gerente ver.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)

	orderID := criaVendaBalcao(t, r, 40) // teto do op-a é 12% → nasce pending

	w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(novoNSU()))
	if w.Code != http.StatusConflict {
		t.Fatalf("pedido com desconto pendente foi liquidado: status %d — %s", w.Code, w.Body.String())
	}

	// Depois de aprovado pelo gerente (que não foi quem vendeu), liquida.
	if w := doAs(r, http.MethodPatch, "/api/v1/balcao/orders/"+orderID+"/approve",
		managerA, map[string]any{}); w.Code != http.StatusOK {
		t.Fatalf("aprovar: %d — %s", w.Code, w.Body.String())
	}
	if w := doAs(r, http.MethodPost, "/api/v1/balcao/orders/"+orderID+"/settle-external",
		operatorA, settlePayload(novoNSU())); w.Code != http.StatusOK {
		t.Fatalf("liquidação após aprovação: %d — %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PAYLOAD
// ---------------------------------------------------------------------------

func TestLiquidacaoExigeNSU(t *testing.T) {
	// Sem NSU não existe conciliação possível: a liquidação vira "o operador
	// disse que pagou", sem nada que amarre à linha do extrato do adquirente.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)
	orderID := criaVendaBalcao(t, r, 0)
	path := "/api/v1/balcao/orders/" + orderID + "/settle-external"

	for _, body := range []map[string]any{
		{},
		{"nsu": ""},
		{"nsu": "12"},
		{"nsu": "004417", "brand": "vsia"},
	} {
		w := doAs(r, http.MethodPost, path, operatorA, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("payload %v: status %d, esperado 400 — %s", body, w.Code, w.Body.String())
		}
	}
}

func TestRegression_PedidoWebNaoNasceComMetodoExternal(t *testing.T) {
	// `external` significa "vai ser pago na maquininha da loja", e não existe
	// maquininha no site. Aceitar num pedido web criaria um pedido que nenhum
	// PSP vai cobrar e que a liquidação recusaria depois — órfão desde o
	// nascimento.
	db := setupTestDB(t)
	r := settleRouter(db, &stubLedger{}, nil)

	payload := balcaoPayload(0)
	payload["channel"] = "web"
	payload["paymentMethod"] = "external"
	payload["address"] = map[string]any{
		"street": "Rua A", "number": "10", "neighborhood": "Centro",
		"city": "São Paulo", "state": "SP", "cep": "01000-000",
	}
	w := doAs(r, http.MethodPost, "/api/v1/orders", plainUser, payload)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("pedido web com paymentMethod=external: status %d — %s", w.Code, w.Body.String())
	}
}
