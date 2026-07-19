// Painel do dono — visão geral (/api/v1/admin/overview) e desempenho de
// vendedores (/api/v1/admin/sellers/performance).
//
// PORQUÊ estas duas rotas vivem no order-service e não no payment-service:
// tudo que elas respondem é pergunta sobre PEDIDO (quanto vendeu, quantos
// pedidos por status, quem vendeu, quanto de desconto deu, o que travou depois
// de pago). O payment-service responde a pergunta do CONTADOR (partida dobrada,
// taxa de PSP, estorno) e já tem as rotas dele em /api/v1/ledger. Juntar os
// dois num serviço só significaria acesso cruzado a banco, que é exatamente o
// que a arquitetura proíbe.
//
// CONTRATO: docs/admin-dashboard-api.md. O front (app/src/lib/adminTypes.ts)
// já foi escrito contra ele — qualquer campo renomeado aqui quebra a tela sem
// erro de compilação em lugar nenhum.
package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/authclient"
)

// ---------------------------------------------------------------------------
// Período
// ---------------------------------------------------------------------------

// maxPeriodDays — teto de janela aceita em ?from&to.
//
// PORQUÊ recusar em vez de truncar em silêncio: um período de 10 anos varre a
// tabela inteira de pedidos e o dono não teria como saber que o número na tela
// é de um recorte diferente do que ele pediu. Errar alto e explicar é melhor
// que devolver um número que parece certo. 366 cobre "o ano passado inteiro",
// que é o maior recorte legítimo que a tela oferece.
const maxPeriodDays = 366

// defaultPeriodDays — janela quando o front não manda from/to. 30 dias é o que
// o painel abre por padrão.
const defaultPeriodDays = 30

// stuckThresholdHours — a partir de quantas horas em `paid` um pedido conta
// como travado. 4h é o limiar sugerido no contrato: acima disso a separação
// deixou de acontecer, não está apenas atrasada.
const stuckThresholdHours = 4

// maxStuckOrders — teto de linhas na lista de travados. A tela é um alerta,
// não um relatório: se houver 3.000 travados, os 100 primeiros já provam que a
// operação parou, e carregar todos transformaria a página de diagnóstico de
// incidente na segunda vítima do incidente.
const maxStuckOrders = 100

type adminPeriod struct {
	From time.Time // 00:00:00 UTC do primeiro dia, inclusivo
	To   time.Time // 00:00:00 UTC do dia SEGUINTE ao último (半-aberto no SQL)
	// FromStr/ToStr são o que volta no JSON — exatamente o que o cliente pediu.
	FromStr string
	ToStr   string
}

// Days devolve o número de dias do período (inclusivo nos dois extremos).
func (p adminPeriod) Days() int { return int(p.To.Sub(p.From).Hours() / 24) }

// parsePeriod lê ?from&to no formato YYYY-MM-DD, INCLUSIVO nos dois extremos.
//
// Internamente o intervalo vira semiaberto [from, to+1dia) porque é a única
// forma de comparar TIMESTAMPTZ sem perder o último dia: `created_at <= '2026-07-18'`
// exclui tudo que aconteceu depois da meia-noite daquele dia — ou seja, o dia
// inteiro menos o primeiro instante. Esse é um bug clássico de relatório e ele
// some sozinho ao devolver os totais em janela semiaberta.
func parsePeriod(c *gin.Context) (adminPeriod, bool) {
	const layout = "2006-01-02"
	now := time.Now().UTC()

	toStr := strings.TrimSpace(c.Query("to"))
	fromStr := strings.TrimSpace(c.Query("from"))

	var to time.Time
	if toStr == "" {
		to = now.Truncate(24 * time.Hour)
		toStr = to.Format(layout)
	} else {
		t, err := time.Parse(layout, toStr)
		if err != nil {
			Respond(c, http.StatusBadRequest, "validation_error", "invalid `to`: expected YYYY-MM-DD")
			return adminPeriod{}, false
		}
		to = t.UTC()
	}

	var from time.Time
	if fromStr == "" {
		from = to.AddDate(0, 0, -(defaultPeriodDays - 1))
		fromStr = from.Format(layout)
	} else {
		f, err := time.Parse(layout, fromStr)
		if err != nil {
			Respond(c, http.StatusBadRequest, "validation_error", "invalid `from`: expected YYYY-MM-DD")
			return adminPeriod{}, false
		}
		from = f.UTC()
	}

	if from.After(to) {
		Respond(c, http.StatusBadRequest, "validation_error", "`from` must not be after `to`")
		return adminPeriod{}, false
	}

	days := int(to.Sub(from).Hours()/24) + 1
	if days > maxPeriodDays {
		Respond(c, http.StatusBadRequest, "validation_error",
			fmt.Sprintf("period too long: %d days (max %d)", days, maxPeriodDays))
		return adminPeriod{}, false
	}

	return adminPeriod{
		From:    from,
		To:      to.AddDate(0, 0, 1), // semiaberto: inclui o último dia inteiro
		FromStr: fromStr,
		ToStr:   toStr,
	}, true
}

// ---------------------------------------------------------------------------
// Tipos de resposta — espelham app/src/lib/adminTypes.ts
// ---------------------------------------------------------------------------

type periodDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type timePoint struct {
	Date       string `json:"date"`
	ValueCents int64  `json:"valueCents"`
	Orders     int    `json:"orders"`
}

type overviewKpis struct {
	TodayCents     int64 `json:"todayCents"`
	WeekCents      int64 `json:"weekCents"`
	MonthCents     int64 `json:"monthCents"`
	TodayPrevCents int64 `json:"todayPrevCents"`
	WeekPrevCents  int64 `json:"weekPrevCents"`
	MonthPrevCents int64 `json:"monthPrevCents"`

	AvgTicketCents     int64 `json:"avgTicketCents"`
	AvgTicketPrevCents int64 `json:"avgTicketPrevCents"`
	OrderCount         int   `json:"orderCount"`
	OrderCountPrev     int   `json:"orderCountPrev"`
}

type statusBucket struct {
	Status     string `json:"status"`
	Count      int    `json:"count"`
	ValueCents int64  `json:"valueCents"`
}

type funnelMethod struct {
	Method    string `json:"method"`
	Created   int    `json:"created"`
	Confirmed int    `json:"confirmed"`
}

type paymentFunnel struct {
	Created   int            `json:"created"`
	Confirmed int            `json:"confirmed"`
	Failed    int            `json:"failed"`
	Expired   int            `json:"expired"`
	ByMethod  []funnelMethod `json:"byMethod"`
}

type stuckOrder struct {
	OrderID       string  `json:"orderId"`
	OrderNumber   string  `json:"orderNumber"`
	Status        string  `json:"status"`
	PaidAt        string  `json:"paidAt"`
	StuckForHours float64 `json:"stuckForHours"`
	TotalCents    int64   `json:"totalCents"`
	CustomerName  string  `json:"customerName"`
}

type alert struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Source   string `json:"source"`
	FiredAt  string `json:"firedAt"`
	Href     string `json:"href,omitempty"`
}

type adminOverview struct {
	Period      periodDTO      `json:"period"`
	Kpis        overviewKpis   `json:"kpis"`
	Series      []timePoint    `json:"series"`
	ByStatus    []statusBucket `json:"byStatus"`
	Funnel      paymentFunnel  `json:"funnel"`
	StuckOrders []stuckOrder   `json:"stuckOrders"`
	Alerts      []alert        `json:"alerts"`
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// CostLookup é o que o handler precisa do catalog-service para calcular
// margem. Interface (e não o cliente concreto) porque o teste de agregação
// precisa de custos determinísticos — e porque margem errada na tela do dono
// vira desconto errado no balcão.
type CostLookup interface {
	// Costs devolve custo unitário por productID. Produto sem custo cadastrado
	// simplesmente não aparece no mapa — ausência é informação, e preencher com
	// zero inflaria a margem para 100%, que é o erro mais perigoso possível
	// aqui (faria o gerente achar que há folga para desconto onde não há).
	Costs(ctx context.Context, productIDs []string) (map[string]float64, error)
}

// OperatorDirectory resolve id de operador → nome e loja. Vive no
// auth-service; o order-service só guarda o id opaco.
//
// O tipo do valor é authclient.OperatorInfo porque handler já importa
// authclient — declará-lo aqui fecharia ciclo de import (ver
// internal/authclient/directory.go).
type OperatorDirectory interface {
	// Operators devolve o diretório indexado por userID. O bearer é o token do
	// ADMIN que chamou o painel — não um token de serviço: quem lista operador
	// é quem já tem poder de admin, e propagar a identidade real mantém a
	// autorização no auth-service em vez de duplicá-la aqui.
	Operators(ctx context.Context, bearer string) (map[string]authclient.OperatorInfo, error)
}

type AdminDashboardHandler struct {
	db        *sql.DB
	costs     CostLookup
	operators OperatorDirectory
}

func NewAdminDashboardHandler(db *sql.DB, costs CostLookup, ops OperatorDirectory) *AdminDashboardHandler {
	return &AdminDashboardHandler{db: db, costs: costs, operators: ops}
}

// statusToWire traduz o enum do banco para o vocabulário do contrato.
//
// DIVERGÊNCIA REAL (registrada em docs/admin-dashboard-api.md): o banco usa
// 'pending_payment' e 'cancelled' (grafia britânica); o contrato do front usa
// 'pending' e 'canceled' (americana). Traduzir aqui é mais barato e mais
// seguro que uma migration de ENUM em produção, e mantém o front intacto.
// 'refunded' existe no contrato e NÃO existe no banco — estorno é fato
// contábil e vive no payment-service; nunca sai um bucket 'refunded' daqui.
func statusToWire(dbStatus string) string {
	switch dbStatus {
	case "pending_payment":
		return "pending"
	case "cancelled":
		return "canceled"
	default:
		return dbStatus
	}
}

// toCents converte NUMERIC(12,2) para centavos inteiros.
//
// math.Round e não truncamento: o float64 que sai do driver para 12,34 pode
// ser 12.339999999999999, e int64(12.339999*100) = 1233 — um centavo a menos
// por pedido, que num relatório de fechamento vira divergência com o contábil.
func toCents(v float64) int64 { return int64(math.Round(v * 100)) }

// Overview GET /api/v1/admin/overview?from&to
func (h *AdminDashboardHandler) Overview(c *gin.Context) {
	period, ok := parsePeriod(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Dado agregado de faturamento é sensível e muda a cada minuto: nenhum
	// proxy no caminho pode guardar uma cópia.
	c.Header("Cache-Control", "no-store")

	series, totalCents, orderCount, err := h.revenueSeries(ctx, period)
	if err != nil {
		DBError(c, err)
		return
	}

	kpis, err := h.rollingKpis(ctx)
	if err != nil {
		DBError(c, err)
		return
	}
	kpis.OrderCount = orderCount
	kpis.AvgTicketCents = avgTicket(totalCents, orderCount)

	prevTotal, prevCount, err := h.previousPeriodTotals(ctx, period)
	if err != nil {
		DBError(c, err)
		return
	}
	kpis.OrderCountPrev = prevCount
	kpis.AvgTicketPrevCents = avgTicket(prevTotal, prevCount)

	byStatus, err := h.byStatus(ctx, period)
	if err != nil {
		DBError(c, err)
		return
	}

	funnel, err := h.funnel(ctx, period)
	if err != nil {
		DBError(c, err)
		return
	}

	stuck, err := h.stuckOrders(ctx)
	if err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, adminOverview{
		Period:      periodDTO{From: period.FromStr, To: period.ToStr},
		Kpis:        kpis,
		Series:      series,
		ByStatus:    byStatus,
		Funnel:      funnel,
		StuckOrders: stuck,
		Alerts:      overviewAlerts(stuck),
	})
}

// avgTicket protege contra a divisão por zero do painel recém-instalado. Zero
// pedidos tem ticket médio ZERO, não NaN — `NaN` não é JSON válido e derrubaria
// a tela inteira em vez de mostrar um painel vazio.
func avgTicket(totalCents int64, count int) int64 {
	if count == 0 {
		return 0
	}
	return totalCents / int64(count)
}

// revenueSeries devolve um ponto por dia do período, SEM buracos.
//
// PORQUÊ preencher os dias vazios aqui e não no front: um gráfico que pula o
// domingo sem venda desenha a semana comprimida e sugere continuidade onde não
// houve. O eixo do tempo é responsabilidade de quem conhece o período.
//
// A receita é ancorada em `paid_at`, não em `created_at`: venda é dinheiro que
// entrou. Pedido criado e não pago não é faturamento, e contá-lo faria o painel
// mostrar receita que não existe.
func (h *AdminDashboardHandler) revenueSeries(ctx context.Context, p adminPeriod) ([]timePoint, int64, int, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT (paid_at AT TIME ZONE 'UTC')::date AS d,
		       COALESCE(SUM(total), 0)::float8,
		       COUNT(*)
		  FROM orders
		 WHERE paid_at >= $1 AND paid_at < $2
		   AND status <> 'cancelled'
		 GROUP BY d
	`, p.From, p.To)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	type agg struct {
		cents  int64
		orders int
	}
	byDay := make(map[string]agg)
	var totalCents int64
	var totalOrders int
	for rows.Next() {
		var d time.Time
		var sum float64
		var n int
		if err := rows.Scan(&d, &sum, &n); err != nil {
			return nil, 0, 0, err
		}
		cents := toCents(sum)
		byDay[d.Format("2006-01-02")] = agg{cents: cents, orders: n}
		totalCents += cents
		totalOrders += n
	}
	// rows.Err() distingue "não houve venda" de "a leitura morreu no meio".
	// Sem isso um resultado truncado chegaria ao dono como queda de faturamento.
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}

	out := make([]timePoint, 0, p.Days())
	for d := p.From; d.Before(p.To); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		a := byDay[key] // zero value = dia sem venda, que é exatamente o ponto
		out = append(out, timePoint{Date: key, ValueCents: a.cents, Orders: a.orders})
	}
	return out, totalCents, totalOrders, nil
}

// rollingKpis calcula hoje/7 dias/30 dias e os períodos anteriores equivalentes
// numa ÚNICA varredura, com FILTER.
//
// PORQUÊ uma consulta só e não seis: seis consultas leem o mesmo intervalo de
// índice seis vezes. Com FILTER o Postgres lê os últimos 60 dias uma vez e
// distribui as linhas pelos seis baldes. Estas KPIs NÃO respeitam o ?from&to
// de propósito — "vendas de hoje" é sempre hoje, independente do filtro que o
// dono aplicou no gráfico abaixo.
func (h *AdminDashboardHandler) rollingKpis(ctx context.Context) (overviewKpis, error) {
	var k overviewKpis
	var today, week, month, todayPrev, weekPrev, monthPrev float64

	err := h.db.QueryRowContext(ctx, `
		WITH base AS (
		  SELECT paid_at, total
		    FROM orders
		   WHERE paid_at >= now() - interval '60 days'
		     AND status <> 'cancelled'
		)
		SELECT
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= date_trunc('day', now())), 0)::float8,
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= now() - interval '7 days'), 0)::float8,
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= now() - interval '30 days'), 0)::float8,
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= date_trunc('day', now()) - interval '1 day'
		                                AND paid_at <  date_trunc('day', now())), 0)::float8,
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= now() - interval '14 days'
		                                AND paid_at <  now() - interval '7 days'), 0)::float8,
		  COALESCE(SUM(total) FILTER (WHERE paid_at >= now() - interval '60 days'
		                                AND paid_at <  now() - interval '30 days'), 0)::float8
		  FROM base
	`).Scan(&today, &week, &month, &todayPrev, &weekPrev, &monthPrev)
	if err != nil {
		return k, err
	}

	k.TodayCents = toCents(today)
	k.WeekCents = toCents(week)
	k.MonthCents = toCents(month)
	k.TodayPrevCents = toCents(todayPrev)
	k.WeekPrevCents = toCents(weekPrev)
	k.MonthPrevCents = toCents(monthPrev)
	return k, nil
}

// previousPeriodTotals mede a janela imediatamente anterior, do MESMO tamanho.
// É o que dá sentido ao "+12% vs. período anterior" no topo da tela.
func (h *AdminDashboardHandler) previousPeriodTotals(ctx context.Context, p adminPeriod) (int64, int, error) {
	span := p.To.Sub(p.From)
	prevFrom := p.From.Add(-span)

	var sum float64
	var n int
	err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total), 0)::float8, COUNT(*)
		  FROM orders
		 WHERE paid_at >= $1 AND paid_at < $2
		   AND status <> 'cancelled'
	`, prevFrom, p.From).Scan(&sum, &n)
	if err != nil {
		return 0, 0, err
	}
	return toCents(sum), n, nil
}

// byStatus conta o ESTADO ATUAL dos pedidos criados no período.
//
// Ancorado em created_at (e não paid_at como a receita): a pergunta aqui é
// "dos pedidos que entraram, onde eles estão", e pedido cancelado ou nunca pago
// não tem paid_at para ser ancorado.
func (h *AdminDashboardHandler) byStatus(ctx context.Context, p adminPeriod) ([]statusBucket, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT status::text, COUNT(*), COALESCE(SUM(total), 0)::float8
		  FROM orders
		 WHERE created_at >= $1 AND created_at < $2
		 GROUP BY status
	`, p.From, p.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Slice não-nil: `[]` é uma tabela vazia na tela; `null` faz o
	// `.map()` do front estourar. Painel novo não pode quebrar com zero pedidos.
	out := make([]statusBucket, 0, 6)
	for rows.Next() {
		var s string
		var n int
		var sum float64
		if err := rows.Scan(&s, &n, &sum); err != nil {
			return nil, err
		}
		out = append(out, statusBucket{Status: statusToWire(s), Count: n, ValueCents: toCents(sum)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Status < out[j].Status })
	return out, nil
}

// funnel mede criado → confirmado a partir do CICLO DE VIDA DO PEDIDO.
//
// ⚠️ LIMITE CONHECIDO (documentado em docs/admin-dashboard-api.md): a intenção
// de pagamento vive no payment-service, em outro banco. O order-service não
// enxerga tentativa de cartão recusada nem Pix que expirou como eventos
// separados — ele só vê o pedido. Então:
//
//	created   = pedidos criados no período
//	confirmed = os que chegaram a ter paid_at
//	failed    = cancelados que NUNCA foram pagos
//	expired   = ainda em pending_payment e velhos demais para virar venda (24h)
//
// A conversão que o front deriva (confirmed/created) é a real; o recorte
// failed/expired é aproximado e conservador. A alternativa seria o painel
// afirmar zero, que é pior: zero parece informação.
const expiryHours = 24

func (h *AdminDashboardHandler) funnel(ctx context.Context, p adminPeriod) (paymentFunnel, error) {
	f := paymentFunnel{ByMethod: make([]funnelMethod, 0, 3)}

	err := h.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE paid_at IS NOT NULL),
		       COUNT(*) FILTER (WHERE status = 'cancelled' AND paid_at IS NULL),
		       COUNT(*) FILTER (WHERE status = 'pending_payment'
		                          AND created_at < now() - ($3 || ' hours')::interval)
		  FROM orders
		 WHERE created_at >= $1 AND created_at < $2
	`, p.From, p.To, expiryHours).Scan(&f.Created, &f.Confirmed, &f.Failed, &f.Expired)
	if err != nil {
		return f, err
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT payment_method::text,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE paid_at IS NOT NULL)
		  FROM orders
		 WHERE created_at >= $1 AND created_at < $2
		 GROUP BY payment_method
		 ORDER BY payment_method::text
	`, p.From, p.To)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var m funnelMethod
		if err := rows.Scan(&m.Method, &m.Created, &m.Confirmed); err != nil {
			return f, err
		}
		f.ByMethod = append(f.ByMethod, m)
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	return f, nil
}

// stuckOrders — pago e parado. É o número que denuncia operação parada.
//
// PORQUÊ não respeita ?from&to: um pedido travado há 3 dias continua travado
// hoje, e filtrá-lo pelo período do gráfico o esconderia justamente de quem
// abriu o painel para descobrir o que está errado AGORA.
//
// `stuckForHours` é calculado no SERVIDOR: o relógio do navegador pode estar
// meses errado, e "atrasado" é decisão de negócio, não de renderização.
func (h *AdminDashboardHandler) stuckOrders(ctx context.Context) ([]stuckOrder, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id::text, number, status::text, paid_at,
		       total::float8,
		       COALESCE(customer_name, ''),
		       EXTRACT(EPOCH FROM (now() - paid_at)) / 3600.0
		  FROM orders
		 WHERE status = 'paid'
		   AND paid_at IS NOT NULL
		   AND paid_at < now() - ($1 || ' hours')::interval
		 ORDER BY paid_at ASC
		 LIMIT $2
	`, stuckThresholdHours, maxStuckOrders)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]stuckOrder, 0)
	for rows.Next() {
		var s stuckOrder
		var paidAt time.Time
		var total, hours float64
		if err := rows.Scan(&s.OrderID, &s.OrderNumber, &s.Status, &paidAt,
			&total, &s.CustomerName, &hours); err != nil {
			return nil, err
		}
		s.Status = statusToWire(s.Status)
		s.PaidAt = paidAt.UTC().Format(time.RFC3339)
		s.TotalCents = toCents(total)
		s.StuckForHours = math.Round(hours*10) / 10
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// overviewAlerts transforma o que o order-service enxerga em alerta acionável.
//
// A severidade é decidida AQUI, não no front: "quantas horas travado é grave"
// é regra de negócio, e regra de negócio duplicada no navegador diverge na
// primeira mudança.
func overviewAlerts(stuck []stuckOrder) []alert {
	out := make([]alert, 0)
	if len(stuck) == 0 {
		return out
	}

	// O mais antigo dita a gravidade: 40 pedidos parados há 5h é fila; 1 pedido
	// parado há 2 dias é mercadoria vendida que ninguém separou.
	worst := 0.0
	for _, s := range stuck {
		if s.StuckForHours > worst {
			worst = s.StuckForHours
		}
	}
	sev := "warn"
	if worst >= 24 || len(stuck) >= 20 {
		sev = "critical"
	}

	out = append(out, alert{
		ID:       "stuck-orders",
		Severity: sev,
		Title:    fmt.Sprintf("%d pedido(s) pago(s) sem separação", len(stuck)),
		Detail: fmt.Sprintf("O mais antigo está parado há %.1fh. Pagamento confirmado que não vira separação costuma ser fila do outbox parada.",
			worst),
		Source:  "order-service",
		FiredAt: time.Now().UTC().Format(time.RFC3339),
		Href:    "/admin/observabilidade",
	})
	return out
}

// ---------------------------------------------------------------------------
// Desempenho de vendedores
// ---------------------------------------------------------------------------

type sellerPerformance struct {
	SellerID   string `json:"sellerId"`
	SellerName string `json:"sellerName"`
	StoreID    string `json:"storeId"`
	StoreName  string `json:"storeName"`

	TotalCents     int64 `json:"totalCents"`
	OrderCount     int   `json:"orderCount"`
	AvgTicketCents int64 `json:"avgTicketCents"`
	// Frações 0..1 (não 0..100) — convenção do contrato para todo percentual.
	AvgDiscountPct   float64     `json:"avgDiscountPct"`
	AvgMarginPct     float64     `json:"avgMarginPct"`
	ManagerApprovals int         `json:"managerApprovals"`
	Series           []timePoint `json:"series"`
}

type storeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type sellerPerformanceReport struct {
	Period  periodDTO           `json:"period"`
	Stores  []storeRef          `json:"stores"`
	Sellers []sellerPerformance `json:"sellers"`
}

// SellersPerformance GET /api/v1/admin/sellers/performance?from&to&storeId
func (h *AdminDashboardHandler) SellersPerformance(c *gin.Context) {
	period, ok := parsePeriod(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	storeFilter := strings.TrimSpace(c.Query("storeId"))

	// Margem e custo de aquisição saem daqui. Nenhum intermediário guarda.
	c.Header("Cache-Control", "no-store")

	// 1) Agregado por operador. Só canal 'balcao': venda web não tem vendedor,
	//    e incluí-la criaria uma linha fantasma "sem operador" com o
	//    faturamento inteiro do site, que esconderia todos os vendedores reais.
	rows, err := h.db.QueryContext(ctx, `
		SELECT o.operator_id,
		       COALESCE(o.store_id, ''),
		       COUNT(*),
		       COALESCE(SUM(o.total), 0)::float8,
		       COALESCE(SUM(o.discount_amount), 0)::float8,
		       COUNT(*) FILTER (WHERE o.approval_status IN ('pending','approved','rejected'))
		  FROM orders o
		 WHERE o.channel = 'balcao'
		   AND o.paid_at >= $1 AND o.paid_at < $2
		   AND o.status <> 'cancelled'
		   AND o.operator_id IS NOT NULL
		   AND ($3 = '' OR o.store_id = $3)
		 GROUP BY o.operator_id, o.store_id
	`, period.From, period.To, storeFilter)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	type row struct {
		operatorID string
		storeID    string
		orders     int
		totalCents int64
		discCents  int64
		approvals  int
	}
	agg := make([]row, 0, 16)
	for rows.Next() {
		var r row
		var total, disc float64
		if err := rows.Scan(&r.operatorID, &r.storeID, &r.orders, &total, &disc, &r.approvals); err != nil {
			DBError(c, err)
			return
		}
		r.totalCents = toCents(total)
		r.discCents = toCents(disc)
		agg = append(agg, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	// 2) Série diária por operador — uma consulta para todos, não uma por
	//    vendedor. Com 30 vendedores, o laço seriam 30 varreduras do mesmo
	//    intervalo de índice.
	seriesByOp, err := h.sellerSeries(ctx, period, storeFilter)
	if err != nil {
		DBError(c, err)
		return
	}

	// 3) Margem — exige custo, que só o catalog-service tem.
	marginByOp, err := h.marginByOperator(ctx, period, storeFilter)
	if err != nil {
		// Falha ao falar com o catálogo NÃO derruba a tela inteira: o dono
		// ainda precisa ver faturamento e desconto. A margem sai zerada.
		//
		// O log é OBRIGATÓRIO aqui e já faltou uma vez: sem ele, "a margem de
		// todo mundo é 0%" chega ao dono como se fosse um fato do negócio, e
		// não há nada no servidor explicando que a consulta de custo falhou.
		// Zero silencioso num número de margem é pior que erro visível.
		slog.Error("margem indisponível: consulta de custo ao catalog-service falhou",
			"request_id", c.GetString("request_id"),
			"error", err.Error())
		marginByOp = map[string]float64{}
	}

	// 4) Nomes. Diretório indisponível não é motivo para 500: a tela degrada
	//    para o id, que continua identificando a linha.
	directory := map[string]authclient.OperatorInfo{}
	if h.operators != nil {
		d, derr := h.operators.Operators(ctx, c.GetHeader("Authorization"))
		if derr == nil {
			directory = d
		} else {
			// Mesmo raciocínio do log da margem: sem esta linha, "todos os
			// vendedores viraram UUID na tela" não tem explicação nenhuma no
			// servidor, e o sintoma parece bug de front.
			slog.Warn("diretório de operadores indisponível — a tela vai exibir ids em vez de nomes",
				"request_id", c.GetString("request_id"),
				"error", derr.Error())
		}
	}

	sellers := make([]sellerPerformance, 0, len(agg))
	storeSet := map[string]string{}
	for _, r := range agg {
		info := directory[r.operatorID]
		storeID := r.storeID
		if storeID == "" {
			storeID = info.StoreID
		}
		name := info.Name
		if name == "" {
			name = r.operatorID
		}
		storeName := info.StoreName
		if storeName == "" {
			storeName = storeID
		}
		if storeID != "" {
			storeSet[storeID] = storeName
		}

		// Desconto médio como fração do BRUTO (total + desconto concedido).
		// Sobre o líquido daria um número maior e lisonjeiro: 20 de desconto em
		// 100 de bruto é 20%, não 25%.
		gross := r.totalCents + r.discCents
		var discPct float64
		if gross > 0 {
			discPct = float64(r.discCents) / float64(gross)
		}

		series := seriesByOp[r.operatorID]
		if series == nil {
			series = make([]timePoint, 0)
		}

		sellers = append(sellers, sellerPerformance{
			SellerID:         r.operatorID,
			SellerName:       name,
			StoreID:          storeID,
			StoreName:        storeName,
			TotalCents:       r.totalCents,
			OrderCount:       r.orders,
			AvgTicketCents:   avgTicket(r.totalCents, r.orders),
			AvgDiscountPct:   round4(discPct),
			AvgMarginPct:     round4(marginByOp[r.operatorID]),
			ManagerApprovals: r.approvals,
			Series:           series,
		})
	}
	// Maior faturamento primeiro — é a ordem que a tela usa e a que o dono espera.
	sort.Slice(sellers, func(i, j int) bool { return sellers[i].TotalCents > sellers[j].TotalCents })

	stores := make([]storeRef, 0, len(storeSet))
	for id, name := range storeSet {
		stores = append(stores, storeRef{ID: id, Name: name})
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].Name < stores[j].Name })

	c.JSON(http.StatusOK, sellerPerformanceReport{
		Period:  periodDTO{From: period.FromStr, To: period.ToStr},
		Stores:  stores,
		Sellers: sellers,
	})
}

// round4 corta a fração em 4 casas (0,0342 = 3,42%). Sem isso sai
// 0.034200000000000005 no JSON — ruído que não significa nada e que faz dois
// relatórios do mesmo dado parecerem diferentes.
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func (h *AdminDashboardHandler) sellerSeries(ctx context.Context, p adminPeriod, storeFilter string) (map[string][]timePoint, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT operator_id,
		       (paid_at AT TIME ZONE 'UTC')::date AS d,
		       COALESCE(SUM(total), 0)::float8,
		       COUNT(*)
		  FROM orders
		 WHERE channel = 'balcao'
		   AND paid_at >= $1 AND paid_at < $2
		   AND status <> 'cancelled'
		   AND operator_id IS NOT NULL
		   AND ($3 = '' OR store_id = $3)
		 GROUP BY operator_id, d
		 ORDER BY operator_id, d
	`, p.From, p.To, storeFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]timePoint{}
	for rows.Next() {
		var op string
		var d time.Time
		var sum float64
		var n int
		if err := rows.Scan(&op, &d, &sum, &n); err != nil {
			return nil, err
		}
		out[op] = append(out[op], timePoint{
			Date: d.Format("2006-01-02"), ValueCents: toCents(sum), Orders: n,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// maxCostProducts — teto de produtos distintos consultados no catálogo por
// chamada. Acima disso a margem é calculada só sobre os produtos que couberam.
//
// PORQUÊ um teto: sem ele, um período longo numa loja com catálogo grande
// dispararia centenas de requisições ao catalog-service a cada refresh do
// painel. 4.000 é ~20 lotes de 200 e cobre com folga o sortimento realmente
// vendido num ano de balcão.
const maxCostProducts = 4000

// costBatchSize casa com o teto do catalog-service (maxCostBatch=200). Lote
// maior seria recusado com 400.
const costBatchSize = 200

// marginByOperator devolve a margem média realizada por operador, como fração.
//
// ⚠️ SEGURANÇA (requisito do contrato): o custo unitário NUNCA sai deste
// método. Ele entra na conta e o que volta é uma razão agregada. Custo de
// aquisição no navegador entrega a estrutura de compra da Utilar para qualquer
// um com o DevTools aberto — inclusive para o próprio vendedor.
//
// A margem é ponderada por receita (Σreceita − ΣCMV) / Σreceita, não a média
// das margens por item: a média simples daria o mesmo peso a um parafuso e a
// uma betoneira, e o número serve para decidir desconto em dinheiro.
//
// A receita do item é o valor de LISTA (quantidade × unit_price). O desconto do
// pedido é rateado proporcionalmente, porque quem dá 10% no pedido dá 10% na
// margem de cada item.
func (h *AdminDashboardHandler) marginByOperator(ctx context.Context, p adminPeriod, storeFilter string) (map[string]float64, error) {
	if h.costs == nil {
		return map[string]float64{}, nil
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT o.operator_id,
		       oi.product_id::text,
		       SUM(oi.quantity)::float8,
		       SUM(oi.quantity * oi.unit_price)::float8,
		       -- fator de desconto do pedido aplicado ao item, rateado
		       SUM(oi.quantity * oi.unit_price *
		           CASE WHEN o.subtotal > 0
		                THEN GREATEST(0, 1 - (o.discount_amount / o.subtotal))
		                ELSE 1 END)::float8
		  FROM orders o
		  JOIN order_items oi ON oi.order_id = o.id
		 WHERE o.channel = 'balcao'
		   AND o.paid_at >= $1 AND o.paid_at < $2
		   AND o.status <> 'cancelled'
		   AND o.operator_id IS NOT NULL
		   AND ($3 = '' OR o.store_id = $3)
		 GROUP BY o.operator_id, oi.product_id
		 LIMIT $4
	`, p.From, p.To, storeFilter, maxCostProducts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type line struct {
		operator  string
		productID string
		qty       float64
		netRev    float64
	}
	lines := make([]line, 0, 64)
	productSet := map[string]struct{}{}
	for rows.Next() {
		var l line
		var listRev float64
		if err := rows.Scan(&l.operator, &l.productID, &l.qty, &listRev, &l.netRev); err != nil {
			return nil, err
		}
		lines = append(lines, l)
		productSet[l.productID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return map[string]float64{}, nil
	}

	ids := make([]string, 0, len(productSet))
	for id := range productSet {
		ids = append(ids, id)
	}
	sort.Strings(ids) // ordem estável: deixa o teste determinístico

	costs := make(map[string]float64, len(ids))
	for i := 0; i < len(ids); i += costBatchSize {
		end := i + costBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch, err := h.costs.Costs(ctx, ids[i:end])
		if err != nil {
			return nil, err
		}
		for k, v := range batch {
			costs[strings.ToLower(k)] = v
		}
	}

	type acc struct{ revenue, cogs float64 }
	byOp := map[string]*acc{}
	for _, l := range lines {
		cost, ok := costs[strings.ToLower(l.productID)]
		// Produto sem custo cadastrado fica FORA da conta inteira (receita e
		// CMV). Incluir a receita e assumir custo zero produziria margem
		// inflada — o erro mais perigoso possível aqui, porque faria o gerente
		// enxergar folga para desconto onde não existe.
		if !ok {
			continue
		}
		a := byOp[l.operator]
		if a == nil {
			a = &acc{}
			byOp[l.operator] = a
		}
		a.revenue += l.netRev
		a.cogs += cost * l.qty
	}

	out := make(map[string]float64, len(byOp))
	for op, a := range byOp {
		if a.revenue <= 0 {
			continue
		}
		out[op] = (a.revenue - a.cogs) / a.revenue
	}
	return out, nil
}
