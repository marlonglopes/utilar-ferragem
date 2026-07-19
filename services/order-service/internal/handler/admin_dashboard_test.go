package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/authclient"
	"github.com/utilar/order-service/internal/handler"
)

// ============================================================================
// Agregação do painel — números conferidos À MÃO
// ----------------------------------------------------------------------------
// PORQUÊ os dados são montados aqui em vez de usar o seed: um relatório
// financeiro só pode ser testado contra um número que alguém somou de cabeça.
// Rodar a agregação sobre o seed e comparar com a mesma agregação seria testar
// a consulta contra ela mesma — passaria com a conta errada.
//
// PORQUÊ um período de 2019: o seed e qualquer dado de dev vivem perto de hoje.
// Uma janela antiga e fixa isola completamente o teste do que mais estiver no
// banco, então o número esperado não muda quando alguém roda `make order-db-seed`.
// ============================================================================

// Janela do cenário: 2019-03-01 a 2019-03-03, inclusiva nos dois extremos.
const (
	cenarioFrom = "2019-03-01"
	cenarioTo   = "2019-03-03"
)

// custoFalso devolve custos determinísticos. Interface (e não o catalog real)
// porque margem depende de custo, e um teste de margem contra um catálogo que
// muda não afirma nada.
type custoFalso map[string]float64

func (c custoFalso) Costs(_ context.Context, ids []string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, id := range ids {
		if v, ok := c[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

// diretorioFalso resolve nomes sem chamar o auth-service.
type diretorioFalso map[string]authclient.OperatorInfo

func (d diretorioFalso) Operators(_ context.Context, _ string) (map[string]authclient.OperatorInfo, error) {
	return d, nil
}

// pedidoTeste descreve uma linha do cenário em termos de negócio.
type pedidoTeste struct {
	numero    string
	status    string
	metodo    string
	total     float64
	subtotal  float64
	desconto  float64
	criadoEm  string // YYYY-MM-DD
	pagoEm    string // vazio = nunca pago
	canal     string
	operador  string
	loja      string
	aprovacao string
	itens     []itemTeste
}

type itemTeste struct {
	produtoID  string
	quantidade int
	precoUnit  float64
}

// montarCenario insere os pedidos e devolve a função de limpeza.
//
// A limpeza roda com t.Cleanup e apaga por prefixo do número do pedido: se o
// teste falhar no meio, o banco de dev não fica com pedidos-fantasma de 2019
// poluindo o painel de quem estiver desenvolvendo.
func montarCenario(t *testing.T, db *sql.DB, prefixo string, pedidos []pedidoTeste) {
	t.Helper()

	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM orders WHERE number LIKE $1`, prefixo+"%"); err != nil {
			t.Logf("limpeza do cenário falhou: %v", err)
		}
	})
	// Limpa ANTES também: uma execução anterior interrompida (Ctrl-C, panic)
	// deixaria as linhas para trás e a soma sairia dobrada — falha confusa e
	// difícil de reproduzir.
	if _, err := db.Exec(`DELETE FROM orders WHERE number LIKE $1`, prefixo+"%"); err != nil {
		t.Fatalf("limpeza prévia: %v", err)
	}

	for _, p := range pedidos {
		canal := p.canal
		if canal == "" {
			canal = "web"
		}
		aprov := p.aprovacao
		if aprov == "" {
			aprov = "not_required"
		}
		var pagoEm any
		if p.pagoEm != "" {
			pagoEm = p.pagoEm + " 12:00:00+00"
		}
		var operador, loja any
		if p.operador != "" {
			operador = p.operador
		}
		if p.loja != "" {
			loja = p.loja
		}

		var id string
		err := db.QueryRow(`
			INSERT INTO orders (number, user_id, status, payment_method, subtotal,
			                    shipping_cost, total, created_at, paid_at, channel,
			                    operator_id, store_id, discount_pct, discount_amount,
			                    approval_status, customer_name)
			VALUES ($1,'user-cenario',$2::order_status,$3::payment_method,$4,0,$5,
			        $6::timestamptz,$7::timestamptz,$8::order_channel,$9,$10,0,$11,
			        $12::approval_status,'Cliente Teste')
			RETURNING id::text
		`, prefixo+p.numero, p.status, p.metodo, p.subtotal, p.total,
			p.criadoEm+" 12:00:00+00", pagoEm, canal, operador, loja, p.desconto, aprov).Scan(&id)
		if err != nil {
			t.Fatalf("inserir pedido %s: %v", p.numero, err)
		}

		for i, it := range p.itens {
			_, err := db.Exec(`
				INSERT INTO order_items (order_id, product_id, name, icon, seller_id,
				                         seller_name, quantity, unit_price)
				VALUES ($1, $2::uuid, $3, '📦', 'seller-1', 'Loja Teste', $4, $5)
			`, id, it.produtoID, fmt.Sprintf("Item %d", i), it.quantidade, it.precoUnit)
			if err != nil {
				t.Fatalf("inserir item de %s: %v", p.numero, err)
			}
		}
	}
}

// dashDB abre o banco de teste. Skipa se indisponível (mesmo padrão do
// order_test.go) — mas NÃO exige seed: o cenário se basta.
func dashDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", orderTestDSN())
	if err != nil {
		t.Skipf("banco de teste indisponível: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("banco de teste inacessível: %v", err)
	}
	// O painel depende dos índices da migration 006. Sem eles a consulta ainda
	// responde certo (seq scan), então isto é só um aviso — não uma falha.
	return db
}

func orderTestDSN() string {
	if v := envOr("ORDER_DB_URL", ""); v != "" {
		return v
	}
	return "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func dashRouterComDB(db *sql.DB, custos handler.CostLookup, dir handler.OperatorDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/api/v1/admin", handler.RequireRole(dashSecret, false, "admin"))
	h := handler.NewAdminDashboardHandler(db, custos, dir)
	g.GET("/overview", h.Overview)
	g.GET("/sellers/performance", h.SellersPerformance)
	return r
}

// ---------------------------------------------------------------------------
// Visão geral
// ---------------------------------------------------------------------------

// TestOverview_AgregacaoConferidaAMao.
//
// Cenário (3 dias, janela 2019-03-01..2019-03-03):
//
//	#1  01/03  pago 01/03  pix     R$ 100,00
//	#2  01/03  pago 01/03  pix     R$  50,00
//	#3  02/03  pago 02/03  card    R$ 200,00
//	#4  02/03  NUNCA pago  boleto  R$ 999,00   ← não é faturamento
//	#5  03/03  cancelado   pix     R$ 777,00   ← não é faturamento
//	#6  10/03  pago 10/03  pix     R$ 500,00   ← FORA da janela
//
// Somas feitas à mão:
//
//	01/03 = 100 + 50 = R$ 150,00 → 15000 centavos, 2 pedidos
//	02/03 = 200            → 20000 centavos, 1 pedido
//	03/03 = 0              → 0, 0 pedidos (dia sem venda, mas COM ponto na série)
//	total = 35000 centavos, 3 pedidos
//	ticket médio = 35000 / 3 = 11666 (divisão inteira)
//
// O #6 é o teste do limite superior: um pedido fora da janela entrando na
// soma é o bug de relatório mais comum, e o mais difícil de notar.
func TestOverview_AgregacaoConferidaAMao(t *testing.T) {
	db := dashDB(t)
	montarCenario(t, db, "TST-OV-", []pedidoTeste{
		{numero: "1", status: "delivered", metodo: "pix", total: 100, subtotal: 100, criadoEm: "2019-03-01", pagoEm: "2019-03-01"},
		{numero: "2", status: "delivered", metodo: "pix", total: 50, subtotal: 50, criadoEm: "2019-03-01", pagoEm: "2019-03-01"},
		{numero: "3", status: "shipped", metodo: "card", total: 200, subtotal: 200, criadoEm: "2019-03-02", pagoEm: "2019-03-02"},
		{numero: "4", status: "pending_payment", metodo: "boleto", total: 999, subtotal: 999, criadoEm: "2019-03-02"},
		{numero: "5", status: "cancelled", metodo: "pix", total: 777, subtotal: 777, criadoEm: "2019-03-03"},
		{numero: "6", status: "delivered", metodo: "pix", total: 500, subtotal: 500, criadoEm: "2019-03-10", pagoEm: "2019-03-10"},
	})

	r := dashRouterComDB(db, nil, nil)
	w := dashGet(r, "/api/v1/admin/overview?from="+cenarioFrom+"&to="+cenarioTo, dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Period struct{ From, To string } `json:"period"`
		Kpis   struct {
			AvgTicketCents int64 `json:"avgTicketCents"`
			OrderCount     int   `json:"orderCount"`
		} `json:"kpis"`
		Series []struct {
			Date       string `json:"date"`
			ValueCents int64  `json:"valueCents"`
			Orders     int    `json:"orders"`
		} `json:"series"`
		ByStatus []struct {
			Status     string `json:"status"`
			Count      int    `json:"count"`
			ValueCents int64  `json:"valueCents"`
		} `json:"byStatus"`
		Funnel struct {
			Created   int `json:"created"`
			Confirmed int `json:"confirmed"`
			Failed    int `json:"failed"`
			ByMethod  []struct {
				Method    string `json:"method"`
				Created   int    `json:"created"`
				Confirmed int    `json:"confirmed"`
			} `json:"byMethod"`
		} `json:"funnel"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v — corpo: %s", err, w.Body.String())
	}

	// O período volta exatamente como foi pedido.
	if got.Period.From != cenarioFrom || got.Period.To != cenarioTo {
		t.Errorf("período = %s..%s, esperado %s..%s", got.Period.From, got.Period.To, cenarioFrom, cenarioTo)
	}

	// A série tem UM PONTO POR DIA, inclusive o dia sem venda. Sem isso o
	// gráfico comprime a semana e sugere continuidade onde não houve.
	if len(got.Series) != 3 {
		t.Fatalf("série com %d pontos, esperado 3 (um por dia da janela inclusiva)", len(got.Series))
	}
	esperado := []struct {
		data   string
		cents  int64
		orders int
	}{
		{"2019-03-01", 15000, 2},
		{"2019-03-02", 20000, 1},
		{"2019-03-03", 0, 0}, // dia sem venda continua na série
	}
	for i, e := range esperado {
		s := got.Series[i]
		if s.Date != e.data || s.ValueCents != e.cents || s.Orders != e.orders {
			t.Errorf("série[%d] = %s/%d centavos/%d pedidos; esperado %s/%d/%d",
				i, s.Date, s.ValueCents, s.Orders, e.data, e.cents, e.orders)
		}
	}

	// 35000 / 3 = 11666 (divisão inteira). Se algum dia isto virar 11666,67,
	// o contrato quebrou: o campo é `*Cents`, inteiro.
	if got.Kpis.OrderCount != 3 {
		t.Errorf("orderCount = %d, esperado 3 (só os pagos e não cancelados)", got.Kpis.OrderCount)
	}
	if got.Kpis.AvgTicketCents != 11666 {
		t.Errorf("avgTicketCents = %d, esperado 11666 (35000/3)", got.Kpis.AvgTicketCents)
	}

	// byStatus é ancorado em created_at: os 5 pedidos criados na janela.
	porStatus := map[string]int{}
	valorPorStatus := map[string]int64{}
	for _, b := range got.ByStatus {
		porStatus[b.Status] = b.Count
		valorPorStatus[b.Status] = b.ValueCents
	}
	if porStatus["delivered"] != 2 {
		t.Errorf("byStatus[delivered] = %d, esperado 2", porStatus["delivered"])
	}
	if valorPorStatus["delivered"] != 15000 {
		t.Errorf("byStatus[delivered].valueCents = %d, esperado 15000", valorPorStatus["delivered"])
	}
	// TRADUÇÃO DO ENUM: o banco grava 'pending_payment' e 'cancelled'; o
	// contrato do front usa 'pending' e 'canceled'. Este é o teste que trava a
	// tradução — sem ela a tela mostra buckets vazios sem erro nenhum.
	if porStatus["pending"] != 1 {
		t.Errorf("byStatus[pending] = %d, esperado 1 (traduzido de 'pending_payment')", porStatus["pending"])
	}
	if porStatus["canceled"] != 1 {
		t.Errorf("byStatus[canceled] = %d, esperado 1 (traduzido de 'cancelled')", porStatus["canceled"])
	}
	if _, existe := porStatus["pending_payment"]; existe {
		t.Error("byStatus vazou o enum do banco ('pending_payment') — o contrato pede 'pending'")
	}
	if _, existe := porStatus["cancelled"]; existe {
		t.Error("byStatus vazou o enum do banco ('cancelled') — o contrato pede 'canceled'")
	}

	// Funil: 5 criados na janela, 3 chegaram a ter paid_at, 1 cancelado sem
	// nunca ter sido pago.
	if got.Funnel.Created != 5 {
		t.Errorf("funnel.created = %d, esperado 5", got.Funnel.Created)
	}
	if got.Funnel.Confirmed != 3 {
		t.Errorf("funnel.confirmed = %d, esperado 3", got.Funnel.Confirmed)
	}
	if got.Funnel.Failed != 1 {
		t.Errorf("funnel.failed = %d, esperado 1 (o cancelado sem paid_at)", got.Funnel.Failed)
	}

	// Conversão que o front deriva: 3/5 = 60%. O backend NÃO manda a razão
	// pronta (decisão do contrato) — este teste garante que os dois números que
	// a alimentam estão corretos.
	metodos := map[string][2]int{}
	for _, m := range got.Funnel.ByMethod {
		metodos[m.Method] = [2]int{m.Created, m.Confirmed}
	}
	if metodos["pix"] != [2]int{3, 2} {
		t.Errorf("funnel.byMethod[pix] = %v, esperado [3 criados, 2 confirmados]", metodos["pix"])
	}
	if metodos["boleto"] != [2]int{1, 0} {
		t.Errorf("funnel.byMethod[boleto] = %v, esperado [1 criado, 0 confirmados]", metodos["boleto"])
	}
}

// TestOverview_TabelaVaziaNaoQuebra — painel recém-instalado.
//
// PORQUÊ este teste importa mais do que parece: o primeiro uso do painel é
// sempre com zero pedidos no recorte, e é exatamente aí que ele precisa
// funcionar — se a primeira impressão do dono é uma tela de erro, ele não volta.
// Os modos de falha cobertos são divisão por zero (ticket médio) e `null` em
// lugar de lista vazia (o `.map()` do front estoura em null, não em []).
func TestOverview_TabelaVaziaNaoQuebra(t *testing.T) {
	db := dashDB(t)

	// Janela de 2005: garantidamente sem nenhum pedido, em qualquer base.
	r := dashRouterComDB(db, nil, nil)
	w := dashGet(r, "/api/v1/admin/overview?from=2005-01-01&to=2005-01-03", dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("período vazio devolveu %d: %s", w.Code, w.Body.String())
	}

	// Checagem no JSON CRU: decodificar em struct converteria `null` em slice
	// vazio silenciosamente e o teste passaria com o bug presente.
	var cru map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &cru); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	for _, campo := range []string{"series", "byStatus", "stuckOrders", "alerts"} {
		if string(cru[campo]) == "null" {
			t.Errorf("%s veio `null` — o front faz .map() nisso e a tela quebra; use []", campo)
		}
	}

	var got struct {
		Kpis struct {
			AvgTicketCents int64 `json:"avgTicketCents"`
			OrderCount     int   `json:"orderCount"`
		} `json:"kpis"`
		Series []any `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	// Zero pedidos tem ticket médio ZERO, não NaN. `NaN` nem é JSON válido —
	// derrubaria a tela inteira em vez de mostrar um painel vazio.
	if got.Kpis.AvgTicketCents != 0 || got.Kpis.OrderCount != 0 {
		t.Errorf("período vazio: ticket=%d count=%d, esperado 0/0",
			got.Kpis.AvgTicketCents, got.Kpis.OrderCount)
	}
	// A série continua tendo um ponto por dia, todos zerados: um gráfico vazio
	// com eixo é informação ("não vendemos nada"); um gráfico sem eixo é um bug.
	if len(got.Series) != 3 {
		t.Errorf("série vazia com %d pontos, esperado 3 (um por dia, zerados)", len(got.Series))
	}
}

// TestOverview_PeriodoInvalidoERecusado — período não pode ser aceito em
// silêncio quando é impossível ou grande demais.
//
// PORQUÊ recusar em vez de truncar: um `from` depois do `to` truncado para
// "últimos 30 dias" devolveria um número plausível para uma pergunta que
// ninguém fez. Já um período de 10 anos varre a tabela inteira de pedidos e
// compete com a venda pelo pool de conexões — o painel derrubaria a loja.
func TestOverview_PeriodoInvalidoERecusado(t *testing.T) {
	db := dashDB(t)
	r := dashRouterComDB(db, nil, nil)
	tok := dashToken(t, "admin")

	casos := []struct {
		nome  string
		query string
	}{
		{"from depois do to", "?from=2019-03-10&to=2019-03-01"},
		{"formato invalido no from", "?from=10/03/2019&to=2019-03-01"},
		{"formato invalido no to", "?from=2019-03-01&to=ontem"},
		{"janela longa demais", "?from=2010-01-01&to=2026-01-01"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := dashGet(r, "/api/v1/admin/overview"+tc.query, tok)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d, esperado 400 — corpo: %s", w.Code, w.Body.String())
			}
			var env handler.ErrorEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("resposta não usa o envelope de erro do projeto: %v", err)
			}
			if env.Code != "validation_error" {
				t.Errorf("code = %q, esperado validation_error", env.Code)
			}
		})
	}
}

// TestOverview_ParametroAusenteUsaPadrao — o front chama sem from/to no
// primeiro load. Não pode ser 400.
func TestOverview_ParametroAusenteUsaPadrao(t *testing.T) {
	db := dashDB(t)
	r := dashRouterComDB(db, nil, nil)

	w := dashGet(r, "/api/v1/admin/overview", dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("sem from/to devolveu %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Series []any `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Series) != 30 {
		t.Errorf("período padrão devolveu %d pontos, esperado 30 dias", len(got.Series))
	}
}

// TestOverview_NaoCacheia — faturamento não pode ficar em cache de proxy.
func TestOverview_NaoCacheia(t *testing.T) {
	db := dashDB(t)
	r := dashRouterComDB(db, nil, nil)
	w := dashGet(r, "/api/v1/admin/overview?from=2005-01-01&to=2005-01-02", dashToken(t, "admin"))
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, esperado no-store — agregado de receita não pode ser guardado por intermediário", got)
	}
}

// ---------------------------------------------------------------------------
// Desempenho de vendedores
// ---------------------------------------------------------------------------

// TestSellersPerformance_AgregacaoConferidaAMao.
//
// Cenário — dois vendedores na loja-1, janela 2019-03-01..2019-03-03:
//
//	ana:  #A1 total R$ 180,00, desconto R$ 20,00, subtotal 200, aprovação 'approved'
//	          itens: 2x produto-P a R$ 100,00 (lista R$ 200,00)
//	      #A2 total R$ 100,00, desconto R$   0,00, subtotal 100
//	          itens: 1x produto-P a R$ 100,00
//	bruno:#B1 total R$  90,00, desconto R$ 10,00, subtotal 100
//	          itens: 1x produto-Q a R$ 100,00
//
// Custos (do catálogo falso): produto-P = R$ 60,00; produto-Q = R$ 50,00.
//
// Conferência à mão — ANA:
//
//	totalCents  = 18000 + 10000 = 28000
//	orderCount  = 2
//	ticket      = 28000 / 2 = 14000
//	bruto       = 28000 + 2000 (desconto) = 30000
//	descontoPct = 2000 / 30000 = 0,0667 (4 casas)
//	receita líq = #A1: 200 × (1 − 20/200) = 180
//	              #A2: 100 × (1 − 0/100)  = 100  → 280
//	CMV         = 3 unidades × 60 = 180
//	margem      = (280 − 180) / 280 = 0,3571
//	aprovações  = 1 (só o #A1 saiu de 'not_required')
//
// Conferência à mão — BRUNO:
//
//	totalCents  = 9000; ticket = 9000; bruto = 10000; descontoPct = 0,1
//	receita líq = 100 × (1 − 10/100) = 90; CMV = 1 × 50 = 50
//	margem      = (90 − 50) / 90 = 0,4444
func TestSellersPerformance_AgregacaoConferidaAMao(t *testing.T) {
	db := dashDB(t)

	const produtoP = "11111111-1111-1111-1111-111111111111"
	const produtoQ = "22222222-2222-2222-2222-222222222222"

	montarCenario(t, db, "TST-SP-", []pedidoTeste{
		{
			numero: "A1", status: "delivered", metodo: "card",
			total: 180, subtotal: 200, desconto: 20,
			criadoEm: "2019-03-01", pagoEm: "2019-03-01",
			canal: "balcao", operador: "op-ana", loja: "loja-1", aprovacao: "approved",
			itens: []itemTeste{{produtoID: produtoP, quantidade: 2, precoUnit: 100}},
		},
		{
			numero: "A2", status: "delivered", metodo: "pix",
			total: 100, subtotal: 100,
			criadoEm: "2019-03-02", pagoEm: "2019-03-02",
			canal: "balcao", operador: "op-ana", loja: "loja-1",
			itens: []itemTeste{{produtoID: produtoP, quantidade: 1, precoUnit: 100}},
		},
		{
			numero: "B1", status: "delivered", metodo: "pix",
			total: 90, subtotal: 100, desconto: 10,
			criadoEm: "2019-03-01", pagoEm: "2019-03-01",
			canal: "balcao", operador: "op-bruno", loja: "loja-1",
			itens: []itemTeste{{produtoID: produtoQ, quantidade: 1, precoUnit: 100}},
		},
		// Venda WEB no mesmo período: não tem vendedor e NÃO pode aparecer na
		// tabela. Se aparecesse, criaria uma linha fantasma com o faturamento
		// do site inteiro e esconderia todos os vendedores reais.
		{
			numero: "W1", status: "delivered", metodo: "pix",
			total: 5000, subtotal: 5000,
			criadoEm: "2019-03-01", pagoEm: "2019-03-01",
			itens: []itemTeste{{produtoID: produtoP, quantidade: 50, precoUnit: 100}},
		},
	})

	custos := custoFalso{produtoP: 60, produtoQ: 50}
	dir := diretorioFalso{
		"op-ana":   {Name: "Ana Souza", StoreID: "loja-1", StoreName: "Utilar Centro"},
		"op-bruno": {Name: "Bruno Lima", StoreID: "loja-1", StoreName: "Utilar Centro"},
	}

	r := dashRouterComDB(db, custos, dir)
	w := dashGet(r, "/api/v1/admin/sellers/performance?from="+cenarioFrom+"&to="+cenarioTo, dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Stores  []struct{ ID, Name string } `json:"stores"`
		Sellers []struct {
			SellerID         string  `json:"sellerId"`
			SellerName       string  `json:"sellerName"`
			StoreName        string  `json:"storeName"`
			TotalCents       int64   `json:"totalCents"`
			OrderCount       int     `json:"orderCount"`
			AvgTicketCents   int64   `json:"avgTicketCents"`
			AvgDiscountPct   float64 `json:"avgDiscountPct"`
			AvgMarginPct     float64 `json:"avgMarginPct"`
			ManagerApprovals int     `json:"managerApprovals"`
			Series           []struct {
				Date       string `json:"date"`
				ValueCents int64  `json:"valueCents"`
			} `json:"series"`
		} `json:"sellers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v — corpo: %s", err, w.Body.String())
	}

	if len(got.Sellers) != 2 {
		t.Fatalf("esperado 2 vendedores, veio %d — venda web pode ter vazado para a tabela: %+v",
			len(got.Sellers), got.Sellers)
	}

	porID := map[string]int{}
	for i, s := range got.Sellers {
		porID[s.SellerID] = i
	}
	ana := got.Sellers[porID["op-ana"]]
	bruno := got.Sellers[porID["op-bruno"]]

	// Ordenação: maior faturamento primeiro.
	if got.Sellers[0].SellerID != "op-ana" {
		t.Errorf("primeiro da lista = %s, esperado op-ana (maior faturamento)", got.Sellers[0].SellerID)
	}

	// --- Ana ---
	if ana.SellerName != "Ana Souza" {
		t.Errorf("sellerName = %q, esperado 'Ana Souza' (veio do diretório do auth-service)", ana.SellerName)
	}
	if ana.StoreName != "Utilar Centro" {
		t.Errorf("storeName = %q, esperado 'Utilar Centro'", ana.StoreName)
	}
	if ana.TotalCents != 28000 {
		t.Errorf("ana.totalCents = %d, esperado 28000 (18000 + 10000)", ana.TotalCents)
	}
	if ana.OrderCount != 2 {
		t.Errorf("ana.orderCount = %d, esperado 2", ana.OrderCount)
	}
	if ana.AvgTicketCents != 14000 {
		t.Errorf("ana.avgTicketCents = %d, esperado 14000 (28000/2)", ana.AvgTicketCents)
	}
	// 2000 / 30000 = 0,066666… → 0,0667. Fração 0..1, NUNCA 0..100: o front
	// multiplica por 100 ao exibir, então 6,67 aqui viraria 667% na tela.
	if !quaseIgual(ana.AvgDiscountPct, 0.0667) {
		t.Errorf("ana.avgDiscountPct = %v, esperado 0.0667 (2000/30000, fração do BRUTO)", ana.AvgDiscountPct)
	}
	// (280 − 180) / 280 = 0,357142… → 0,3571
	if !quaseIgual(ana.AvgMarginPct, 0.3571) {
		t.Errorf("ana.avgMarginPct = %v, esperado 0.3571 ((280−180)/280)", ana.AvgMarginPct)
	}
	if ana.ManagerApprovals != 1 {
		t.Errorf("ana.managerApprovals = %d, esperado 1 (só o #A1 saiu de not_required)", ana.ManagerApprovals)
	}
	if len(ana.Series) != 2 {
		t.Errorf("ana.series com %d pontos, esperado 2 (vendeu em 01 e 02/03)", len(ana.Series))
	}

	// --- Bruno ---
	if bruno.TotalCents != 9000 {
		t.Errorf("bruno.totalCents = %d, esperado 9000", bruno.TotalCents)
	}
	if !quaseIgual(bruno.AvgDiscountPct, 0.1) {
		t.Errorf("bruno.avgDiscountPct = %v, esperado 0.1 (1000/10000)", bruno.AvgDiscountPct)
	}
	// (90 − 50) / 90 = 0,44444… → 0,4444
	if !quaseIgual(bruno.AvgMarginPct, 0.4444) {
		t.Errorf("bruno.avgMarginPct = %v, esperado 0.4444 ((90−50)/90)", bruno.AvgMarginPct)
	}
	if bruno.ManagerApprovals != 0 {
		t.Errorf("bruno.managerApprovals = %d, esperado 0", bruno.ManagerApprovals)
	}

	// A loja aparece uma vez só, com nome resolvido.
	if len(got.Stores) != 1 || got.Stores[0].ID != "loja-1" || got.Stores[0].Name != "Utilar Centro" {
		t.Errorf("stores = %+v, esperado [{loja-1 Utilar Centro}]", got.Stores)
	}
}

// TestSellersPerformance_NuncaVazaCusto — regressão de segurança.
//
// O requisito é explícito no contrato: custo de aquisição NÃO trafega para o
// navegador, só a margem já agregada. Custo unitário no DevTools entrega a
// estrutura de compra da Utilar — inclusive para o próprio vendedor, se um dia
// ganhar acesso a alguma tela de admin.
//
// A verificação é sobre o JSON CRU e por SUBSTRING, não sobre uma struct: um
// campo novo adicionado no futuro (`cost`, `unitCost`, `cogs`) passaria
// despercebido por qualquer decodificação tipada.
func TestSellersPerformance_NuncaVazaCusto(t *testing.T) {
	db := dashDB(t)

	const produtoP = "11111111-1111-1111-1111-111111111111"
	montarCenario(t, db, "TST-CUSTO-", []pedidoTeste{{
		numero: "1", status: "delivered", metodo: "pix",
		total: 100, subtotal: 100,
		criadoEm: "2019-03-01", pagoEm: "2019-03-01",
		canal: "balcao", operador: "op-ana", loja: "loja-1",
		// Custo distinto e reconhecível: 73,91 não aparece em nenhum outro
		// número do cenário, então achá-lo na resposta só pode ser vazamento.
		itens: []itemTeste{{produtoID: produtoP, quantidade: 1, precoUnit: 100}},
	}})

	r := dashRouterComDB(db, custoFalso{produtoP: 73.91}, nil)
	w := dashGet(r, "/api/v1/admin/sellers/performance?from="+cenarioFrom+"&to="+cenarioTo, dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	corpo := w.Body.String()
	for _, proibido := range []string{"73.91", "7391", "\"cost\"", "\"cogs\"", "\"unitCost\"", "\"custo\""} {
		if contemInsensivel(corpo, proibido) {
			t.Errorf("a resposta contém %q — custo de aquisição NUNCA pode trafegar; só a margem agregada.\nCorpo: %s",
				proibido, corpo)
		}
	}

	// E a margem — que É permitida — tem que estar lá e correta:
	// (100 − 73,91) / 100 = 0,2609
	var got struct {
		Sellers []struct {
			AvgMarginPct float64 `json:"avgMarginPct"`
		} `json:"sellers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Sellers) != 1 || !quaseIgual(got.Sellers[0].AvgMarginPct, 0.2609) {
		t.Errorf("margem = %+v, esperado 0.2609 ((100−73,91)/100)", got.Sellers)
	}
}

// TestSellersPerformance_ProdutoSemCustoFicaForaDaMargem.
//
// PORQUÊ este é o caso perigoso: preencher custo ausente com ZERO daria 100%
// de margem. É o erro mais caro possível nesta tela — faria o gerente enxergar
// folga para desconto onde não existe nenhuma, e o desconto sai em dinheiro.
func TestSellersPerformance_ProdutoSemCustoFicaForaDaMargem(t *testing.T) {
	db := dashDB(t)

	const comCusto = "11111111-1111-1111-1111-111111111111"
	const semCusto = "33333333-3333-3333-3333-333333333333"

	montarCenario(t, db, "TST-SEMCUSTO-", []pedidoTeste{{
		numero: "1", status: "delivered", metodo: "pix",
		total: 200, subtotal: 200,
		criadoEm: "2019-03-01", pagoEm: "2019-03-01",
		canal: "balcao", operador: "op-ana", loja: "loja-1",
		itens: []itemTeste{
			{produtoID: comCusto, quantidade: 1, precoUnit: 100},
			{produtoID: semCusto, quantidade: 1, precoUnit: 100},
		},
	}})

	// Só o primeiro produto tem custo cadastrado.
	r := dashRouterComDB(db, custoFalso{comCusto: 60}, nil)
	w := dashGet(r, "/api/v1/admin/sellers/performance?from="+cenarioFrom+"&to="+cenarioTo, dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Sellers []struct {
			AvgMarginPct float64 `json:"avgMarginPct"`
		} `json:"sellers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Sellers) != 1 {
		t.Fatalf("esperado 1 vendedor, veio %d", len(got.Sellers))
	}

	// A margem considera SÓ o item com custo: (100 − 60) / 100 = 0,40.
	// Se o item sem custo entrasse com custo zero, a conta seria
	// (200 − 60) / 200 = 0,70 — e é justamente esse 0,70 otimista e falso que
	// este teste existe para impedir.
	if !quaseIgual(got.Sellers[0].AvgMarginPct, 0.40) {
		t.Errorf("margem = %v, esperado 0.40 (só o item com custo conhecido). "+
			"0.70 significaria que o produto sem custo entrou como custo zero",
			got.Sellers[0].AvgMarginPct)
	}
}

// TestSellersPerformance_FiltroPorLoja — o dono filtra por filial.
func TestSellersPerformance_FiltroPorLoja(t *testing.T) {
	db := dashDB(t)

	montarCenario(t, db, "TST-LOJA-", []pedidoTeste{
		{numero: "1", status: "delivered", metodo: "pix", total: 100, subtotal: 100,
			criadoEm: "2019-03-01", pagoEm: "2019-03-01",
			canal: "balcao", operador: "op-ana", loja: "loja-1"},
		{numero: "2", status: "delivered", metodo: "pix", total: 300, subtotal: 300,
			criadoEm: "2019-03-01", pagoEm: "2019-03-01",
			canal: "balcao", operador: "op-carla", loja: "loja-2"},
	})

	r := dashRouterComDB(db, nil, nil)
	w := dashGet(r, "/api/v1/admin/sellers/performance?from="+cenarioFrom+"&to="+cenarioTo+"&storeId=loja-2",
		dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Sellers []struct {
			SellerID   string `json:"sellerId"`
			TotalCents int64  `json:"totalCents"`
		} `json:"sellers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Sellers) != 1 || got.Sellers[0].SellerID != "op-carla" || got.Sellers[0].TotalCents != 30000 {
		t.Errorf("filtro storeId=loja-2 devolveu %+v, esperado só op-carla com 30000", got.Sellers)
	}
}

// TestSellersPerformance_SemVendedorNaoQuebra — tabela vazia.
func TestSellersPerformance_SemVendedorNaoQuebra(t *testing.T) {
	db := dashDB(t)
	r := dashRouterComDB(db, nil, nil)

	w := dashGet(r, "/api/v1/admin/sellers/performance?from=2005-01-01&to=2005-01-03", dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("período vazio devolveu %d: %s", w.Code, w.Body.String())
	}
	var cru map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &cru); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	for _, campo := range []string{"sellers", "stores"} {
		if string(cru[campo]) == "null" {
			t.Errorf("%s veio `null` — o front faz .map() nisso; use []", campo)
		}
	}
}

// TestSellersPerformance_SemDiretorioDegradaParaID — o auth-service fora do ar
// não pode derrubar a tela inteira. O dono ainda precisa ver quanto vendeu.
func TestSellersPerformance_SemDiretorioDegradaParaID(t *testing.T) {
	db := dashDB(t)
	montarCenario(t, db, "TST-NODIR-", []pedidoTeste{{
		numero: "1", status: "delivered", metodo: "pix", total: 100, subtotal: 100,
		criadoEm: "2019-03-01", pagoEm: "2019-03-01",
		canal: "balcao", operador: "op-ana", loja: "loja-1",
	}})

	r := dashRouterComDB(db, nil, diretorioQuebrado{})
	w := dashGet(r, "/api/v1/admin/sellers/performance?from="+cenarioFrom+"&to="+cenarioTo, dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("diretório indisponível devolveu %d — deveria degradar, não falhar: %s", w.Code, w.Body.String())
	}
	var got struct {
		Sellers []struct {
			SellerID   string `json:"sellerId"`
			SellerName string `json:"sellerName"`
		} `json:"sellers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Sellers) != 1 || got.Sellers[0].SellerName != "op-ana" {
		t.Errorf("sem diretório, sellerName deveria cair para o id; veio %+v", got.Sellers)
	}
}

type diretorioQuebrado struct{}

func (diretorioQuebrado) Operators(context.Context, string) (map[string]authclient.OperatorInfo, error) {
	return nil, fmt.Errorf("auth-service indisponível")
}

// quaseIgual compara frações com tolerância de meio ponto na 4ª casa — a
// precisão que round4 garante. Comparação exata de float em teste de agregação
// é receita de flake.
func quaseIgual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.00005
}

func contemInsensivel(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}

// ---------------------------------------------------------------------------
// Pedidos travados
// ---------------------------------------------------------------------------

// TestOverview_PedidosTravados — o número que denuncia operação parada.
//
// "Travado" = pagamento confirmado e o pedido ainda em `paid`, sem separação.
// É o sintoma direto de outbox parado (ver a tela de observabilidade): o
// dinheiro entrou, o cliente está esperando, e ninguém separou a mercadoria.
//
// Este teste usa tempo RELATIVO A AGORA (e não a janela fixa de 2019) porque a
// consulta é deliberadamente independente do ?from&to — um pedido travado há 3
// dias continua travado hoje, e filtrá-lo pelo período do gráfico o esconderia
// justamente de quem abriu o painel para descobrir o que está errado.
func TestOverview_PedidosTravados(t *testing.T) {
	db := dashDB(t)

	prefixo := "TST-STUCK-"
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM orders WHERE number LIKE $1`, prefixo+"%"); err != nil {
			t.Logf("limpeza: %v", err)
		}
	})
	if _, err := db.Exec(`DELETE FROM orders WHERE number LIKE $1`, prefixo+"%"); err != nil {
		t.Fatalf("limpeza prévia: %v", err)
	}

	// Três casos na fronteira do limiar de 4h:
	//   travado-10h  → pago há 10h, ainda em 'paid'      → APARECE
	//   recente-1h   → pago há 1h,  ainda em 'paid'      → NÃO aparece (dentro do prazo)
	//   avancou-10h  → pago há 10h, já em 'shipped'      → NÃO aparece (a operação andou)
	casos := []struct {
		numero string
		status string
		horas  int
	}{
		{"travado-10h", "paid", 10},
		{"recente-1h", "paid", 1},
		{"avancou-10h", "shipped", 10},
	}
	for _, c := range casos {
		_, err := db.Exec(`
			INSERT INTO orders (number, user_id, status, payment_method, subtotal,
			                    shipping_cost, total, created_at, paid_at, customer_name)
			VALUES ($1, 'user-cenario', $2::order_status, 'pix', 250, 0, 250,
			        now() - ($3 || ' hours')::interval,
			        now() - ($3 || ' hours')::interval, 'Cliente Travado')
		`, prefixo+c.numero, c.status, c.horas)
		if err != nil {
			t.Fatalf("inserir %s: %v", c.numero, err)
		}
	}

	r := dashRouterComDB(db, nil, nil)
	w := dashGet(r, "/api/v1/admin/overview", dashToken(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		StuckOrders []struct {
			OrderNumber   string  `json:"orderNumber"`
			Status        string  `json:"status"`
			StuckForHours float64 `json:"stuckForHours"`
			TotalCents    int64   `json:"totalCents"`
			CustomerName  string  `json:"customerName"`
		} `json:"stuckOrders"`
		Alerts []struct {
			ID       string `json:"id"`
			Severity string `json:"severity"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}

	nossos := map[string]float64{}
	for _, s := range got.StuckOrders {
		if strings.HasPrefix(s.OrderNumber, prefixo) {
			nossos[s.OrderNumber] = s.StuckForHours
			if s.TotalCents != 25000 {
				t.Errorf("%s totalCents = %d, esperado 25000", s.OrderNumber, s.TotalCents)
			}
			if s.Status != "paid" {
				t.Errorf("%s status = %q, esperado paid", s.OrderNumber, s.Status)
			}
			// O contrato pede customerName para identificar o pedido travado —
			// e NADA além disso. CPF e endereço não entram na tela de admin.
			if s.CustomerName != "Cliente Travado" {
				t.Errorf("%s customerName = %q", s.OrderNumber, s.CustomerName)
			}
		}
	}

	if _, ok := nossos[prefixo+"travado-10h"]; !ok {
		t.Errorf("o pedido pago há 10h e ainda em 'paid' NÃO apareceu como travado — "+
			"é exatamente o caso que o painel existe para pegar. Travados: %+v", got.StuckOrders)
	}
	if _, ok := nossos[prefixo+"recente-1h"]; ok {
		t.Error("pedido pago há 1h apareceu como travado — o limiar é 4h, e alarme falso treina o dono a ignorar o painel")
	}
	if _, ok := nossos[prefixo+"avancou-10h"]; ok {
		t.Error("pedido já em 'shipped' apareceu como travado — a operação andou, não está travada")
	}

	// stuckForHours é calculado NO SERVIDOR: o relógio do navegador pode estar
	// meses errado e "atrasado" é decisão de negócio, não de renderização.
	if h := nossos[prefixo+"travado-10h"]; h < 9.9 || h > 10.1 {
		t.Errorf("stuckForHours = %v, esperado ~10", h)
	}

	// Travado existindo, o alerta correspondente tem que existir — e apontar
	// para a tela que investiga a causa (a fila do outbox).
	var achouAlerta bool
	for _, a := range got.Alerts {
		if a.ID == "stuck-orders" {
			achouAlerta = true
			if a.Severity != "warn" && a.Severity != "critical" {
				t.Errorf("alerta de travados com severidade %q", a.Severity)
			}
		}
	}
	if !achouAlerta {
		t.Error("há pedido travado e nenhum alerta foi emitido")
	}
}
