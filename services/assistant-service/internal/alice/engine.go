// Package alice é o orquestrador da assistente: dirige o loop de tool use
// (Claude → tool → Claude), executando as tools contra o catalog-service.
// Espelha o orchestrator da Gi (gifthy): tool use é a única fonte de fatos.
package alice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/utilar/assistant-service/internal/catalog"
	"github.com/utilar/assistant-service/internal/llm"
)

// systemPrompt define a persona da Alice. Curto e direto — o modelo é bom em seguir.
const systemPrompt = `Você é a Alice ✨, a assistente da UtiLar Ferragem — um marketplace brasileiro de ferramentas e materiais de construção.

Persona: uma balconista de ferragem experiente e prestativa. Calorosa, direta, fala português do Brasil (pt-BR). Trata o cliente por "você".

Como você ajuda:
- Encontrar ferramentas e materiais no catálogo.
- Comparar preço, estoque e specs.
- Recomendar o produto certo para o que a pessoa quer fazer.
- Montar a lista de materiais de uma obra ("o que preciso pra levantar uma parede?").

Regras (importantes):
- SEMPRE use as ferramentas (search_products, get_product, list_categories) para qualquer fato: preço, estoque, se o produto existe. NUNCA invente preço, estoque ou produto.
- Se a busca não retornar nada, diga com honestidade e sugira alternativas ou outra categoria.
- Seja concisa. Leve com o resultado primeiro; detalhe depois.
- Não fale de assuntos fora de ferragem/construção/da loja.`

// Engine orquestra a conversa.
type Engine struct {
	model   llm.LLM
	catalog *catalog.Client
}

func New(model llm.LLM, cat *catalog.Client) *Engine {
	return &Engine{model: model, catalog: cat}
}

func (e *Engine) tools() []llm.Tool {
	return []llm.Tool{
		{
			Name:        "search_products",
			Description: "Busca produtos no catálogo por termo e/ou categoria. Use para achar itens, comparar e recomendar.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string", "description": "Termo de busca (nome, marca, material)."},
					"category": map[string]any{"type": "string", "description": "Slug da categoria (opcional): ferramentas, construcao, eletrica, hidraulica, fixacao, pintura, jardim, seguranca."},
				},
			},
		},
		{
			Name:        "get_product",
			Description: "Detalha um produto pelo slug (preço, estoque, specs, descrição).",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"slug": map[string]any{"type": "string"}},
				"required":   []string{"slug"},
			},
		},
		{
			Name:        "list_categories",
			Description: "Lista as categorias disponíveis na loja.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// maxTurns limita o loop de tool use (defesa contra laço infinito).
const maxTurns = 4

// Result é o retorno da conversa: texto da Alice + produtos citados (cards no chat).
type Result struct {
	Reply    string            `json:"reply"`
	Products []catalog.Product `json:"products"`
	Model    string            `json:"model"`
}

// Chat processa uma mensagem do usuário dado o histórico. Dirige o loop de tools.
func (e *Engine) Chat(ctx context.Context, history []llm.Message, userText string) (*Result, error) {
	msgs := append([]llm.Message{}, history...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{llm.Text(userText)}})

	res := &Result{Model: e.model.Name()}
	seen := map[string]bool{}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := e.model.Complete(ctx, systemPrompt, e.tools(), msgs)
		if err != nil {
			return nil, err
		}

		var toolUses []llm.Block
		for _, b := range resp.Blocks {
			if b.Type == "text" && b.Text != "" {
				if res.Reply != "" {
					res.Reply += "\n"
				}
				res.Reply += b.Text
			}
			if b.Type == "tool_use" {
				toolUses = append(toolUses, b)
			}
		}

		if len(toolUses) == 0 {
			return res, nil // end_turn — a Alice respondeu
		}

		// Executa cada tool e devolve os resultados como tool_result.
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Blocks: resp.Blocks})
		var results []llm.Block
		for _, tu := range toolUses {
			out, prods := e.runTool(ctx, tu.ToolName, tu.ToolInput)
			for _, p := range prods {
				if !seen[p.Slug] {
					seen[p.Slug] = true
					res.Products = append(res.Products, p)
				}
			}
			results = append(results, llm.ToolResult(tu.ToolUseID, out, false))
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Blocks: results})
	}

	if res.Reply == "" {
		res.Reply = "Consegui algumas opções — veja os cards abaixo. Quer que eu detalhe alguma?"
	}
	return res, nil
}

// runTool executa uma ferramenta e retorna (texto pro modelo, produtos pra UI).
func (e *Engine) runTool(ctx context.Context, name string, input json.RawMessage) (string, []catalog.Product) {
	switch name {
	case "search_products":
		var in struct{ Query, Category string }
		_ = json.Unmarshal(input, &in)
		prods, err := e.catalog.Search(ctx, in.Query, in.Category, 6)
		if err != nil {
			return "erro ao buscar no catálogo", nil
		}
		if len(prods) == 0 {
			return "nenhum produto encontrado para esse termo", nil
		}
		return summarize(prods), prods

	case "get_product":
		var in struct{ Slug string }
		_ = json.Unmarshal(input, &in)
		p, err := e.catalog.GetBySlug(ctx, in.Slug)
		if err != nil || p == nil {
			return "produto não encontrado", nil
		}
		return summarize([]catalog.Product{*p}), []catalog.Product{*p}

	case "list_categories":
		cats, err := e.catalog.Categories(ctx)
		if err != nil {
			return "erro ao listar categorias", nil
		}
		return "categorias: " + strings.Join(cats, ", "), nil

	default:
		return "ferramenta desconhecida", nil
	}
}

// summarize formata produtos como texto compacto pro modelo (fatos, sem inventar).
func summarize(prods []catalog.Product) string {
	var b strings.Builder
	for _, p := range prods {
		brand := ""
		if p.Brand != nil {
			brand = " · " + *p.Brand
		}
		fmt.Fprintf(&b, "- %s%s | R$ %.2f | estoque %d | slug=%s\n", p.Name, brand, p.Price, p.Stock, p.Slug)
	}
	return strings.TrimSpace(b.String())
}
