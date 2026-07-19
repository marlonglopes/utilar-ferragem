package llm

import (
	"context"
	"encoding/json"
	"strings"
)

// Mock é a Alice sem chave de API: guiada por regras, mas ainda usando tool use
// (busca real no catálogo) pra não inventar. Demonstra o fluxo ponta a ponta.
// Mesmo espírito do modo mock do resto do Utilar.
type Mock struct{}

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Complete(_ context.Context, _ string, _ []Tool, msgs []Message) (*Response, error) {
	last := msgs[len(msgs)-1]

	// Se a última mensagem traz resultado de tool, fecha com uma resposta amistosa.
	for _, b := range last.Blocks {
		if b.Type == "tool_result" {
			txt := "Achei estas opções no nosso catálogo — dá uma olhada nos cards aqui embaixo. " +
				"Quer que eu detalhe alguma, veja o estoque ou monte a lista pra uma obra?"
			if b.IsError || strings.Contains(strings.ToLower(b.Text), "nenhum") {
				txt = "Não encontrei nada com esse termo agora. Pode tentar de outro jeito? " +
					"Ex.: uma categoria (ferramentas, construção, elétrica) ou a marca."
			}
			return &Response{Blocks: []Block{Text(txt)}, StopReason: "end_turn"}, nil
		}
	}

	// Última mensagem é texto do usuário: decide se busca ou saúda.
	userText := ""
	for _, b := range last.Blocks {
		if b.Type == "text" {
			userText += " " + b.Text
		}
	}
	q := strings.ToLower(userText)

	greeting := contains(q, "oi", "olá", "ola", "bom dia", "boa tarde", "boa noite", "ajuda", "quem é você", "quem e voce")
	if greeting && !hasProductIntent(q) {
		return &Response{Blocks: []Block{Text(
			"Oi! Eu sou a Alice ✨, sua ajudante aqui da UtiLar Ferragem. " +
				"Posso achar ferramentas e materiais, comparar preços e estoque, e montar a lista pra sua obra. " +
				"O que você está procurando?",
		)}, StopReason: "end_turn"}, nil
	}

	// Intenção de produto → chama a tool de busca com o termo limpo.
	term := cleanQuery(userText)
	input, _ := json.Marshal(map[string]string{"query": term})
	return &Response{
		Blocks:     []Block{{Type: "tool_use", ToolUseID: "mock-1", ToolName: "search_products", ToolInput: input}},
		StopReason: "tool_use",
	}, nil
}

func hasProductIntent(q string) bool {
	return contains(q, "procur", "quero", "preciso", "tem ", "vende", "comprar", "furad", "cimento",
		"parafuso", "tinta", "cabo", "broca", "martelo", "serra", "disjuntor", "tubo", "torneira",
		"ferrament", "constru", "elétr", "eletr", "hidrául", "hidraul", "fixaç", "fixac")
}

func cleanQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, stop := range []string{"eu ", "quero ", "preciso de ", "preciso ", "procuro ", "procurando ",
		"vocês têm ", "voces tem ", "tem ", "por favor", "um ", "uma ", "uns ", "umas ", "?"} {
		s = strings.ReplaceAll(s, stop, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

func contains(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

var _ LLM = (*Mock)(nil)
