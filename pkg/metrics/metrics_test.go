package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/metrics"
)

func router(t *testing.T, token string) (*gin.Engine, *metrics.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := metrics.New("payment-service")
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/api/v1/payments/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/metrics", m.Handler(token))
	return r, m
}

func do(r *gin.Engine, method, path, auth string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestMetricsExigeToken(t *testing.T) {
	r, _ := router(t, "s3cr3t-token-de-scrape")

	if w := do(r, "GET", "/metrics", ""); w.Code != http.StatusNotFound {
		t.Errorf("sem Authorization deveria ser 404, veio %d", w.Code)
	}
	if w := do(r, "GET", "/metrics", "Bearer errado"); w.Code != http.StatusNotFound {
		t.Errorf("token errado deveria ser 404, veio %d", w.Code)
	}
	if w := do(r, "GET", "/metrics", "s3cr3t-token-de-scrape"); w.Code != http.StatusNotFound {
		t.Errorf("sem prefixo Bearer deveria ser 404, veio %d", w.Code)
	}
	w := do(r, "GET", "/metrics", "Bearer s3cr3t-token-de-scrape")
	if w.Code != http.StatusOK {
		t.Fatalf("token correto deveria abrir, veio %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "utilar_http_requests_total") {
		t.Error("payload não contém as métricas HTTP")
	}
}

// FAIL-CLOSED: esquecer de configurar METRICS_TOKEN não pode virar endpoint
// público de dados financeiros.
func TestMetricsSemTokenConfiguradoFicaFechado(t *testing.T) {
	r, _ := router(t, "")
	for _, auth := range []string{"", "Bearer ", "Bearer qualquer"} {
		if w := do(r, "GET", "/metrics", auth); w.Code != http.StatusNotFound {
			t.Fatalf("token vazio abriu o endpoint (auth=%q, status=%d)", auth, w.Code)
		}
	}
}

// REGRESSÃO cardinality bomb + PII: a série tem que usar o PADRÃO da rota, não
// o path com o UUID do pagamento.
func TestMetricsUsaPadraoDeRotaNaoOPathComID(t *testing.T) {
	r, _ := router(t, "tok")
	do(r, "GET", "/api/v1/payments/9f8c3a51-0000-4000-8000-000000000001", "")

	body := do(r, "GET", "/metrics", "Bearer tok").Body.String()
	if !strings.Contains(body, `route="/api/v1/payments/:id"`) {
		t.Errorf("esperava o padrão da rota nas labels:\n%s", body)
	}
	if strings.Contains(body, "9f8c3a51") {
		t.Fatalf("ID do pagamento vazou pra /metrics — PII + explosão de séries:\n%s", body)
	}
}

// Um scanner batendo em URLs aleatórias não pode criar uma série por URL.
func TestMetricsAgrupaRotasInexistentesEmUnmatched(t *testing.T) {
	r, _ := router(t, "tok")
	for _, p := range []string{"/aaa", "/bbb", "/ccc", "/.env", "/wp-admin"} {
		do(r, "GET", p, "")
	}
	body := do(r, "GET", "/metrics", "Bearer tok").Body.String()
	if !strings.Contains(body, `route="unmatched"`) {
		t.Error("rotas inexistentes deveriam colapsar em unmatched")
	}
	for _, p := range []string{"wp-admin", ".env"} {
		if strings.Contains(body, p) {
			t.Fatalf("path arbitrário virou label: %q", p)
		}
	}
}

func TestMetricsAgrupaStatusPorClasse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := metrics.New("svc")
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	r.GET("/metrics", m.Handler("tok"))

	do(r, "GET", "/boom", "")
	body := do(r, "GET", "/metrics", "Bearer tok").Body.String()
	if !strings.Contains(body, `status="5xx"`) {
		t.Errorf("esperava status agrupado em 5xx:\n%s", body)
	}
}
