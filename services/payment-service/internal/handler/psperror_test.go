package handler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// REGRESSÃO AV1-H5: o corpo cru do PSP NUNCA pode chegar ao cliente.
//
// Os gateways embutem até 2KB da resposta do PSP na mensagem de erro, e
// `ErrInvalidRequest` cobre 400/401/403/409/422. O handler devolvia
// `pspErr.Error()` direto no body do 400 — vazando credencial de auth em 401 e
// PII do comprador em 422 de validação.
func TestMensagemDeErroDoPSPNaoVazaCorpoAoCliente(t *testing.T) {
	vazamentos := []struct {
		nome     string
		err      error
		proibido []string
	}{
		{
			nome: "401 de credencial nossa",
			err: fmt.Errorf("%w: appmax-v1 GET /v1/orders/1 → 401: {\"error\":\"credenciais inválidas\",\"client_id\":\"cid-de-producao\"}",
				psp.ErrInvalidRequest),
			proibido: []string{"cid-de-producao", "credenciais", "401"},
		},
		{
			nome: "422 ecoando PII do comprador",
			err: fmt.Errorf("%w: appmax-v1 POST /v1/customers → 422: {\"errors\":{\"document_number\":[\"12345678909 inválido\"],\"email\":[\"joao@exemplo.com.br\"]}}",
				psp.ErrInvalidRequest),
			proibido: []string{"12345678909", "joao@exemplo.com.br", "document_number"},
		},
		{
			nome: "erro com PAN ecoado",
			err: fmt.Errorf("%w: appmax-v1 POST /v1/payments/credit-card → 400: {\"card\":{\"number\":\"4111111111111111\"}}",
				psp.ErrInvalidRequest),
			proibido: []string{"4111111111111111", "/v1/payments"},
		},
		{
			nome:     "erro genérico do upstream",
			err:      fmt.Errorf("%w: api.appmax.com.br: connection refused", psp.ErrUpstream),
			proibido: []string{"api.appmax.com.br", "connection refused"},
		},
	}

	for _, tc := range vazamentos {
		t.Run(tc.nome, func(t *testing.T) {
			msg := clientSafePSPMessage(tc.err)
			for _, p := range tc.proibido {
				if strings.Contains(msg, p) {
					t.Errorf("VAZOU %q na mensagem entregue ao cliente: %q", p, msg)
				}
			}
			if msg == "" {
				t.Error("mensagem vazia — o cliente fica sem saber o que fazer")
			}
		})
	}
}

// A allowlist tem que continuar entregando o que é ÚTIL e é do próprio cliente:
// sem isso o checkout vira "erro genérico" e o suporte é quem paga a conta.
func TestErrosDeValidacaoContinuamAcionaveisParaOCliente(t *testing.T) {
	casos := map[string]string{
		"pix requires payer_cpf":                                   "CPF",
		"boleto requires payer_cpf and payer_name":                 "CPF",
		"card via appmax-v1 requires a tokenized card (CardToken)": "cartão",
		"unsupported method \"crypto\"":                            "forma de pagamento",
		"amount deve ser > 0":                                      "valor",
	}
	for erroInterno, esperado := range casos {
		msg := clientSafePSPMessage(fmt.Errorf("%w: %s", psp.ErrInvalidRequest, erroInterno))
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(esperado)) {
			t.Errorf("erro %q virou %q, esperava algo sobre %q", erroInterno, msg, esperado)
		}
		if msg == genericPSPMessage {
			t.Errorf("erro acionável %q caiu no genérico", erroInterno)
		}
	}
}

// Erro desconhecido cai no genérico — allowlist, não denylist. Um formato de
// erro novo que o PSP inventar não pode virar vazamento por omissão.
func TestErroDesconhecidoCaiNoGenerico(t *testing.T) {
	for _, err := range []error{
		nil,
		fmt.Errorf("%w: formato_novo_do_psp: {\"segredo\":\"xyz\"}", psp.ErrInvalidRequest),
	} {
		if got := clientSafePSPMessage(err); got != genericPSPMessage {
			t.Errorf("clientSafePSPMessage(%v) = %q, esperado o genérico", err, got)
		}
	}
}

// A mensagem genérica não pode revelar o MOTIVO da recusa — motivo é
// informação que ajuda o fraudador a calibrar a próxima tentativa.
func TestMensagemGenericaNaoRevelaMotivoDeRecusa(t *testing.T) {
	baixo := strings.ToLower(genericPSPMessage)
	for _, palavra := range []string{"saldo", "limite", "recusado pelo banco", "antifraude", "risco", "bloqueado"} {
		if strings.Contains(baixo, palavra) {
			t.Errorf("mensagem genérica revela motivo (%q): %q", palavra, genericPSPMessage)
		}
	}
}
