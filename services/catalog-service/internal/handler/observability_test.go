package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// ============================================================================
// Observabilidade agregada — /api/v1/admin/observability
// ----------------------------------------------------------------------------
// O que está em jogo: esta rota expõe a TOPOLOGIA INTERNA do sistema (quais
// serviços existem, quais estão de pé, qual está falhando) e o tamanho da fila
// que liga pagamento a pedido. É mapa para quem quer atacar e é o painel de
// incidente para o dono. Só admin.
//
// Além da autorização, o que precisa ser travado por teste é a MATEMÁTICA dos
// limiares: é ela que decide se o dono é acordado às 3h ou se o incidente passa
// despercebido. Um limiar errado aqui não dá erro em lugar nenhum — só silêncio.
// ============================================================================

// obsRouter monta a rota como no main.go: mesmo RequireAdmin, mesmo grupo.
// devMode=false — o que se testa aqui é a barreira criptográfica, e com devMode
// a rota aceitaria o header X-User-Role.
func obsRouter(targets []handler.ServiceTarget, metricsToken string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewObservabilityHandler(targets, metricsToken)
	g := r.Group("/api/v1/admin", handler.RequireAdmin(segredoUsuario, false))
	g.GET("/observability", h.Snapshot)
	return r
}

func obsGet(r *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestObservabilidade_SoAdminPassa — matriz completa de papéis.
//
// `store_operator` merece atenção especial: ele TEM acesso legítimo à rota
// /api/v1/store deste mesmo serviço (custo do carrinho dele). Este teste
// garante que essa permissão não transborda para o painel de infraestrutura.
func TestObservabilidade_SoAdminPassa(t *testing.T) {
	r := obsRouter(nil, "")

	casos := []struct {
		nome   string
		token  string
		status int
		code   string
	}{
		{"anonimo", "", http.StatusUnauthorized, "unauthorized"},
		{"customer", tokenComPapel(t, "customer"), http.StatusForbidden, "forbidden"},
		{"seller", tokenComPapel(t, "seller"), http.StatusForbidden, "forbidden"},
		{"store_operator", tokenComPapel(t, "store_operator"), http.StatusForbidden, "forbidden"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := obsGet(r, tc.token)
			if w.Code != tc.status {
				t.Fatalf("papel %q: status %d, esperado %d — corpo: %s",
					tc.nome, w.Code, tc.status, w.Body.String())
			}
			var env struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("resposta não usa o envelope de erro do projeto: %v", err)
			}
			if env.Code != tc.code {
				t.Errorf("code = %q, esperado %q", env.Code, tc.code)
			}
		})
	}

	// E o admin passa.
	if w := obsGet(r, tokenComPapel(t, "admin")); w.Code != http.StatusOK {
		t.Fatalf("admin recebeu %d, esperado 200 — corpo: %s", w.Code, w.Body.String())
	}
}

// TestObservabilidade_NaoCacheia — topologia interna e taxa de erro não podem
// ficar guardadas num proxy no caminho.
func TestObservabilidade_NaoCacheia(t *testing.T) {
	r := obsRouter(nil, "")
	w := obsGet(r, tokenComPapel(t, "admin"))
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, esperado no-store", got)
	}
}

// TestObservabilidade_SemAlvoNaoQuebra — instalação nova, nada configurado.
//
// Mesmo motivo do painel com zero pedidos: a primeira vez que o dono abre a
// tela é justamente quando nada foi configurado ainda. `null` em `services` ou
// `alerts` faz o `.map()` do front estourar e a tela inteira morre.
func TestObservabilidade_SemAlvoNaoQuebra(t *testing.T) {
	r := obsRouter(nil, "")
	w := obsGet(r, tokenComPapel(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	// Checagem no JSON CRU: decodificar em struct transformaria `null` em
	// slice vazio e o teste passaria com o bug presente.
	var cru map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &cru); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	for _, campo := range []string{"services", "alerts"} {
		if string(cru[campo]) == "null" {
			t.Errorf("%s veio `null` — o front faz .map() nisso e a tela quebra; use []", campo)
		}
	}
	if _, ok := cru["outbox"]; !ok {
		t.Error("outbox ausente — o front lê outbox.severity sem checar existência")
	}
	if _, ok := cru["collectedAt"]; !ok {
		t.Error("collectedAt ausente")
	}
}

// TestObservabilidade_ServicoForaDoArViraCritico.
//
// Serviço que não responde ao /health é `up:false`, severidade `critical` e um
// alerta. É o caso mais básico do painel e o mais fácil de quebrar num
// refactor: se `up:false` virasse `ok` por omissão, a tela mostraria tudo verde
// durante uma queda total.
func TestObservabilidade_ServicoForaDoArViraCritico(t *testing.T) {
	// Porta fechada de propósito — nada escutando ali.
	r := obsRouter([]handler.ServiceTarget{
		{Name: "order", BaseURL: "http://127.0.0.1:1"},
	}, "token-qualquer")

	w := obsGet(r, tokenComPapel(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got handler.ObservabilitySnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("esperado 1 serviço, veio %d", len(got.Services))
	}
	s := got.Services[0]
	if s.Up {
		t.Error("serviço com a porta fechada foi reportado como up — o painel estaria mentindo durante uma queda")
	}
	if s.Status != "critical" {
		t.Errorf("status = %q, esperado critical para serviço fora do ar", s.Status)
	}
	// LatencySeries nunca pode ser nil, nem para serviço morto.
	if s.LatencySeries == nil {
		t.Error("latencySeries veio nil — a sparkline do front estoura em null")
	}

	var achou bool
	for _, a := range got.Alerts {
		if a.ID == "service-order" && a.Severity == "critical" {
			achou = true
		}
	}
	if !achou {
		t.Errorf("serviço fora do ar não gerou alerta crítico; alertas: %+v", got.Alerts)
	}
}

// TestObservabilidade_LeMetricasEOutbox — o caminho feliz completo.
//
// Um serviço falso serve /health e /metrics no formato Prometheus real, com os
// nomes de série que o pkg/metrics e o payment-service publicam de verdade. É
// o teste que prova que o parser entende o que os serviços de fato emitem —
// um parser testado só contra um exemplo inventado por mim provaria nada.
func TestObservabilidade_LeMetricasEOutbox(t *testing.T) {
	const metricsToken = "token-de-metricas"

	// Corpo de /metrics montado à mão, com números escolhidos para que a
	// conferência seja possível de cabeça:
	//
	//   Latência: 100 requests no total (+Inf), distribuídos assim —
	//     ≤ 0,005s:  50   ≤ 0,01s:  80   ≤ 0,025s: 95   ≤ 0,05s: 99   +Inf: 100
	//   p50 = 50º request → cai exatamente no limite de 0,005s → 5 ms
	//
	//   Requests: 950 de 2xx + 50 de 5xx = 1000 → errorRate = 50/1000 = 0,05
	//
	//   Outbox: 7 pendentes, o mais antigo parado há 240s.
	//     240s ≥ 120s (limiar de warn) e < 900s (crítico) → severity = "warn"
	corpoMetrics := strings.Join([]string{
		`# HELP utilar_http_request_duration_seconds Latência`,
		`# TYPE utilar_http_request_duration_seconds histogram`,
		`utilar_http_request_duration_seconds_bucket{service="order-service",route="/api/v1/orders",method="GET",le="0.005"} 50`,
		`utilar_http_request_duration_seconds_bucket{service="order-service",route="/api/v1/orders",method="GET",le="0.01"} 80`,
		`utilar_http_request_duration_seconds_bucket{service="order-service",route="/api/v1/orders",method="GET",le="0.025"} 95`,
		`utilar_http_request_duration_seconds_bucket{service="order-service",route="/api/v1/orders",method="GET",le="0.05"} 99`,
		`utilar_http_request_duration_seconds_bucket{service="order-service",route="/api/v1/orders",method="GET",le="+Inf"} 100`,
		`# TYPE utilar_http_requests_total counter`,
		`utilar_http_requests_total{service="order-service",route="/api/v1/orders",method="GET",status="2xx"} 950`,
		`utilar_http_requests_total{service="order-service",route="/api/v1/orders",method="GET",status="5xx"} 50`,
		`# TYPE utilar_outbox_pending_events gauge`,
		`utilar_outbox_pending_events{service="payment-service"} 7`,
		`# TYPE utilar_outbox_oldest_unpublished_age_seconds gauge`,
		`utilar_outbox_oldest_unpublished_age_seconds{service="payment-service"} 240`,
		`# TYPE utilar_outbox_published_total counter`,
		`utilar_outbox_published_total{service="payment-service",event_type="payment.confirmed",outcome="ok"} 1200`,
		`utilar_outbox_published_total{service="payment-service",event_type="payment.confirmed",outcome="error"} 3`,
		`# TYPE process_start_time_seconds gauge`,
		`process_start_time_seconds 1000000000`,
		"",
	}, "\n")

	var pediuMetricsSemToken bool
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
		case "/metrics":
			// O agregador PRECISA mandar o bearer: /metrics é fail-closed e
			// sem o header o outro lado responde 404. Se este teste vir uma
			// requisição sem Authorization, o agregador está quebrado.
			if req.Header.Get("Authorization") != "Bearer "+metricsToken {
				pediuMetricsSemToken = true
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprint(w, corpoMetrics)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	r := obsRouter([]handler.ServiceTarget{
		{Name: "payment", BaseURL: fake.URL},
	}, metricsToken)

	w := obsGet(r, tokenComPapel(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if pediuMetricsSemToken {
		t.Fatal("o agregador chamou /metrics SEM o bearer — em produção isso vira 404 e o painel fica cego")
	}

	var got handler.ObservabilitySnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("esperado 1 serviço, veio %d", len(got.Services))
	}
	s := got.Services[0]

	if !s.Up {
		t.Error("serviço respondendo /health foi reportado como fora do ar")
	}
	// p50: o 50º de 100 requests cai exatamente no bucket de 0,005s = 5 ms.
	if s.P50Ms < 4.9 || s.P50Ms > 5.1 {
		t.Errorf("p50Ms = %v, esperado ~5 (o 50º de 100 requests cai no bucket de 0,005s)", s.P50Ms)
	}
	// 50 de 5xx em 1000 requests = 5%, que é o limiar de crítico.
	if s.ErrorRate < 0.0499 || s.ErrorRate > 0.0501 {
		t.Errorf("errorRate = %v, esperado 0.05 (50 de 5xx em 1000)", s.ErrorRate)
	}
	if s.Status != "critical" {
		t.Errorf("status = %q, esperado critical: 5%% de 5xx bate o limiar", s.Status)
	}
	if len(s.LatencySeries) != 1 {
		t.Errorf("latencySeries com %d pontos, esperado 1 após a primeira coleta", len(s.LatencySeries))
	}

	// --- Outbox: o número mais importante da rota ---
	if got.Outbox.Pending != 7 {
		t.Errorf("outbox.pending = %d, esperado 7", got.Outbox.Pending)
	}
	if got.Outbox.OldestAgeSeconds != 240 {
		t.Errorf("outbox.oldestAgeSeconds = %v, esperado 240", got.Outbox.OldestAgeSeconds)
	}
	if got.Outbox.Failed != 3 {
		t.Errorf("outbox.failed = %d, esperado 3 (outcome=error)", got.Outbox.Failed)
	}
	// 240s ≥ 120 (warn) e < 900 (crítico), com só 7 pendentes.
	// É a decisão de projeto: IDADE pesa mais que TAMANHO.
	if got.Outbox.Severity != "warn" {
		t.Errorf("outbox.severity = %q, esperado warn (240s passa do limiar de 120s, mas não do de 900s)",
			got.Outbox.Severity)
	}

	var achouAlertaOutbox bool
	for _, a := range got.Alerts {
		if a.ID == "outbox-backlog" {
			achouAlertaOutbox = true
			if a.Severity != "warn" {
				t.Errorf("alerta de outbox com severidade %q, esperado warn", a.Severity)
			}
			// O alerta tem que linkar para onde o sintoma aparece: a fila
			// parada é a CAUSA dos pedidos travados da visão geral.
			if a.Href == "" {
				t.Error("alerta de outbox sem href — o front transforma o alerta em link")
			}
		}
	}
	if !achouAlertaOutbox {
		t.Errorf("fila do outbox em warn não gerou alerta; alertas: %+v", got.Alerts)
	}
}

// TestObservabilidade_LimiaresDoOutbox — a tabela de decisão que o front
// espelha (`outboxSeverity` em app/src/lib/adminAdapters.ts).
//
// PORQUÊ testar a tabela inteira e não só um ponto: cada linha aqui é uma
// decisão de acordar (ou não acordar) o dono às 3h da manhã. O ponto central é
// que 500 eventos escoando rápido é SAUDÁVEL e 3 eventos parados há uma hora é
// INCIDENTE — o oposto do que o instinto sugere ao olhar só o tamanho da fila.
func TestObservabilidade_LimiaresDoOutbox(t *testing.T) {
	casos := []struct {
		nome      string
		pendentes int
		idadeSeg  float64
		esperado  string
	}{
		{"fila vazia", 0, 0, "ok"},
		{"fila grande escoando rápido", 480, 5, "warn"},    // tamanho passa de 100
		{"poucos eventos parados há 5min", 3, 300, "warn"}, // idade passa de 120s
		{"poucos eventos parados há 1h", 3, 3600, "critical"},
		{"limiar exato de warn por idade", 1, 120, "warn"},
		{"logo abaixo do limiar de warn", 1, 119, "ok"},
		{"limiar exato de critico por idade", 1, 900, "critical"},
		{"limiar exato de critico por tamanho", 500, 0, "critical"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			got := handler.OutboxSeverityForTest(tc.pendentes, tc.idadeSeg)
			if got != tc.esperado {
				t.Errorf("severidade(%d pendentes, %vs) = %q, esperado %q",
					tc.pendentes, tc.idadeSeg, got, tc.esperado)
			}
		})
	}
}
