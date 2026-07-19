// Package pricing resolve o preço unitário de um produto para uma quantidade.
//
// PORQUÊ um pacote separado, sem banco e sem gin: esta regra é consumida por
// três lugares diferentes — a API de produto (mostrar "a partir de 10 un: R$
// X"), o order-service (cobrar o valor certo no fechamento) e o PDV do balcão
// (calcular a margem antes de dar desconto). Se ela morasse dentro de um
// handler, o order-service reimplementaria — e duas implementações de preço
// divergem, sempre. Função pura também é o único jeito de testar as bordas
// (quantidade na fronteira exata da faixa) sem subir Postgres.
package pricing

import "sort"

// Tier é uma faixa de atacado: comprando `MinQty` ou mais, o unitário é `Price`.
//
// Modelo "a partir de N" (sem limite superior) em vez de faixas fechadas
// (min..max): é como o balcão já negocia ("de 10 pra cima sai por X") e não
// deixa buraco entre faixas quando alguém digita 1-9 e 11-20 por engano.
type Tier struct {
	MinQty float64 `json:"minQty"`
	Price  float64 `json:"price"`
}

// Resolve devolve o preço UNITÁRIO para `qty`, e a faixa que o justificou.
//
// Regra: vence a faixa de maior MinQty que `qty` alcança. Se nenhuma alcança
// (ou não há faixas), vale `base` — o preço de balcão do produto.
//
// Deliberadamente NÃO exige que as faixas sejam decrescentes em preço: uma
// faixa mais cara para quantidade maior é um erro de cadastro, mas escondê-lo
// cobrando o preço "melhor" faria a loja vender por um valor que ninguém
// cadastrou. A validação disso é da rota de escrita (ValidateTiers), não daqui.
//
// Quantidade <= 0 devolve o preço base: é entrada inválida, e devolver a faixa
// de atacado nesse caso mascararia o bug para quem chamou.
func Resolve(base float64, tiers []Tier, qty float64) (unit float64, matched *Tier) {
	if qty <= 0 || len(tiers) == 0 {
		return base, nil
	}

	// Cópia antes de ordenar: `tiers` costuma vir direto do scan de uma query e
	// reordenar o slice do chamador é um efeito colateral invisível.
	sorted := make([]Tier, len(tiers))
	copy(sorted, tiers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinQty < sorted[j].MinQty })

	unit = base
	for i := range sorted {
		if qty >= sorted[i].MinQty {
			unit = sorted[i].Price
			matched = &sorted[i]
			continue
		}
		// Ordenado: a partir daqui nenhuma outra faixa alcança.
		break
	}
	return unit, matched
}

// Total é o valor da linha (unitário resolvido × quantidade).
//
// Existe como função e não como multiplicação no chamador para que a decisão
// de arredondamento fique num lugar só quando trocarmos float64 por decimal —
// mudança já sinalizada em docs/ingestao-de-produtos.md, porque venda
// fracionada (2,5 × R$ 1,89) tira o float64 da zona segura para dinheiro.
func Total(base float64, tiers []Tier, qty float64) float64 {
	unit, _ := Resolve(base, tiers, qty)
	return unit * qty
}

// MinTierPrice é o menor preço alcançável com atacado — o "a partir de R$ X"
// da vitrine. Devolve `base` se nenhuma faixa for mais barata.
func MinTierPrice(base float64, tiers []Tier) float64 {
	min := base
	for _, t := range tiers {
		if t.Price < min {
			min = t.Price
		}
	}
	return min
}
