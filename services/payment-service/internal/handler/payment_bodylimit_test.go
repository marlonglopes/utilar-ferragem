// H5: POST /payments com body > maxPaymentRequestBody é rejeitado com 400
// antes do bind tocar DB ou order-service.
package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/handler"
)

// Cap declarado em payment.go: maxPaymentRequestBody = 16 * 1024.
// O teste cria body de 24KB pra estourar com folga.
const oversizedBodyBytes = 24 * 1024

// setupBodyCapRouter cria um PaymentHandler com DB nil + stubs nil.
// O handler aborta no bind antes de tocar qualquer dependência, então
// é seguro passar nil em DevMode.
func setupBodyCapRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	r.Use(func(c *gin.Context) {
		c.Set("user_id", testUserID)
		c.Set("user_email", "test@utilar.dev")
		c.Next()
	})
	pH := handler.NewPaymentHandler(nil, nil, nil, true)
	r.POST("/api/v1/payments", pH.Create)
	return r
}

func TestCreate_BodyOverLimit_Returns400(t *testing.T) {
	r := setupBodyCapRouter(t)

	// JSON válido mas com filler enorme num campo desconhecido — passa pela
	// validação sintática mas estoura o cap.
	filler := strings.Repeat("A", oversizedBodyBytes)
	body := []byte(`{"order_id":"x","method":"pix","amount":1,"_filler":"` + filler + `"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Sem MaxBytesReader: ShouldBindJSON aceitaria; ele iria pra próxima etapa
	// e provavelmente falharia em DB (panic com nil) ou em validação.
	// Com MaxBytesReader: bind retorna erro → 400.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestCreate_BodyUnderLimit_BindOK(t *testing.T) {
	r := setupBodyCapRouter(t)

	// Body pequeno e bem-formado. Esperamos passar do bind. Como DB é nil,
	// o handler vai eventualmente panic — nesse caso esperamos 500. O
	// importante é que NÃO seja 400 por causa do cap.
	body := []byte(`{"order_id":"11111111-1111-1111-1111-111111111111","method":"pix","amount":100}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer fake")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	defer func() {
		// Se o panic do nil DB chegar aqui, recupera. O essencial é que o
		// fluxo passou do MaxBytesReader.
		_ = recover()
	}()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Fatalf("body pequeno foi rejeitado pelo cap (body=%s)", w.Body.String())
	}
}
