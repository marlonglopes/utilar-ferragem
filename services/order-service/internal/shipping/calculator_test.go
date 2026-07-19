package shipping_test

import (
	"errors"
	"testing"

	"github.com/utilar/order-service/internal/shipping"
)

// Tabela de teste espelhando o seed da migration 002 (capital + interior),
// montada em memória — o cálculo é puro e não precisa de banco.
func testRates() []shipping.Rate {
	return []shipping.Rate{
		{ID: "1", ZoneName: "São Paulo - Capital", CEPStart: 1000000, CEPEnd: 5999999,
			ServiceCode: "standard", ServiceName: "Entrega padrão",
			BaseCost: 19.90, CostPerItem: 2.50, DeliveryDays: 2, FreeAbove: 299.00, Active: true},
		{ID: "2", ZoneName: "São Paulo - Capital", CEPStart: 1000000, CEPEnd: 5999999,
			ServiceCode: "express", ServiceName: "Entrega expressa",
			BaseCost: 39.90, CostPerItem: 4.00, DeliveryDays: 1, FreeAbove: 0, Active: true},
		{ID: "3", ZoneName: "Interior de SP", CEPStart: 10000000, CEPEnd: 19999999,
			ServiceCode: "standard", ServiceName: "Entrega padrão",
			BaseCost: 34.90, CostPerItem: 3.50, DeliveryDays: 5, FreeAbove: 499.00, Active: true},
		{ID: "4", ZoneName: "Zona desativada", CEPStart: 20000000, CEPEnd: 29999999,
			ServiceCode: "standard", ServiceName: "Entrega padrão",
			BaseCost: 10.00, CostPerItem: 0, DeliveryDays: 3, FreeAbove: 0, Active: false},
	}
}

func TestNormalizeCEP(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"01310-100", 1310100},
		{"01310100", 1310100},
		{" 01310-100 ", 1310100},
		{"99999-999", 99999999},
	}
	for _, tc := range ok {
		got, err := shipping.NormalizeCEP(tc.in)
		if err != nil {
			t.Errorf("NormalizeCEP(%q) erro inesperado: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeCEP(%q) = %d, queria %d", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"", "123", "0131010", "013101000", "abcdefgh", "01310-10a"} {
		if _, err := shipping.NormalizeCEP(bad); !errors.Is(err, shipping.ErrInvalidCEP) {
			t.Errorf("NormalizeCEP(%q) deveria falhar com ErrInvalidCEP, veio %v", bad, err)
		}
	}
}

func TestCalculate_CapitalTwoOptions(t *testing.T) {
	opts, err := shipping.Calculate(testRates(), shipping.Quote{
		CEP: "01310-100", Subtotal: 150.00, ItemCount: 2,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("esperava 2 opções na capital, veio %d: %+v", len(opts), opts)
	}

	// Ordenado da mais barata pra mais cara.
	// standard: 19.90 + 2.50*2 = 24.90 ; express: 39.90 + 4.00*2 = 47.90
	if opts[0].ServiceCode != "standard" || opts[0].Cost != 24.90 {
		t.Errorf("opção 0 = %+v; queria standard R$24.90", opts[0])
	}
	if opts[1].ServiceCode != "express" || opts[1].Cost != 47.90 {
		t.Errorf("opção 1 = %+v; queria express R$47.90", opts[1])
	}
	if opts[0].DeliveryDays != 2 || opts[1].DeliveryDays != 1 {
		t.Errorf("prazos errados: %+v", opts)
	}
}

func TestCalculate_FreeShippingAboveThreshold(t *testing.T) {
	opts, err := shipping.Calculate(testRates(), shipping.Quote{
		CEP: "01310100", Subtotal: 350.00, ItemCount: 3,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	var std *shipping.Option
	for i := range opts {
		if opts[i].ServiceCode == "standard" {
			std = &opts[i]
		}
	}
	if std == nil {
		t.Fatal("opção standard ausente")
	}
	if std.Cost != 0 || !std.Free {
		t.Errorf("acima de R$299 o standard deveria ser grátis, veio %+v", *std)
	}

	// O express NÃO tem frete grátis (free_above = 0) — regra por faixa, e essa
	// distinção é o ponto do campo existir por linha e não ser global.
	for _, o := range opts {
		if o.ServiceCode == "express" && (o.Free || o.Cost == 0) {
			t.Errorf("express não deveria ter frete grátis: %+v", o)
		}
	}
}

// REGRESSÃO: exatamente no limiar já vale grátis (>=, não >).
func TestCalculate_FreeShippingAtExactThreshold(t *testing.T) {
	opts, _ := shipping.Calculate(testRates(), shipping.Quote{
		CEP: "01310100", Subtotal: 299.00, ItemCount: 1,
	})
	if !opts[0].Free {
		t.Errorf("subtotal exatamente no limiar (299.00) deveria dar frete grátis: %+v", opts[0])
	}
}

func TestCalculate_CostGrowsWithItemCount(t *testing.T) {
	one, _ := shipping.Calculate(testRates(), shipping.Quote{CEP: "01310100", Subtotal: 50, ItemCount: 1})
	ten, _ := shipping.Calculate(testRates(), shipping.Quote{CEP: "01310100", Subtotal: 50, ItemCount: 10})
	if !(ten[0].Cost > one[0].Cost) {
		t.Errorf("10 itens deveriam custar mais que 1: %v vs %v", ten[0].Cost, one[0].Cost)
	}
	// 19.90 + 2.50*10 = 44.90
	if ten[0].Cost != 44.90 {
		t.Errorf("custo para 10 itens = %v; queria 44.90", ten[0].Cost)
	}
}

// REGRESSÃO CRÍTICA: CEP sem cobertura tem que ERRAR. Se algum dia isso voltar
// a devolver lista vazia sem erro, o handler calcularia frete zero e a loja
// entregaria de graça pro Brasil inteiro.
func TestCalculate_NoCoverageIsAnError(t *testing.T) {
	_, err := shipping.Calculate(testRates(), shipping.Quote{
		CEP: "70000-000", Subtotal: 100, ItemCount: 1,
	})
	if !errors.Is(err, shipping.ErrNoCoverage) {
		t.Fatalf("esperava ErrNoCoverage, veio %v", err)
	}
}

// Faixa inativa não vale — o operador desliga uma região sem apagar a linha.
func TestCalculate_IgnoresInactiveRates(t *testing.T) {
	_, err := shipping.Calculate(testRates(), shipping.Quote{
		CEP: "20000-000", Subtotal: 100, ItemCount: 1,
	})
	if !errors.Is(err, shipping.ErrNoCoverage) {
		t.Fatalf("faixa inativa não deveria cobrir o CEP, veio %v", err)
	}
}

func TestCalculate_InvalidCEP(t *testing.T) {
	_, err := shipping.Calculate(testRates(), shipping.Quote{CEP: "123", Subtotal: 100, ItemCount: 1})
	if !errors.Is(err, shipping.ErrInvalidCEP) {
		t.Fatalf("esperava ErrInvalidCEP, veio %v", err)
	}
}

// Sobreposição de faixas com o mesmo serviço: a mais barata vence, e a cotação
// não mostra duas linhas "Entrega padrão" com preços diferentes.
func TestCalculate_OverlappingRatesPickCheapest(t *testing.T) {
	rates := append(testRates(), shipping.Rate{
		ID: "5", ZoneName: "Promo Centro", CEPStart: 1300000, CEPEnd: 1400000,
		ServiceCode: "standard", ServiceName: "Entrega padrão",
		BaseCost: 9.90, CostPerItem: 0, DeliveryDays: 3, Active: true,
	})
	opts, err := shipping.Calculate(rates, shipping.Quote{CEP: "01310100", Subtotal: 50, ItemCount: 2})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	count := 0
	for _, o := range opts {
		if o.ServiceCode == "standard" {
			count++
			if o.Cost != 9.90 {
				t.Errorf("deveria vencer a faixa mais barata (9.90), veio %v", o.Cost)
			}
		}
	}
	if count != 1 {
		t.Errorf("esperava 1 opção standard, veio %d", count)
	}
}

func TestCostFor_SelectsRequestedService(t *testing.T) {
	opt, err := shipping.CostFor(testRates(), shipping.Quote{
		CEP: "01310100", Subtotal: 50, ItemCount: 1,
	}, "express")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if opt.ServiceCode != "express" || opt.Cost != 43.90 { // 39.90 + 4.00
		t.Errorf("CostFor(express) = %+v; queria express R$43.90", opt)
	}
}

func TestCostFor_EmptyServiceDefaultsToCheapest(t *testing.T) {
	opt, err := shipping.CostFor(testRates(), shipping.Quote{
		CEP: "01310100", Subtotal: 50, ItemCount: 1,
	}, "")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if opt.ServiceCode != "standard" {
		t.Errorf("sem serviço explícito deveria cobrar o mais barato, veio %q", opt.ServiceCode)
	}
}

// REGRESSÃO: serviço inexistente para a faixa não pode virar frete zero.
func TestCostFor_UnknownServiceIsAnError(t *testing.T) {
	// O interior só tem 'standard' na tabela de teste.
	_, err := shipping.CostFor(testRates(), shipping.Quote{
		CEP: "13000-000", Subtotal: 50, ItemCount: 1,
	}, "express")
	if err == nil {
		t.Fatal("pedir express onde só há standard deveria falhar, não virar frete 0")
	}
}
