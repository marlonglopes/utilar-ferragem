package alice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// toolSugerirComplementares oferece o que acompanha um serviço ou um produto.
//
// Duas origens, DELIBERADAMENTE distintas, e cada sugestão diz de qual veio:
//
//  1. REGRA TÉCNICA — vem da base de conhecimento. "Assentar piso exige
//     argamassa AC-III, espaçador, rejunte e desempenadeira dentada" não é
//     estatística, é fato técnico: sem esses itens o serviço não se executa.
//  2. CO-COMPRA — vem do histórico REAL de pedidos, sempre agregado
//     ("40 clientes levaram junto"). É sinal empírico, mais fraco que a regra
//     técnica, e apresentado como tal.
//
// Misturar as duas origens sem rótulo seria um erro: o vendedor precisa saber
// se está dizendo "você VAI precisar disso" ou "costuma sair junto". A primeira
// afirmação ele defende; a segunda, se apresentada como necessidade, queima a
// confiança dele quando o cliente descobre.
func (e *Engine) toolSugerirComplementares(ctx context.Context, input json.RawMessage, tc *toolCtx) string {
	var in struct {
		Servico string `json:"servico"`
		Slug    string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "erro: argumentos inválidos."
	}
	if strings.TrimSpace(in.Servico) == "" && strings.TrimSpace(in.Slug) == "" {
		return "erro: preciso de um serviço ou de um slug de produto."
	}

	var b strings.Builder
	achouAlgo := false

	// ---- 1) Complementares por REGRA TÉCNICA (base de conhecimento) ----
	if in.Servico != "" {
		s, ok := e.kb.ResolveServico(in.Servico)
		if !ok {
			tc.semFundamento = append(tc.semFundamento, in.Servico)
			b.WriteString(fmt.Sprintf("Não tenho o serviço %q na base — não invente os complementares dele.\n", in.Servico))
		} else {
			achouAlgo = true
			b.WriteString("COMPLEMENTARES POR EXIGÊNCIA TÉCNICA (fato, não estatística):\n")
			b.WriteString("Motivo a usar: \"porque " + strings.ToLower(s.Nome) + " exige isso para executar\".\n\n")

			mapeados := 0
			for _, c := range s.Consumos {
				m, ok := e.kb.Material(c.MaterialID)
				if !ok {
					continue
				}
				fmt.Fprintf(&b, "- %s (%s)\n  POR QUE: %s exige este material. Fonte: %s\n",
					m.Nome, m.UnidVenda, s.Nome, c.Coef.Fonte.Human())
				if mapeados < 5 {
					if p := e.buscarProdutoReal(ctx, m.BuscaCatalogo, tc); p != nil {
						mapeados++
						fmt.Fprintf(&b, "  produto: %s (slug=%s) | R$ %.2f | estoque %d\n",
							p.Name, p.Slug, p.Price, p.Stock)
						if tc.mode.VeCusto() && p.Cost != nil {
							pc := *p
							pc.CalcularMargem()
							fmt.Fprintf(&b, "  [INTERNO] custo R$ %.2f | margem %.1f%%\n", *pc.Cost, deref(pc.Margem))
						}
					}
				}
			}

			ess, _, epi := ferramentasDoServico(e.kb, s)
			if len(ess) > 0 {
				fmt.Fprintf(&b, "\nFERRAMENTAS QUE O SERVIÇO EXIGE: %s\n", strings.Join(ess, "; "))
			}
			if len(epi) > 0 {
				fmt.Fprintf(&b, "EPI (obrigatório, nunca ofereça como opcional): %s\n", strings.Join(epi, ", "))
			}
		}
	}

	// ---- 2) Complementares por CO-COMPRA (histórico agregado de pedidos) ----
	if in.Slug != "" {
		if !e.pedidos.Disponivel() {
			b.WriteString("\n(Sugestão por co-compra indisponível: histórico de pedidos não configurado. " +
				"Não invente números de 'quem comprou também levou'.)\n")
		} else {
			pares, err := e.pedidos.CoCompras(ctx, in.Slug, 5)
			switch {
			case err != nil:
				b.WriteString("\n(Não consegui consultar o histórico de co-compra agora. Não invente.)\n")
			case len(pares) == 0:
				b.WriteString("\n(Sem padrão de co-compra suficiente para este produto — " +
					"abaixo do mínimo de pedidos, não dá para afirmar nada.)\n")
			default:
				achouAlgo = true
				b.WriteString("\nCOMPLEMENTARES POR CO-COMPRA (padrão agregado dos pedidos reais):\n")
				for _, p := range pares {
					fmt.Fprintf(&b, "- %s (slug=%s)\n  POR QUE: apareceu junto em %d pedidos distintos.\n",
						p.Nome, p.Slug, p.Ocorrencias)
				}
				b.WriteString("\nApresente isto como TENDÊNCIA observada, não como exigência técnica. " +
					"Nunca cite clientes: o dado é agregado e anônimo.\n")
			}
		}
	}

	if !achouAlgo {
		return b.String() + "\nNão tenho complementar fundamentado para oferecer aqui. " +
			"Diga isso em vez de sugerir algo por palpite."
	}
	b.WriteString("\nSempre diga POR QUE está sugerindo cada item — sugestão sem motivo o cliente não aceita " +
		"e o vendedor não repassa com confiança.\n")
	return b.String()
}
