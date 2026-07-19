package alice

import (
	"strings"

	"github.com/utilar/assistant-service/internal/catalog"
)

// Mode é o contexto em que a Alice está atendendo. É a MESMA Alice — muda o
// tom, o que ela vê e o objetivo.
//
// SEGURANÇA: o modo é DERIVADO DA AUTENTICAÇÃO, nunca de um campo do corpo da
// requisição. Se o cliente pudesse pedir `{"mode":"vendedor"}`, qualquer
// visitante do site leria o custo e a margem da loja — que é o dado mais
// sensível do negócio. Por isso ModeFromClaims é a única fábrica de ModeVendedor
// e ela exige papel de operador num token com assinatura válida.
type Mode string

const (
	// ModeCliente — site público, visitante anônimo. Didática, explica o porquê.
	// NUNCA vê custo nem margem.
	ModeCliente Mode = "cliente"
	// ModeVendedor — balcão, operador de loja autenticado. Direta e técnica.
	// Vê custo, margem e em qual loja está o estoque.
	ModeVendedor Mode = "vendedor"
)

// papéis que habilitam o modo vendedor. Mesma nomenclatura do auth-service.
// `seller` NÃO entra: é o vendedor do marketplace (terceiro), não o operador da
// loja — ele não tem por que ver o custo da UtiLar.
var papeisDeBalcao = map[string]bool{
	"store_operator": true,
	"admin":          true,
}

// ModeFromClaims decide o modo a partir das claims JÁ VALIDADAS do JWT.
// Recebe o papel e se a assinatura foi conferida; qualquer dúvida vira cliente.
//
// Falha fechada de propósito: token ausente, expirado, com assinatura inválida
// ou com papel desconhecido cai em ModeCliente. O pior caso é um operador
// legítimo receber a experiência pública — irritante, mas sem vazamento.
func ModeFromClaims(autenticado bool, papel string) Mode {
	if !autenticado {
		return ModeCliente
	}
	if papeisDeBalcao[strings.TrimSpace(strings.ToLower(papel))] {
		return ModeVendedor
	}
	return ModeCliente
}

// VeCusto informa se o modo pode ver custo e margem.
func (m Mode) VeCusto() bool { return m == ModeVendedor }

// ---------------------------------------------------------------------------
// Redação de custo
// ---------------------------------------------------------------------------

// RedigirCusto apaga custo e margem de uma lista de produtos.
//
// Isto é a ÚLTIMA linha de defesa, aplicada na saída, e é redundante de
// propósito: o cliente do catálogo já evita pedir custo no modo cliente. Mas
// "já evita" é uma promessa sobre o caminho feliz, e vazamento de custo é
// irreversível — quando o número chega ao navegador, chegou. Uma varredura
// determinística na borda não depende de nenhum caminho ter se comportado.
func RedigirCusto(prods []catalog.Product) []catalog.Product {
	out := make([]catalog.Product, len(prods))
	copy(out, prods)
	for i := range out {
		out[i].Cost = nil
		out[i].Margem = nil
		out[i].Estoques = nil // localização por loja também é interna
	}
	return out
}

// personaDoModo devolve o trecho do system prompt específico do modo.
// O restante do prompt é comum às duas experiências.
func personaDoModo(m Mode) string {
	if m == ModeVendedor {
		return `MODO: BALCÃO (operador de loja autenticado).

Você está falando com um COLEGA que está atendendo um cliente no balcão, com fila atrás.
- Seja DIRETA e TÉCNICA. Vá ao ponto, sem introdução e sem repetir a pergunta. Economize palavras.
- Você VÊ o custo e a margem. Use isso para proteger o resultado da loja.
- Você VÊ em qual loja ou CD está o estoque. Diga onde está quando for relevante.
- Objetivo: FECHAR A VENDA PROTEGENDO A MARGEM.
- Se o cliente pedir desconto, NÃO sugira simplesmente ceder margem: ofereça o produto
  equivalente que resolve a mesma necessidade e preserva o resultado. Diga a margem de cada opção.
- Custo e margem são informação INTERNA. Ao sugerir a fala para o cliente final, nunca inclua o custo.`
	}
	return `MODO: CLIENTE (site público, visitante).

Você está falando com alguém que provavelmente não é da área e está decidindo o que comprar.
- Seja DIDÁTICA: explique o PORQUÊ, não só o quê. "Precisa de AC-III porque porcelanato tem baixa absorção."
- Tom acolhedor e claro. Evite jargão; quando usar um termo técnico, explique em seguida.
- Objetivo: ajudar a pessoa a DECIDIR BEM e não errar na compra.
- Sobre estoque, informe apenas a DISPONIBILIDADE (tem ou não tem, e a previsão).
- Você NÃO tem acesso a custo nem a margem, e não deve especular sobre eles em hipótese alguma.
  Se perguntarem quanto a loja paga ou quanto ela ganha, diga com naturalidade que essa é uma
  informação interna e volte a ajudar com a escolha.`
}
