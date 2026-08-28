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
	// Stripe: CPF/CNPJ do boleto não passa no dígito verificador (código
	// tax_id_invalid, mapeado no gateway pra stripe_tax_id_invalid). É o dado do
	// próprio cliente malformado — seguro e útil dizer exatamente o que é.
	case strings.Contains(msg, "tax_id_invalid"):
		return "CPF inválido — confira os números e tente de novo"
	// Stripe: cartão recusado/inválido. Deliberadamente NÃO revela o porquê
	// (saldo, roubo, CVC) — só diz que foi recusado e o que fazer. Revelar o
	// motivo ajuda o fraudador a calibrar a próxima tentativa (mesma razão do
	// genericPSPMessage).
	case strings.Contains(msg, "card_declined"), strings.Contains(msg, "expired_card"),
		strings.Contains(msg, "incorrect_cvc"), strings.Contains(msg, "incorrect_number"),
		strings.Contains(msg, "invalid_number"), strings.Contains(msg, "invalid_expiry"),
		strings.Contains(msg, "incorrect_zip"), strings.Contains(msg, "postal_code_invalid"):
		return "cartão recusado ou dados inválidos; confira os dados ou use outro cartão"
	case strings.Contains(msg, "amount_too_small"):
		return "valor abaixo do mínimo aceito para esta forma de pagamento"
	case strings.Contains(msg, "amount_too_large"):
		return "valor acima do máximo aceito para esta forma de pagamento"
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
