package circuitbreaker

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

// ============================================================================
// Métricas do disjuntor
// ----------------------------------------------------------------------------
// PORQUÊ o disjuntor precisa aparecer no painel: um disjuntor aberto é uma
// FALHA SILENCIOSA por definição — ele existe justamente para o sistema não
// travar, então o sintoma que o operador veria (lentidão) desaparece. Sem
// métrica, "o catálogo está fora há 40 minutos e o checkout está recusando
// tudo" chega pelo telefone do cliente, não pelo alerta.
//
// CARDINALIDADE: o único label é `breaker`, que é o nome do serviço remoto —
// conjunto fechado, definido no código. Nunca order_id, user_id ou host.
// Mesma regra do pkg/metrics.
// ============================================================================

// Metrics são as séries do disjuntor, registradas no Registerer do serviço
// (ver pkg/metrics.Registry.Registerer).
type Metrics struct {
	state    *prometheus.GaugeVec
	failures *prometheus.CounterVec
	trips    *prometheus.CounterVec
	rejected *prometheus.CounterVec
}

// NewMetrics registra as séries. `service` vira ConstLabel, igual ao pkg/metrics.
//
// Usa MustRegister: métrica duplicada é erro de programação no boot, e falhar
// no boot é melhor que subir com observabilidade pela metade.
func NewMetrics(reg prometheus.Registerer, service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	m := &Metrics{
		state: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "utilar_circuit_breaker_state",
			Help:        "Estado do disjuntor: 0=fechado, 1=aberto, 2=meio-aberto.",
			ConstLabels: labels,
		}, []string{"breaker"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "utilar_circuit_breaker_failures_total",
			Help:        "Falhas contabilizadas pelo disjuntor (não inclui erro de negócio).",
			ConstLabels: labels,
		}, []string{"breaker"}),
		trips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "utilar_circuit_breaker_trips_total",
			Help:        "Quantas vezes o disjuntor ABRIU. É esta a série que vira alerta.",
			ConstLabels: labels,
		}, []string{"breaker"}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "utilar_circuit_breaker_rejected_total",
			Help: "Chamadas recusadas sem ir à rede (circuito aberto). " +
				"Mede o ESTRAGO da indisponibilidade: quantos pedidos não puderam ser criados.",
			ConstLabels: labels,
		}, []string{"breaker"}),
	}
	reg.MustRegister(m.state, m.failures, m.trips, m.rejected)
	return m
}

// Instrument liga uma Config às métricas e ao log.
//
// Devolve a Config alterada (valor, não ponteiro) para poder ser usada direto em
// New: `circuitbreaker.New("catalog", m.Instrument(cfg))`. Preserva callbacks
// que o chamador já tenha definido, encadeando em vez de sobrescrever — perder
// silenciosamente um OnStateChange do chamador seria uma armadilha.
func (m *Metrics) Instrument(cfg Config) Config {
	prevState, prevFail := cfg.OnStateChange, cfg.OnFailure

	cfg.OnStateChange = func(name string, from, to State) {
		m.state.WithLabelValues(name).Set(float64(to))
		if to == StateOpen {
			m.trips.WithLabelValues(name).Inc()
			// WARN e não ERROR: o disjuntor abrindo é o sistema se protegendo
			// como projetado. O ERROR é do serviço que está fora, e ele já
			// logou lá.
			slog.Warn("disjuntor ABRIU — chamadas ao serviço remoto serão recusadas",
				"breaker", name, "de", from.String(), "para", to.String())
		} else {
			slog.Info("disjuntor mudou de estado",
				"breaker", name, "de", from.String(), "para", to.String())
		}
		if prevState != nil {
			prevState(name, from, to)
		}
	}

	cfg.OnFailure = func(name string) {
		m.failures.WithLabelValues(name).Inc()
		if prevFail != nil {
			prevFail(name)
		}
	}
	return cfg
}

// Rejected contabiliza uma chamada recusada por circuito aberto.
//
// Fica a cargo do chamador (e não do próprio Breaker) porque só ele sabe se o
// ErrOpen resultou em pedido recusado ou em fallback silencioso — e a série só
// tem valor se significar sempre a mesma coisa.
func (m *Metrics) Rejected(breaker string) {
	m.rejected.WithLabelValues(breaker).Inc()
}
