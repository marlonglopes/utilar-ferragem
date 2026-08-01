package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/utilar/payment-service/internal/psp"
)

// PaymentConfigHandler expõe a CONFIGURAÇÃO de pagamento em LEITURA para o
// painel: qual PSP está ativo, quais métodos, e a saúde da credencial.
//
// ⚠️ NUNCA devolve segredo. Trocar de PSP ou a credencial continua sendo
// variável de ambiente (PSP_PROVIDER + *_SECRET) — segredo não se edita por
// tela. Nem o `Motivo` cru do check sai daqui: ele pode citar a mensagem do
// provider ("invalid API key ...") e virar pista de credencial. Só um
// booleano de saúde + status genérico.
type PaymentConfigHandler struct {
	gateway psp.Gateway
	check   *PSPCheck
}

func NewPaymentConfigHandler(gateway psp.Gateway, check *PSPCheck) *PaymentConfigHandler {
	return &PaymentConfigHandler{gateway: gateway, check: check}
}

// Get GET /api/v1/admin/payment/config — só admin (grupo de rota).
func (h *PaymentConfigHandler) Get(c *gin.Context) {
	healthy := true
	if h.check != nil {
		healthy = h.check.Estado().OK
	}
	provider := ""
	if h.gateway != nil {
		provider = h.gateway.Name()
	}
	status := "ok"
	if !healthy {
		status = "degraded"
	}
	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		// Métodos que a loja aceita no checkout (o app suporta os três em todos
		// os PSPs). A configuração fina por método é do provider, não daqui.
		"methods": []string{"pix", "credit_card", "boleto"},
		"healthy": healthy,
		"status":  status,
	})
}
