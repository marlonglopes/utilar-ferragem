package handler

import (
	"strings"
)

// clientSafePSPMessage traduz um erro de gateway numa mensagem que pode ser
// entregue ao comprador (audit AV1-H5).
//
// PRINCÍPIO: o cliente só recebe informação que já é dele e que ele pode AGIR
// em cima. Tudo mais — corpo do PSP, status HTTP upstream, nome de campo
// interno, credencial, ID de merchant — fica no log, correlacionado pelo
// request_id que já vai no envelope de erro.
//
// ALLOWLIST, não denylist: casamos os erros de VALIDAÇÃO que nós mesmos
// geramos (gateway.CreatePayment valida antes de qualquer chamada de rede) e
// devolvemos texto NOSSO. Qualquer coisa não reconhecida cai no genérico. Uma
// denylist ("remova o que parecer sensível") falha no primeiro formato de erro
// novo que o PSP inventar — e a gente só descobre pelo vazamento.
func clientSafePSPMessage(err error) string {
	if err == nil {
		return genericPSPMessage
	}
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "payer_cpf"), strings.Contains(msg, "requires payer_cpf"):
		return "CPF é obrigatório para esta forma de pagamento"
	case strings.Contains(msg, "payer_name"):
		return "nome completo é obrigatório para esta forma de pagamento"
	case strings.Contains(msg, "cardtoken"), strings.Contains(msg, "tokenized card"):
		return "os dados do cartão não foram tokenizados corretamente; recarregue a página e tente de novo"
	case strings.Contains(msg, "unsupported method"):
		return "forma de pagamento não suportada"
	case strings.Contains(msg, "amount deve ser"), strings.Contains(msg, "amount must"):
		return "valor do pedido inválido"
	case strings.Contains(msg, "phone"):
		return "telefone é obrigatório para esta forma de pagamento"
	default:
		return genericPSPMessage
	}
}

// genericPSPMessage é deliberadamente vago e acionável: diz o que fazer sem
// revelar o motivo. Motivo de recusa é informação que ajuda o fraudador a
// calibrar a próxima tentativa.
const genericPSPMessage = "não foi possível processar o pagamento; confira os dados e tente novamente"
