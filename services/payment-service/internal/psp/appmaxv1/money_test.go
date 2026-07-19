package appmaxv1

import "testing"

// Dinheiro em float64 é a classe de bug mais cara num PSP: a API da Appmax v1 é
// 100% em centavos e o psp.CreateRequest chega em reais, então todo pedido passa
// por ToCents. Estes casos são os que quebram truncamento ingênuo (int64(x*100)),
// onde 19.99*100 == 1998.9999999999998 vira 1998 e o cliente é cobrado a menos.
//
// LIMITE CONHECIDO: valores com meio centavo (1.005, 2.675) não são
// representáveis em float64 — 1.005 é na verdade 1.00499999999999989, e
// math.Round devolve 100, não 101. Isso NÃO é um bug aqui porque o Amount
// autoritativo vem de NUMERIC(12,2) do order-service e nunca tem mais de 2
// casas. Se um dia entrar preço por unidade fracionada (2,5 m de cabo a
// R$ 1,89/m), a multiplicação passa a gerar meio centavo e este helper precisa
// virar decimal de verdade (shopspring/decimal) — não float64.
func TestToCentsRoundsInsteadOfTruncating(t *testing.T) {
	cases := []struct {
		reais float64
		want  int64
	}{
		{0, 0},
		{0.01, 1},
		{0.1, 10},
		{1, 100},
		{19.99, 1999},   // clássico do float: 1998.9999999999998
		{34.90, 3490},   // preço real do seed (cimento)
		{264.00, 26400}, // tinta 18L
		{1284.50, 128450},
		{99999.99, 9999999},
	}

	for _, tc := range cases {
		if got := ToCents(tc.reais); got != tc.want {
			t.Errorf("ToCents(%.4f) = %d, quero %d (diferença de R$ %.2f)",
				tc.reais, got, tc.want, float64(got-tc.want)/100)
		}
	}
}

// Ida e volta não pode perder centavo em nenhum valor plausível de pedido.
func TestCentsRoundTrip(t *testing.T) {
	for cents := int64(0); cents <= 200000; cents += 7 {
		if got := ToCents(FromCents(cents)); got != cents {
			t.Fatalf("round-trip perdeu precisão: %d → %.2f → %d", cents, FromCents(cents), got)
		}
	}
}

// A soma dos itens tem que bater com o total enviado no pedido. Se cada item for
// convertido isolado e o total for convertido do subtotal em reais, o
// arredondamento pode divergir e a Appmax recusa o pedido por inconsistência.
func TestPerItemCentsSumMatchesOrderTotal(t *testing.T) {
	// 3 itens de R$ 0.005 cada — o caso onde a divergência aparece.
	prices := []float64{34.90, 47.50, 28.40}
	qty := []int64{20, 8, 4}

	var sumCents int64
	var sumReais float64
	for i, p := range prices {
		sumCents += ToCents(p) * qty[i]
		sumReais += p * float64(qty[i])
	}

	if got := ToCents(sumReais); got != sumCents {
		t.Errorf("divergência: soma-por-item = %d centavos, total-convertido = %d centavos (R$ %.2f de diferença)",
			sumCents, got, float64(got-sumCents)/100)
	}
}
