package pricing_test

import (
	"math"
	"testing"

	"github.com/utilar/catalog-service/internal/pricing"
)

// cimento é a tabela de exemplo do balcão: saco a R$ 42,90, R$ 39,90 de 10 em
// diante, R$ 36,90 de 50 (palete) em diante.
var cimento = []pricing.Tier{
	{MinQty: 10, Price: 39.90},
	{MinQty: 50, Price: 36.90},
}

func eq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestResolve_SemFaixasUsaPrecoBase(t *testing.T) {
	unit, matched := pricing.Resolve(42.90, nil, 100)
	if !eq(unit, 42.90) {
		t.Errorf("sem faixas o unitário deve ser o base; veio %v", unit)
	}
	if matched != nil {
		t.Errorf("sem faixas não deve haver faixa vencedora; veio %+v", matched)
	}
}

func TestResolve_AbaixoDaPrimeiraFaixaUsaPrecoBase(t *testing.T) {
	unit, matched := pricing.Resolve(42.90, cimento, 9)
	if !eq(unit, 42.90) {
		t.Errorf("9 sacos ainda é varejo; queria 42.90, veio %v", unit)
	}
	if matched != nil {
		t.Errorf("nenhuma faixa deveria vencer com qty=9; veio %+v", matched)
	}
}

// REGRESSÃO: a fronteira EXATA é o erro clássico ("de 10 pra cima" cobrando
// varejo em exatamente 10). MinQty é inclusivo.
func TestResolve_FronteiraExataDaFaixaEInclusiva(t *testing.T) {
	unit, matched := pricing.Resolve(42.90, cimento, 10)
	if !eq(unit, 39.90) {
		t.Errorf("qty=10 deve entrar na faixa de 10+; queria 39.90, veio %v", unit)
	}
	if matched == nil || !eq(matched.MinQty, 10) {
		t.Errorf("faixa vencedora deveria ser MinQty=10; veio %+v", matched)
	}
}

func TestResolve_VenceAFaixaDeMaiorMinQtyAlcancada(t *testing.T) {
	for _, tc := range []struct {
		qty  float64
		want float64
	}{
		{10, 39.90},
		{49, 39.90},
		{50, 36.90},
		{500, 36.90},
	} {
		if unit, _ := pricing.Resolve(42.90, cimento, tc.qty); !eq(unit, tc.want) {
			t.Errorf("qty=%v: queria %v, veio %v", tc.qty, tc.want, unit)
		}
	}
}

// REGRESSÃO: as faixas chegam do banco em qualquer ordem (e o ORDER BY pode
// sumir num refactor de query). A função ordena por conta própria.
func TestResolve_IndependeDaOrdemDasFaixas(t *testing.T) {
	desordenado := []pricing.Tier{
		{MinQty: 50, Price: 36.90},
		{MinQty: 10, Price: 39.90},
	}
	if unit, _ := pricing.Resolve(42.90, desordenado, 12); !eq(unit, 39.90) {
		t.Errorf("faixas fora de ordem quebraram a resolução; veio %v", unit)
	}
}

// REGRESSÃO: Resolve não pode reordenar o slice do chamador — o handler reusa
// o mesmo slice pra serializar as faixas na resposta.
func TestResolve_NaoMutaOSliceDoChamador(t *testing.T) {
	original := []pricing.Tier{
		{MinQty: 50, Price: 36.90},
		{MinQty: 10, Price: 39.90},
	}
	pricing.Resolve(42.90, original, 100)
	if !eq(original[0].MinQty, 50) || !eq(original[1].MinQty, 10) {
		t.Errorf("Resolve reordenou o slice do chamador: %+v", original)
	}
}

// Venda fracionada: 2,5 m de cabo. Se algum dia alguém trocar float64 por int
// aqui, este teste cai.
func TestResolve_QuantidadeFracionada(t *testing.T) {
	cabo := []pricing.Tier{{MinQty: 2.5, Price: 4.50}}
	if unit, _ := pricing.Resolve(5.90, cabo, 2.5); !eq(unit, 4.50) {
		t.Errorf("2,5 m deveria alcançar a faixa de 2,5; veio %v", unit)
	}
	if unit, _ := pricing.Resolve(5.90, cabo, 2.4); !eq(unit, 5.90) {
		t.Errorf("2,4 m ainda é varejo; veio %v", unit)
	}
}

// Quantidade inválida devolve base — não a faixa mais barata. Devolver atacado
// pra qty<=0 mascararia bug de carrinho no chamador.
func TestResolve_QuantidadeInvalidaNaoGanhaAtacado(t *testing.T) {
	for _, qty := range []float64{0, -1, -100} {
		if unit, m := pricing.Resolve(42.90, cimento, qty); !eq(unit, 42.90) || m != nil {
			t.Errorf("qty=%v deveria devolver base sem faixa; veio %v / %+v", qty, unit, m)
		}
	}
}

// Faixa mal cadastrada (mais cara na quantidade maior) é cobrada como está.
// Documenta a decisão: quem valida é a rota de escrita, não a resolução.
func TestResolve_FaixaInvertidaNaoEMascarada(t *testing.T) {
	invertida := []pricing.Tier{
		{MinQty: 10, Price: 39.90},
		{MinQty: 50, Price: 44.90}, // erro de cadastro
	}
	if unit, _ := pricing.Resolve(42.90, invertida, 60); !eq(unit, 44.90) {
		t.Errorf("a faixa cadastrada deve valer como está; veio %v", unit)
	}
}

func TestTotal_MultiplicaPeloUnitarioResolvido(t *testing.T) {
	if got := pricing.Total(42.90, cimento, 50); !eq(got, 36.90*50) {
		t.Errorf("total de 50 sacos: queria %v, veio %v", 36.90*50, got)
	}
}

func TestMinTierPrice(t *testing.T) {
	if got := pricing.MinTierPrice(42.90, cimento); !eq(got, 36.90) {
		t.Errorf("menor preço com atacado: queria 36.90, veio %v", got)
	}
	if got := pricing.MinTierPrice(42.90, nil); !eq(got, 42.90) {
		t.Errorf("sem faixas, o menor é o base; veio %v", got)
	}
	// Faixa mais cara que o base não pode virar "a partir de".
	caro := []pricing.Tier{{MinQty: 10, Price: 99.00}}
	if got := pricing.MinTierPrice(42.90, caro); !eq(got, 42.90) {
		t.Errorf("faixa mais cara não pode baixar o 'a partir de'; veio %v", got)
	}
}
