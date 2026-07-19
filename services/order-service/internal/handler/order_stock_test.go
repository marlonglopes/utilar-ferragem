package handler

// Teste interno (package handler) porque checkStock é a função pura que
// implementa o piso de proteção de estoque e não vale a pena exportar só
// pra testar.

import (
	"testing"

	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/model"
)

func item(productID string, qty int) model.OrderItem {
	return model.OrderItem{
		ProductID: productID, Name: "Produto", SellerID: "s1", SellerName: "Loja",
		Quantity: qty, UnitPrice: 10,
	}
}

func catalogWith(stock map[string]float64) map[string]*catalogclient.Product {
	out := make(map[string]*catalogclient.Product, len(stock))
	for id, s := range stock {
		out[id] = &catalogclient.Product{ID: id, Name: "Produto", Price: 10, Stock: s}
	}
	return out
}

func TestCheckStock_AllowsWithinStock(t *testing.T) {
	got := checkStock(
		[]model.OrderItem{item("p1", 3), item("p2", 1)},
		catalogWith(map[string]float64{"p1": 5, "p2": 1}),
	)
	if got != nil {
		t.Errorf("pedido dentro do estoque não deveria falhar: %+v", got)
	}
}

// REGRESSÃO — este é O buraco: dava pra fechar um pedido de 999 unidades de um
// produto com estoque 1, porque applyAuthoritativePricing só usava Price e Name
// e ignorava o Stock que já vinha na resposta do catálogo.
func TestCheckStock_RejectsOrderAboveStock(t *testing.T) {
	got := checkStock(
		[]model.OrderItem{item("p1", 999)}, // 999 = teto do binding de Quantity
		catalogWith(map[string]float64{"p1": 1}),
	)
	if got == nil {
		t.Fatal("pedir 999 de um produto com estoque 1 tem que ser rejeitado")
	}
	if got.ProductID != "p1" || got.Requested != 999 || got.Available != 1 {
		t.Errorf("shortage deveria dizer qual item e quanto há, veio %+v", got)
	}
}

// Estoque exato passa — rejeitar aqui impediria vender a última unidade.
func TestCheckStock_AllowsExactStock(t *testing.T) {
	if got := checkStock([]model.OrderItem{item("p1", 4)}, catalogWith(map[string]float64{"p1": 4})); got != nil {
		t.Errorf("pedir exatamente o estoque disponível deveria passar: %+v", got)
	}
}

func TestCheckStock_RejectsZeroStock(t *testing.T) {
	if got := checkStock([]model.OrderItem{item("p1", 1)}, catalogWith(map[string]float64{"p1": 0})); got == nil {
		t.Fatal("produto esgotado deveria ser rejeitado")
	}
}

// REGRESSÃO: linhas duplicadas do mesmo produto precisam ser SOMADAS antes da
// comparação. Checar item a item deixaria passar 2 unidades de um produto que
// só tem 1.
func TestCheckStock_SumsDuplicateLines(t *testing.T) {
	got := checkStock(
		[]model.OrderItem{item("p1", 1), item("p1", 1)},
		catalogWith(map[string]float64{"p1": 1}),
	)
	if got == nil {
		t.Fatal("duas linhas de 1 unidade num produto com estoque 1 deveria falhar")
	}
	if got.Requested != 2 {
		t.Errorf("quantidade pedida deveria ser somada (2), veio %d", got.Requested)
	}
}

// Encontra o item errado mesmo quando não é o primeiro do carrinho.
func TestCheckStock_ReportsOffendingItemNotFirst(t *testing.T) {
	got := checkStock(
		[]model.OrderItem{item("p1", 1), item("p2", 50), item("p3", 1)},
		catalogWith(map[string]float64{"p1": 10, "p2": 2, "p3": 10}),
	)
	if got == nil || got.ProductID != "p2" {
		t.Fatalf("deveria apontar p2 como o item sem saldo, veio %+v", got)
	}
}

// Catálogo indisponível (dev mode, mapa vazio) não bloqueia — a validação de
// preço já é pulada nesse caminho, e barrar aqui quebraria o smoke test local.
func TestCheckStock_NoCatalogSkipsValidation(t *testing.T) {
	if got := checkStock([]model.OrderItem{item("p1", 999)}, map[string]*catalogclient.Product{}); got != nil {
		t.Errorf("sem catálogo não há o que validar, veio %+v", got)
	}
}

func TestReservationTTL_BoletoHoldsLonger(t *testing.T) {
	// Boleto compensa em até 3 dias úteis; 30min descartaria a reserva antes
	// do dinheiro cair e o cliente pagaria por um item já vendido a outro.
	if reservationTTL(model.MethodBoleto) <= reservationTTL(model.MethodPix) {
		t.Error("TTL do boleto tem que ser maior que o do Pix")
	}
	if reservationTTL(model.MethodCard) != reservationTTL(model.MethodPix) {
		t.Error("cartão e Pix resolvem em minutos, mesmo TTL")
	}
}

func TestNewUUIDv4_FormatAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		id, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if len(id) != 36 || id[14] != '4' {
			t.Fatalf("UUID v4 malformado: %q", id)
		}
		if seen[id] {
			t.Fatalf("UUID repetido: %q", id)
		}
		seen[id] = true
	}
}
