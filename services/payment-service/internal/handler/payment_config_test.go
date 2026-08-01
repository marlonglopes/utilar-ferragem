package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// A config de pagamento é READ-ONLY e NÃO pode vazar segredo — nem o `Motivo`
// cru do check de saúde, que pode citar a mensagem do provider (e uma chave).
// Este teste força o estado "degraded" com um motivo que contém uma pista de
// credencial e prova que ela não aparece na resposta.
func TestPaymentConfig_NaoVazaSegredoNemMotivoCru(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Credencial "expirada": o Motivo cru vai conter a string sensível.
	segredo := "sk_live_SEGREDO_NAO_PODE_VAZAR"
	check := NewPSPCheck(gwFake{nome: "appmax-v1", err: errors.New("401 api_key_expired " + segredo)}, 0)
	check.Verify(context.Background()) // popula o estado (degraded)

	h := NewPaymentConfigHandler(gwFake{nome: "appmax-v1"}, check)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/config", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()

	// Expõe o provider e o estado, mas SEM o segredo/motivo cru.
	if !strings.Contains(body, "appmax-v1") {
		t.Errorf("deveria expor o provider: %s", body)
	}
	if !strings.Contains(body, "degraded") {
		t.Errorf("credencial ruim → status degraded: %s", body)
	}
	if strings.Contains(body, segredo) || strings.Contains(strings.ToLower(body), "api_key") {
		t.Fatalf("VAZOU credencial/motivo cru na config: %s", body)
	}
	// Métodos aparecem (o checkout aceita os três).
	for _, m := range []string{"pix", "credit_card", "boleto"} {
		if !strings.Contains(body, m) {
			t.Errorf("método %q ausente: %s", m, body)
		}
	}
}
