package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/pkg/idempotency"
)

// Regressão: o SPA passou a enviar Idempotency-Key em POST /payments pra evitar
// cobrança duplicada no duplo clique, mas o CORS não declarava esse header.
//
// O efeito é traiçoeiro: o navegador manda um PREFLIGHT, o servidor responde
// 204 sem listar o header, e o navegador ABORTA a requisição real. O usuário vê
// "Failed to fetch" no Pix e no boleto — e o servidor NÃO REGISTRA NADA, porque
// a requisição nunca chegou. Ou seja: quebra o pagamento inteiro e não deixa
// rastro em log nenhum.
func TestCORS_PermiteIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{"*"}))
	r.POST("/api/v1/payments", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/payments", nil)
	req.Header.Set("Origin", "http://192.168.0.7:5175")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization,idempotency-key")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	permitidos := strings.ToLower(w.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(permitidos, strings.ToLower(idempotency.HeaderName)) {
		t.Fatalf("CORS não permite %s — o preflight barra e o pagamento nem sai do navegador.\n"+
			"Allow-Headers = %q", idempotency.HeaderName, permitidos)
	}
	// Os que já funcionavam não podem ter sumido no processo.
	for _, h := range []string{"content-type", "authorization"} {
		if !strings.Contains(permitidos, h) {
			t.Errorf("CORS deixou de permitir %q", h)
		}
	}
}
