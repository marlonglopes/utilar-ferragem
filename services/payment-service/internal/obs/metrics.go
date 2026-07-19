// Package obs são as métricas de NEGÓCIO do payment-service.
//
// pkg/metrics cuida do genérico (latência HTTP, status, runtime Go). Aqui ficam
// as perguntas que só fazem sentido num sistema de pagamento e que são as que
// realmente acordam alguém às 3h:
//
//   - o outbox parou de publicar? (qual a idade do evento mais velho não publicado)
//   - a taxa de recusa de cartão subiu?
//   - o PSP está lento?
//   - a reconciliação achou divergência de dinheiro?
//
// CARDINALIDADE: nenhum label aqui carrega id de pedido, pagamento, usuário ou
// qualquer PII. Só provider, método, status e motivo — todos de domínio fechado.
package obs

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics agrupa os coletores de negócio.
type Metrics struct {
	paymentsCreated   *prometheus.CounterVec
	paymentsConfirmed *prometheus.CounterVec
	paymentsFailed    *prometheus.CounterVec
	pspDuration       *prometheus.HistogramVec
	pspErrors         *prometheus.CounterVec

	webhooksReceived *prometheus.CounterVec

	outboxPending   prometheus.Gauge
	outboxOldestAge prometheus.Gauge
	outboxPublished *prometheus.CounterVec

	reconChecked      *prometheus.CounterVec
	reconDiscrepancy  *prometheus.CounterVec
	reconRuns         *prometheus.CounterVec
	reconDuration     *prometheus.HistogramVec
	reconOpenDiscreps prometheus.Gauge

	ledgerPosted   *prometheus.CounterVec
	ledgerRejected *prometheus.CounterVec
}

// New registra os coletores. `reg` vem de metrics.Registry.Registerer().
func New(reg prometheus.Registerer) *Metrics {
	l := prometheus.Labels{"service": "payment-service"}
	m := &Metrics{
		paymentsCreated: counter(l, "utilar_payments_created_total",
			"Pagamentos criados.", "provider", "method"),
		paymentsConfirmed: counter(l, "utilar_payments_confirmed_total",
			"Pagamentos confirmados.", "provider", "method"),
		paymentsFailed: counter(l, "utilar_payments_failed_total",
			"Pagamentos que terminaram em falha/recusa.", "provider", "method", "reason"),
		pspDuration: histogram(l, "utilar_psp_request_duration_seconds",
			"Latência das chamadas ao PSP.",
			[]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
			"provider", "operation"),
		pspErrors: counter(l, "utilar_psp_errors_total",
			"Erros nas chamadas ao PSP, por classe normalizada.", "provider", "operation", "kind"),
		webhooksReceived: counter(l, "utilar_webhooks_received_total",
			"Webhooks recebidos, por desfecho do processamento.", "provider", "outcome"),

		outboxPending: gauge(l, "utilar_outbox_pending_events",
			"Eventos no outbox ainda não publicados."),
		outboxOldestAge: gauge(l, "utilar_outbox_oldest_unpublished_age_seconds",
			"Idade do evento não publicado mais antigo. É ESTA a métrica que detecta outbox parado: a fila pode estar pequena e mesmo assim travada."),
		outboxPublished: counter(l, "utilar_outbox_published_total",
			"Eventos publicados no broker.", "event_type", "outcome"),

		reconChecked: counter(l, "utilar_reconciliation_checked_total",
			"Pagamentos verificados na reconciliação.", "provider"),
		reconDiscrepancy: counter(l, "utilar_reconciliation_discrepancies_total",
			"Divergências encontradas, por tipo.", "provider", "kind"),
		reconRuns: counter(l, "utilar_reconciliation_runs_total",
			"Execuções de reconciliação, por desfecho.", "provider", "status"),
		reconDuration: histogram(l, "utilar_reconciliation_duration_seconds",
			"Duração da reconciliação.", []float64{1, 5, 15, 30, 60, 120, 300}, "provider"),
		reconOpenDiscreps: gauge(l, "utilar_reconciliation_open_discrepancies",
			"Divergências de dinheiro ainda NÃO resolvidas por um humano."),

		ledgerPosted: counter(l, "utilar_ledger_transactions_posted_total",
			"Lançamentos gravados no livro.", "kind"),
		ledgerRejected: counter(l, "utilar_ledger_transactions_rejected_total",
			"Lançamentos recusados, por motivo.", "kind", "reason"),
	}
	reg.MustRegister(
		m.paymentsCreated, m.paymentsConfirmed, m.paymentsFailed,
		m.pspDuration, m.pspErrors, m.webhooksReceived,
		m.outboxPending, m.outboxOldestAge, m.outboxPublished,
		m.reconChecked, m.reconDiscrepancy, m.reconRuns, m.reconDuration, m.reconOpenDiscreps,
		m.ledgerPosted, m.ledgerRejected,
	)
	return m
}

func counter(l prometheus.Labels, name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help, ConstLabels: l}, labels)
}

func histogram(l prometheus.Labels, name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets, ConstLabels: l}, labels)
}

func gauge(l prometheus.Labels, name, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help, ConstLabels: l})
}

// ===================== API de instrumentação =====================

func (m *Metrics) PaymentCreated(provider, method string) {
	m.paymentsCreated.WithLabelValues(provider, method).Inc()
}

func (m *Metrics) PaymentConfirmed(provider, method string) {
	m.paymentsConfirmed.WithLabelValues(provider, method).Inc()
}

// PaymentFailed. `reason` é de domínio FECHADO ("psp_error", "rejected",
// "expired", "cancelled", "validation") — nunca a mensagem de erro, que é
// texto livre vindo do PSP e explodiria a cardinalidade.
func (m *Metrics) PaymentFailed(provider, method, reason string) {
	m.paymentsFailed.WithLabelValues(provider, method, reason).Inc()
}

// ObservePSP mede uma chamada ao PSP. `kind` só é usado quando err != nil.
func (m *Metrics) ObservePSP(provider, operation string, d time.Duration, errKind string) {
	m.pspDuration.WithLabelValues(provider, operation).Observe(d.Seconds())
	if errKind != "" {
		m.pspErrors.WithLabelValues(provider, operation, errKind).Inc()
	}
}

// WebhookReceived. `outcome`: accepted | duplicate | rejected_signature |
// rejected_amount | psp_error | parse_error | not_found.
func (m *Metrics) WebhookReceived(provider, outcome string) {
	m.webhooksReceived.WithLabelValues(provider, outcome).Inc()
}

func (m *Metrics) OutboxPublished(eventType, outcome string) {
	m.outboxPublished.WithLabelValues(eventType, outcome).Inc()
}

func (m *Metrics) LedgerPosted(kind string) { m.ledgerPosted.WithLabelValues(kind).Inc() }
func (m *Metrics) LedgerRejected(kind, reason string) {
	m.ledgerRejected.WithLabelValues(kind, reason).Inc()
}

// ledger.Observer
func (m *Metrics) ReconciliationChecked(provider string) {
	m.reconChecked.WithLabelValues(provider).Inc()
}
func (m *Metrics) ReconciliationDiscrepancy(provider, kind string) {
	m.reconDiscrepancy.WithLabelValues(provider, kind).Inc()
}
func (m *Metrics) ReconciliationRun(provider, status string, d time.Duration) {
	m.reconRuns.WithLabelValues(provider, status).Inc()
	m.reconDuration.WithLabelValues(provider).Observe(d.Seconds())
}

// ===================== Coletores por polling =====================

// StartDBPolling atualiza periodicamente as métricas que só existem como
// consulta ao banco (profundidade do outbox, divergências abertas).
//
// POR QUE POLLING e não incrementar no ponto de escrita: o outbox pode estar
// travado justamente porque o drainer não está rodando — uma métrica
// alimentada pelo drainer ficaria PARADA no último valor e o alerta nunca
// dispararia. Um poller independente enxerga o problema de fora.
func (m *Metrics) StartDBPolling(ctx context.Context, db *sql.DB, every time.Duration) {
	if every <= 0 {
		every = 15 * time.Second
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			m.pollOnce(ctx, db)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (m *Metrics) pollOnce(ctx context.Context, db *sql.DB) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var pending int64
	var oldest sql.NullFloat64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), EXTRACT(EPOCH FROM (now() - MIN(created_at)))
		FROM payments_outbox WHERE published_at IS NULL`).Scan(&pending, &oldest); err != nil {
		slog.Warn("obs: poll do outbox falhou", "error", err)
	} else {
		m.outboxPending.Set(float64(pending))
		m.outboxOldestAge.Set(oldest.Float64) // 0 quando a fila está vazia
	}

	var open int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reconciliation_discrepancies WHERE resolved_at IS NULL`).Scan(&open); err != nil {
		slog.Warn("obs: poll de divergências falhou", "error", err)
	} else {
		m.reconOpenDiscreps.Set(float64(open))
	}
}
