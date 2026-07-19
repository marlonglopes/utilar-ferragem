// Observabilidade agregada para o painel — GET /api/v1/admin/observability.
//
// # Por que um agregado JSON em vez de o front falar Prometheus
//
// O `pkg/metrics` já expõe tudo isto em `/metrics`, em formato Prometheus. Três
// razões para não mandar o navegador ler aquilo direto:
//
//  1. `/metrics` é fail-closed por METRICS_TOKEN (ver pkg/metrics.Handler). Pôr
//     esse token no bundle do SPA o entregaria a qualquer visitante — e
//     `/metrics` expõe volume financeiro, taxa de recusa de cartão e topologia
//     do sistema. O token fica no servidor e o navegador nunca o vê.
//  2. Seriam 4 requisições cross-origin (uma por serviço) e um parser de texto
//     Prometheus em TypeScript, mantido à mão.
//  3. Os LIMIARES são regra de negócio ("outbox parado há 15min é incidente").
//     Regra de negócio duplicada no navegador diverge na primeira mudança, e
//     aqui divergir significa o painel dizer "ok" durante um incidente.
//
// # Por que no catalog-service
//
// O contrato original (docs/admin-dashboard-api.md) colocava esta rota no
// payment-service. Ela não pertence a nenhum serviço em particular — é sobre
// TODOS eles — e o payment-service é o serviço mais sensível do sistema; dar a
// ele a função de cliente HTTP dos outros três amplia a superfície de quem
// move dinheiro sem ganho nenhum. O agregador só LÊ métricas, então mora no
// serviço de menor privilégio que já tem um grupo /admin. O doc foi corrigido.
//
// # Por que cache
//
// A tela chama esta rota a cada 30s enquanto está aberta, e cada chamada
// dispara 4 scrapes + 4 healthchecks. Sem cache, dois admins com a aba aberta
// dobram o tráfego interno para sempre. O snapshot é compartilhado.
package handler

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Limiares — regra de negócio, decidida no servidor
// ---------------------------------------------------------------------------

// Fila do outbox. IDADE PESA MAIS QUE TAMANHO, e essa é a decisão central:
// 500 eventos escoando em 10s é uma fila saudável num pico de venda; 3 eventos
// parados há uma hora significa que o relay morreu e que todo pagamento
// confirmado desde então não virou pedido pago.
const (
	outboxWarnAgeSeconds     = 120
	outboxCriticalAgeSeconds = 900
	outboxWarnPending        = 100
	outboxCriticalPending    = 500
)

// Saúde de serviço. errorRate é fração de 5xx.
const (
	errorRateWarn     = 0.01 // 1% de 5xx já é anormal para este sistema
	errorRateCritical = 0.05
	latencyWarnMs     = 1000
	latencyCriticalMs = 3000
)

// snapshotTTL — por quanto tempo o snapshot é reaproveitado. Menor que o
// intervalo de polling da tela (30s) para que um refresh manual do dono no meio
// de um incidente traga dado novo, e grande o bastante para que N abas abertas
// custem o mesmo que uma.
const snapshotTTL = 15 * time.Second

// scrapeTimeout — um serviço morto não pode fazer o painel de diagnóstico
// pendurar. Timeout curto e o serviço aparece como `up: false`, que é
// exatamente a informação que o dono abriu a tela para ver.
const scrapeTimeout = 3 * time.Second

// latencyHistoryPoints — tamanho da sparkline (~30 pontos, contrato).
const latencyHistoryPoints = 30

// ---------------------------------------------------------------------------
// Tipos de resposta — espelham app/src/lib/adminTypes.ts
// ---------------------------------------------------------------------------

type ServiceHealth struct {
	Name   string `json:"name"` // auth | catalog | order | payment
	Status string `json:"status"`
	Up     bool   `json:"up"`

	P50Ms     float64 `json:"p50Ms"`
	P95Ms     float64 `json:"p95Ms"`
	P99Ms     float64 `json:"p99Ms"`
	ErrorRate float64 `json:"errorRate"`
	RPM       float64 `json:"rpm"`
	UptimePct float64 `json:"uptimePct"`
	Version   string  `json:"version"`

	// LatencySeries nunca é nil: `null` faz o componente de sparkline do front
	// estourar, e serviço recém-subido (sem histórico) é o caso normal, não o
	// excepcional.
	LatencySeries []float64 `json:"latencySeries"`
}

type OutboxStats struct {
	Pending            int     `json:"pending"`
	Failed             int     `json:"failed"`
	OldestAgeSeconds   float64 `json:"oldestAgeSeconds"`
	PublishedPerMinute float64 `json:"publishedPerMinute"`
	Severity           string  `json:"severity"`
}

type ObservabilityAlert struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Source   string `json:"source"`
	FiredAt  string `json:"firedAt"`
	Href     string `json:"href,omitempty"`
}

type ObservabilitySnapshot struct {
	CollectedAt string               `json:"collectedAt"`
	Services    []ServiceHealth      `json:"services"`
	Outbox      OutboxStats          `json:"outbox"`
	Alerts      []ObservabilityAlert `json:"alerts"`
}

// ---------------------------------------------------------------------------
// Alvos
// ---------------------------------------------------------------------------

// ServiceTarget é um serviço a ser sondado.
type ServiceTarget struct {
	Name    string // auth | catalog | order | payment
	BaseURL string
}

// ObservabilityHandler agrega /metrics + /health dos serviços.
type ObservabilityHandler struct {
	targets      []ServiceTarget
	metricsToken string
	client       *http.Client

	mu       sync.Mutex
	snapshot *ObservabilitySnapshot
	fetched  time.Time
	// history guarda os últimos p95 por serviço para a sparkline. Vive em
	// memória do processo de propósito: é dado de tela, descartável, e
	// persistir sparkline em banco seria escrever a cada 15s para sempre.
	history map[string][]float64
	// firstSeen alimenta uptimePct sem depender de um TSDB. Ver uptime().
	firstSeen map[string]time.Time
	upSamples map[string]int
	okSamples map[string]int
}

func NewObservabilityHandler(targets []ServiceTarget, metricsToken string) *ObservabilityHandler {
	return &ObservabilityHandler{
		targets:      targets,
		metricsToken: metricsToken,
		client:       &http.Client{Timeout: scrapeTimeout},
		history:      map[string][]float64{},
		firstSeen:    map[string]time.Time{},
		upSamples:    map[string]int{},
		okSamples:    map[string]int{},
	}
}

// Snapshot GET /api/v1/admin/observability
func (h *ObservabilityHandler) Snapshot(c *gin.Context) {
	// Topologia interna e taxa de erro não podem ficar em cache de proxy.
	c.Header("Cache-Control", "no-store")

	h.mu.Lock()
	fresh := h.snapshot != nil && time.Since(h.fetched) < snapshotTTL
	if fresh {
		snap := *h.snapshot
		h.mu.Unlock()
		c.JSON(http.StatusOK, snap)
		return
	}
	h.mu.Unlock()

	snap := h.collect(c.Request.Context())

	h.mu.Lock()
	h.snapshot = &snap
	h.fetched = time.Now()
	h.mu.Unlock()

	c.JSON(http.StatusOK, snap)
}

// collect sonda todos os alvos EM PARALELO.
//
// Em série, quatro serviços lentos somariam quatro timeouts (12s) e o painel
// que existe para diagnosticar lentidão seria ele próprio a coisa mais lenta
// da tela. Em paralelo o pior caso é um timeout.
func (h *ObservabilityHandler) collect(ctx context.Context) ObservabilitySnapshot {
	type result struct {
		health ServiceHealth
		outbox *OutboxStats
	}
	results := make([]result, len(h.targets))

	var wg sync.WaitGroup
	for i, t := range h.targets {
		wg.Add(1)
		go func(i int, t ServiceTarget) {
			defer wg.Done()
			health, outbox := h.probe(ctx, t)
			results[i] = result{health: health, outbox: outbox}
		}(i, t)
	}
	wg.Wait()

	snap := ObservabilitySnapshot{
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
		Services:    make([]ServiceHealth, 0, len(results)),
		Alerts:      make([]ObservabilityAlert, 0),
	}
	for _, r := range results {
		snap.Services = append(snap.Services, r.health)
		// O outbox só existe no payment-service. Quem devolver estatística de
		// outbox, vence — não somamos: duas filas somadas esconderiam qual
		// delas parou.
		if r.outbox != nil {
			snap.Outbox = *r.outbox
		}
	}
	if snap.Outbox.Severity == "" {
		snap.Outbox.Severity = outboxSeverity(snap.Outbox.Pending, snap.Outbox.OldestAgeSeconds)
	}
	sort.Slice(snap.Services, func(i, j int) bool { return snap.Services[i].Name < snap.Services[j].Name })
	snap.Alerts = buildAlerts(snap.Services, snap.Outbox)
	return snap
}

// probe sonda um serviço: /health diz se está de pé, /metrics diz como está.
func (h *ObservabilityHandler) probe(ctx context.Context, t ServiceTarget) (ServiceHealth, *OutboxStats) {
	sh := ServiceHealth{
		Name:          t.Name,
		LatencySeries: []float64{},
		UptimePct:     0,
		Version:       "unknown",
	}

	sh.Up = h.ping(ctx, t.BaseURL)

	var outbox *OutboxStats
	if sh.Up {
		if fams, err := h.scrape(ctx, t.BaseURL); err == nil {
			sh.P50Ms, sh.P95Ms, sh.P99Ms = latencyQuantiles(fams)
			sh.ErrorRate, sh.RPM = errorRateAndRPM(fams, processUptimeSeconds(fams))
			if v := metricLabel(fams, "utilar_build_info", "version"); v != "" {
				sh.Version = v
			}
			outbox = outboxFrom(fams)
		}
	}

	sh.UptimePct = h.recordUptime(t.Name, sh.Up)
	sh.LatencySeries = h.recordLatency(t.Name, sh.P95Ms, sh.Up)
	sh.Status = serviceSeverity(sh)
	return sh, outbox
}

func (h *ObservabilityHandler) ping(ctx context.Context, baseURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, scrapeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// scrape lê /metrics com o METRICS_TOKEN. Sem token configurado, a rota do
// outro lado responde 404 (fail-closed do pkg/metrics) e o serviço aparece de
// pé mas sem números — que é honesto: "não consigo medir" ≠ "está tudo bem".
func (h *ObservabilityHandler) scrape(ctx context.Context, baseURL string) (map[string][]sample, error) {
	if h.metricsToken == "" {
		return nil, fmt.Errorf("METRICS_TOKEN não configurado")
	}
	ctx, cancel := context.WithTimeout(ctx, scrapeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.metricsToken)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/metrics devolveu %d", resp.StatusCode)
	}
	// Teto de leitura: /metrics de um serviço com cardinalidade explodida pode
	// vir com dezenas de MB, e o painel não pode virar o vetor de OOM.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parsePrometheus(string(body)), nil
}

// recordUptime aproxima disponibilidade pela fração de sondagens em que o
// serviço respondeu.
//
// ⚠️ É uma aproximação DELIBERADA e o painel não deve ser lido como SLA: a
// janela é a vida do processo do catalog-service, e reiniciar o catalog zera a
// conta. Uptime de verdade exige TSDB (Prometheus + retenção), que é o passo
// seguinte em docs/observability-alerts.md. Melhor um número honesto e limitado
// que um campo inventado — e melhor que 100% fixo, que mentiria.
func (h *ObservabilityHandler) recordUptime(name string, up bool) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.firstSeen[name]; !ok {
		h.firstSeen[name] = time.Now()
	}
	h.upSamples[name]++
	if up {
		h.okSamples[name]++
	}
	return math.Round(float64(h.okSamples[name])/float64(h.upSamples[name])*10000) / 10000 * 100
}

func (h *ObservabilityHandler) recordLatency(name string, p95 float64, up bool) []float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if up {
		h.history[name] = append(h.history[name], math.Round(p95*10)/10)
		if len(h.history[name]) > latencyHistoryPoints {
			h.history[name] = h.history[name][len(h.history[name])-latencyHistoryPoints:]
		}
	}
	// Cópia: devolver o slice interno deixaria o JSON sendo serializado
	// enquanto outra goroutine faz append no mesmo array.
	out := make([]float64, len(h.history[name]))
	copy(out, h.history[name])
	return out
}

// ---------------------------------------------------------------------------
// Severidade
// ---------------------------------------------------------------------------

func outboxSeverity(pending int, oldestAgeSeconds float64) string {
	switch {
	case oldestAgeSeconds >= outboxCriticalAgeSeconds || pending >= outboxCriticalPending:
		return "critical"
	case oldestAgeSeconds >= outboxWarnAgeSeconds || pending >= outboxWarnPending:
		return "warn"
	default:
		return "ok"
	}
}

func serviceSeverity(s ServiceHealth) string {
	if !s.Up {
		return "critical"
	}
	switch {
	case s.ErrorRate >= errorRateCritical || s.P95Ms >= latencyCriticalMs:
		return "critical"
	case s.ErrorRate >= errorRateWarn || s.P95Ms >= latencyWarnMs:
		return "warn"
	default:
		return "ok"
	}
}

func buildAlerts(services []ServiceHealth, outbox OutboxStats) []ObservabilityAlert {
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]ObservabilityAlert, 0)

	for _, s := range services {
		if s.Status == "ok" {
			continue
		}
		detail := fmt.Sprintf("p95 %.0f ms, %.2f%% de 5xx.", s.P95Ms, s.ErrorRate*100)
		title := fmt.Sprintf("%s degradado", s.Name)
		if !s.Up {
			title = fmt.Sprintf("%s fora do ar", s.Name)
			detail = "O /health não respondeu dentro do timeout."
		}
		out = append(out, ObservabilityAlert{
			ID:       "service-" + s.Name,
			Severity: s.Status,
			Title:    title,
			Detail:   detail,
			Source:   s.Name + "-service",
			FiredAt:  now,
			Href:     "/admin/observabilidade",
		})
	}

	if outbox.Severity != "ok" && outbox.Severity != "" {
		out = append(out, ObservabilityAlert{
			ID:       "outbox-backlog",
			Severity: outbox.Severity,
			Title:    "Fila do outbox acumulando",
			Detail: fmt.Sprintf(
				"%d evento(s) pendente(s); o mais antigo está parado há %.0fs. Enquanto isso, pagamento confirmado não vira pedido pago.",
				outbox.Pending, outbox.OldestAgeSeconds),
			Source:  "payment-service",
			FiredAt: now,
			Href:    "/admin",
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Parser do texto Prometheus
// ---------------------------------------------------------------------------

// sample é uma linha do formato de exposição: nome, labels e valor.
type sample struct {
	labels map[string]string
	value  float64
}

// parsePrometheus lê o formato de exposição de texto.
//
// PORQUÊ um parser à mão e não expfmt do client_golang: precisamos de um punhado
// de séries conhecidas (histograma de latência, contador de request, gauges do
// outbox), e o expfmt traria a máquina inteira de decodificação de um formato
// que aqui é lido de um par de serviços internos. O parser é tolerante por
// design: linha que não entende é ignorada, porque o painel tem que continuar
// mostrando o que entendeu quando um serviço adicionar uma métrica nova.
func parsePrometheus(body string) map[string][]sample {
	out := map[string][]sample{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// nome{labels} valor [timestamp]
		var name, rest string
		if i := strings.IndexAny(line, "{ "); i > 0 {
			name, rest = line[:i], line[i:]
		} else {
			continue
		}

		labels := map[string]string{}
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "{") {
			end := strings.LastIndex(rest, "}")
			if end < 0 {
				continue
			}
			labels = parseLabels(rest[1:end])
			rest = strings.TrimSpace(rest[end+1:])
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		out[name] = append(out[name], sample{labels: labels, value: v})
	}
	return out
}

// parseLabels lê `a="1",b="2"`. Valores com vírgula dentro de aspas são raros
// nas métricas do Utilar (a cardinalidade é fechada por design, ver
// pkg/metrics), mas o parser respeita as aspas mesmo assim — um label com
// vírgula quebraria a divisão ingênua e corromperia TODAS as séries daquela
// linha em silêncio.
func parseLabels(s string) map[string]string {
	out := map[string]string{}
	var key strings.Builder
	var val strings.Builder
	inVal, inQuote, escaped := false, false, false

	flush := func() {
		k := strings.TrimSpace(key.String())
		if k != "" {
			out[k] = val.String()
		}
		key.Reset()
		val.Reset()
		inVal = false
	}

	for _, r := range s {
		switch {
		case escaped:
			val.WriteRune(r)
			escaped = false
		case inQuote && r == '\\':
			escaped = true
		case r == '"':
			inQuote = !inQuote
		case inQuote:
			val.WriteRune(r)
		case r == '=':
			inVal = true
		case r == ',':
			flush()
		default:
			if inVal {
				val.WriteRune(r)
			} else {
				key.WriteRune(r)
			}
		}
	}
	flush()
	return out
}

// latencyQuantiles estima p50/p95/p99 a partir dos buckets cumulativos do
// histograma utilar_http_request_duration_seconds.
//
// ⚠️ É estimativa por INTERPOLAÇÃO LINEAR dentro do bucket — a mesma que o
// histogram_quantile do Prometheus faz, e com a mesma limitação: a precisão é a
// dos limites dos buckets. Com os buckets do pkg/metrics (…0.25, 0.5, 1, 2.5…),
// um p95 real de 700ms é reportado em algum ponto entre 500ms e 1s. Suficiente
// para "está lento?", insuficiente para SLA em milissegundos — e é por isso que
// os limiares de severidade são grossos (1s / 3s), não finos.
//
// ⚠️ Os contadores são CUMULATIVOS desde o boot do processo, não uma janela de
// 5 minutos como o contrato descreve. Uma janela real exige duas coletas e uma
// subtração (é o que o Prometheus faz com rate()). Registrado no doc.
func latencyQuantiles(fams map[string][]sample) (p50, p95, p99 float64) {
	buckets := fams["utilar_http_request_duration_seconds_bucket"]
	if len(buckets) == 0 {
		return 0, 0, 0
	}

	// Soma os buckets de todas as rotas: a pergunta da tela é sobre o SERVIÇO.
	totals := map[float64]float64{}
	var inf float64
	for _, s := range buckets {
		le := s.labels["le"]
		if le == "+Inf" {
			inf += s.value
			continue
		}
		b, err := strconv.ParseFloat(le, 64)
		if err != nil {
			continue
		}
		totals[b] += s.value
	}
	if inf == 0 {
		return 0, 0, 0
	}

	bounds := make([]float64, 0, len(totals))
	for b := range totals {
		bounds = append(bounds, b)
	}
	sort.Float64s(bounds)

	q := func(p float64) float64 {
		target := p * inf
		prevBound, prevCount := 0.0, 0.0
		for _, b := range bounds {
			count := totals[b]
			if count >= target {
				// Interpolação dentro do bucket.
				if count == prevCount {
					return b * 1000
				}
				frac := (target - prevCount) / (count - prevCount)
				return (prevBound + frac*(b-prevBound)) * 1000
			}
			prevBound, prevCount = b, count
		}
		// Caiu no +Inf: acima do maior bucket. Devolve o maior limite em vez de
		// +Inf, que não é JSON válido e derrubaria a tela.
		if len(bounds) > 0 {
			return bounds[len(bounds)-1] * 1000
		}
		return 0
	}
	return round1(q(0.50)), round1(q(0.95)), round1(q(0.99))
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// errorRateAndRPM deriva taxa de 5xx e requisições por minuto de
// utilar_http_requests_total.
//
// RPM é a média sobre a vida do processo (total / uptime), não a taxa
// instantânea. Um serviço de pé há dias com um pico agora mostra RPM baixo —
// limitação registrada no doc, junto com a de latencyQuantiles. As duas somem
// quando houver um Prometheus de verdade fazendo rate().
func errorRateAndRPM(fams map[string][]sample, uptimeSeconds float64) (errorRate, rpm float64) {
	var total, errors float64
	for _, s := range fams["utilar_http_requests_total"] {
		total += s.value
		if s.labels["status"] == "5xx" {
			errors += s.value
		}
	}
	if total > 0 {
		errorRate = errors / total
	}
	if uptimeSeconds > 0 {
		rpm = total / (uptimeSeconds / 60)
	}
	return math.Round(errorRate*10000) / 10000, round1(rpm)
}

// processUptimeSeconds usa process_start_time_seconds, exposto pelo
// ProcessCollector que o pkg/metrics já registra.
func processUptimeSeconds(fams map[string][]sample) float64 {
	s := fams["process_start_time_seconds"]
	if len(s) == 0 || s[0].value <= 0 {
		return 0
	}
	up := float64(time.Now().Unix()) - s[0].value
	if up < 0 {
		return 0
	}
	return up
}

func metricLabel(fams map[string][]sample, metric, label string) string {
	for _, s := range fams[metric] {
		if v, ok := s.labels[label]; ok {
			return v
		}
	}
	return ""
}

// outboxFrom extrai a fila do outbox. Só o payment-service publica estas
// séries; nos outros o retorno é nil (e não um zero, que seria indistinguível
// de "fila vazia e saudável").
func outboxFrom(fams map[string][]sample) *OutboxStats {
	pendingS, hasPending := fams["utilar_outbox_pending_events"]
	if !hasPending || len(pendingS) == 0 {
		return nil
	}

	o := &OutboxStats{Pending: int(pendingS[0].value)}
	if s := fams["utilar_outbox_oldest_unpublished_age_seconds"]; len(s) > 0 {
		o.OldestAgeSeconds = round1(s[0].value)
	}
	for _, s := range fams["utilar_outbox_published_total"] {
		if s.labels["outcome"] == "error" || s.labels["outcome"] == "failed" {
			o.Failed += int(s.value)
		}
	}
	if up := processUptimeSeconds(fams); up > 0 {
		var published float64
		for _, s := range fams["utilar_outbox_published_total"] {
			published += s.value
		}
		o.PublishedPerMinute = round1(published / (up / 60))
	}
	o.Severity = outboxSeverity(o.Pending, o.OldestAgeSeconds)
	return o
}

// OutboxSeverityForTest expõe a tabela de decisão de severidade da fila do
// outbox para o teste do pacote _test.
//
// PORQUÊ um export só para teste: os limiares são a regra que decide se o dono
// é acordado às 3h. Testá-los só através de um servidor HTTP falso exigiria
// montar 8 servidores para cobrir 8 linhas da tabela, e o que quebra num
// refactor é a comparação, não o transporte.
func OutboxSeverityForTest(pending int, oldestAgeSeconds float64) string {
	return outboxSeverity(pending, oldestAgeSeconds)
}
