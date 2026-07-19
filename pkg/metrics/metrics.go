// Package metrics expõe métricas Prometheus para os serviços Go do Utilar.
//
// # Por que isto existe
//
// Até aqui, um `grep -rn 'prometheus|otel|/metrics'` em todo o Go do repo
// retornava UM comentário. Num sistema que move dinheiro, isso significa que
// "o outbox parou de publicar" ou "a taxa de recusa de cartão triplicou" só
// vira notícia quando o cliente reclama.
//
// # Cardinalidade (a armadilha clássica)
//
// Toda métrica aqui usa labels de cardinalidade FECHADA: rota (padrão do gin,
// nunca o path com ID), método HTTP, classe de status, provider, método de
// pagamento. NUNCA user_id, order_id, payment_id, email ou CPF. Uma série por
// pedido derruba o Prometheus e transforma /metrics num dump de PII.
//
// # /metrics não é público
//
// Ver Handler: o endpoint exige um bearer token (METRICS_TOKEN) e é comparado
// em tempo constante. Sem token configurado, o handler responde 404 — nunca
// abre por omissão. Detalhes e rationale em docs/observability-alerts.md.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry é o registro de métricas de um serviço. Registry próprio (em vez do
// default global) pra que os testes não vazem séries entre si e pra que o
// serviço controle exatamente o que é exposto.
type Registry struct {
	reg     *prometheus.Registry
	service string

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInFlight prometheus.Gauge
}

// New cria o registry com as métricas HTTP + os coletores de processo/Go.
func New(service string) *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	labels := prometheus.Labels{"service": service}
	m := &Registry{
		reg:     reg,
		service: service,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "utilar_http_requests_total",
			Help:        "Requests HTTP por rota, método e classe de status.",
			ConstLabels: labels,
		}, []string{"route", "method", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "utilar_http_request_duration_seconds",
			Help: "Latência de request HTTP por rota.",
			// Buckets calibrados pro perfil do Utilar: quase tudo abaixo de
			// 250ms; a cauda que importa é o checkout batendo no PSP (1-10s).
			Buckets:     []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			ConstLabels: labels,
		}, []string{"route", "method"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "utilar_http_in_flight_requests",
			Help:        "Requests HTTP em andamento.",
			ConstLabels: labels,
		}),
	}
	reg.MustRegister(m.httpRequests, m.httpDuration, m.httpInFlight)
	return m
}

// Registerer permite ao serviço registrar as próprias métricas de negócio
// (ver services/payment-service/internal/obs).
func (m *Registry) Registerer() prometheus.Registerer { return m.reg }

// Gatherer é usado pelos testes pra inspecionar o que seria exposto.
func (m *Registry) Gatherer() prometheus.Gatherer { return m.reg }

// Middleware instrumenta todo request do gin.
//
// SEGURANÇA (cardinalidade + PII): usa c.FullPath() — o PADRÃO da rota
// (`/api/v1/payments/:id`), nunca o path real com o UUID. Mesma decisão do
// AccessLog (audit M5). Request para rota inexistente vira "unmatched": sem
// isso, um scanner batendo em /aaa, /bbb, /ccc cria uma série por URL e derruba
// o Prometheus (cardinality bomb trivial de explorar).
func (m *Registry) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.httpInFlight.Inc()
		defer m.httpInFlight.Dec()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		m.httpDuration.WithLabelValues(route, c.Request.Method).Observe(time.Since(start).Seconds())
		m.httpRequests.WithLabelValues(route, c.Request.Method, statusClass(c.Writer.Status())).Inc()
	}
}

// statusClass reduz o status a "2xx".."5xx". Status exato multiplicaria as
// séries sem agregar informação útil pra alerta.
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return strconv.Itoa(code)
	}
}

// Handler devolve o handler de /metrics protegido por bearer token.
//
// FAIL-CLOSED: token vazio → 404 sempre. O modo "aberto pra rede interna" é uma
// mentira confortável — no ECS/K8s o "interno" inclui qualquer pod comprometido,
// e /metrics entrega volume financeiro, taxa de recusa e topologia do sistema.
//
// A comparação é em tempo constante (subtle.ConstantTimeCompare): comparação
// com == vaza o prefixo do token via timing e permite recuperá-lo byte a byte.
func (m *Registry) Handler(token string) gin.HandlerFunc {
	promHandler := promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		// Erro ao coletar não pode virar 500 barulhento no scrape nem expor
		// stack trace — logamos e devolvemos o que der.
		ErrorHandling: promhttp.ContinueOnError,
	})
	return func(c *gin.Context) {
		if token == "" {
			c.Status(http.StatusNotFound)
			return
		}
		if !authorized(c.GetHeader("Authorization"), token) {
			// 404 e não 401: não confirmamos sequer que o endpoint existe.
			c.Status(http.StatusNotFound)
			return
		}
		promHandler.ServeHTTP(c.Writer, c.Request)
	}
}

const bearerPrefix = "Bearer "

func authorized(header, token string) bool {
	if len(header) <= len(bearerPrefix) || header[:len(bearerPrefix)] != bearerPrefix {
		return false
	}
	got := header[len(bearerPrefix):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
