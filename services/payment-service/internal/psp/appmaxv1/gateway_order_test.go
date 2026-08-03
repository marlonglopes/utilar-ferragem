package appmaxv1

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// TestBuildOrderInput_MoneyInvariant trava a invariante mais importante da
// itemização: o valor que a Appmax cobra (products_value + shipping_value -
// discount_value) TEM de ser exatamente o total autoritativo, em TODOS os
// cenários — inclusive quando converter cada unit_price/frete pra centavos
// diverge do total por arredondamento. O desconto absorve a diferença; nunca
// cobramos valor diferente do que o order-service mandou.
func TestBuildOrderInput_MoneyInvariant(t *testing.T) {
	cases := []struct {
		name     string
		items    []psp.LineItem
		shipping float64
		total    float64 // reais (autoritativo)
	}{
		{"item único sem frete", []psp.LineItem{{Ref: "p1", Name: "A", Quantity: 1, UnitPrice: 100.00}}, 0, 100.00},
		{"multi-item com frete", []psp.LineItem{
			{Ref: "p1", Name: "A", Quantity: 2, UnitPrice: 19.90},
			{Ref: "p2", Name: "B", Quantity: 1, UnitPrice: 5.50},
		}, 12.00, 57.30},
		{"com desconto (total < itens+frete)", []psp.LineItem{{Ref: "p1", Name: "A", Quantity: 1, UnitPrice: 100.00}}, 10.00, 95.00},
		{"drift de arredondamento absorvido no desconto", []psp.LineItem{{Ref: "p1", Name: "A", Quantity: 3, UnitPrice: 0.335}}, 0, 1.00},
		{"total > itens+frete (diferença vai pro frete)", []psp.LineItem{{Ref: "p1", Name: "A", Quantity: 1, UnitPrice: 10.00}}, 0, 15.00},
		{"quantidade zero vira 1", []psp.LineItem{{Ref: "p1", Name: "A", Quantity: 0, UnitPrice: 10.00}}, 0, 10.00},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			totalCents := ToCents(tc.total)
			oi := buildOrderInput(7, psp.CreateRequest{OrderID: "o-1", Items: tc.items, Shipping: tc.shipping}, totalCents)

			charge := oi.ProductsValue + oi.ShippingValue - oi.DiscountValue
			if charge != totalCents {
				t.Fatalf("cobrança = %d, autoritativo = %d (products=%d shipping=%d discount=%d)",
					charge, totalCents, oi.ProductsValue, oi.ShippingValue, oi.DiscountValue)
			}
			if oi.DiscountValue < 0 || oi.ShippingValue < 0 || oi.ProductsValue < 0 {
				t.Fatalf("valor negativo enviado à Appmax: %+v", oi)
			}
			if oi.CustomerID != 7 {
				t.Errorf("customerID = %d", oi.CustomerID)
			}
		})
	}
}

// Sem itens → item sintético a partir do amount (comportamento antigo, dev/sem order).
func TestBuildOrderInput_SyntheticFallback(t *testing.T) {
	oi := buildOrderInput(3, psp.CreateRequest{OrderID: "abcdef123456"}, 20330)
	if len(oi.Products) != 1 || oi.ProductsValue != 20330 || oi.Products[0].UnitValue != 20330 {
		t.Fatalf("fallback sintético inválido: %+v", oi)
	}
	if oi.DiscountValue != 0 || oi.ShippingValue != 0 {
		t.Errorf("fallback deveria ter desconto/frete zero: %+v", oi)
	}
	if oi.Products[0].SKU == "" || oi.Products[0].Type != ProductTypePhysical {
		t.Errorf("produto sintético sem sku/type: %+v", oi.Products[0])
	}
}

// Com itens, o pedido é realmente itemizado (sku/nome/qty/unit em centavos).
func TestBuildOrderInput_Itemizes(t *testing.T) {
	items := []psp.LineItem{
		{Ref: "prod-1", Name: "Parafuso 3/8", Quantity: 10, UnitPrice: 0.50},
		{Ref: "prod-2", Name: "Furadeira", Quantity: 1, UnitPrice: 299.90},
	}
	total := 5.00 + 299.90 + 20.00 // itens + frete
	oi := buildOrderInput(1, psp.CreateRequest{OrderID: "o", Items: items, Shipping: 20.00}, ToCents(total))

	if len(oi.Products) != 2 {
		t.Fatalf("esperava 2 produtos, veio %d", len(oi.Products))
	}
	if oi.Products[0].SKU != "prod-1" || oi.Products[0].Quantity != 10 || oi.Products[0].UnitValue != 50 {
		t.Errorf("produto 0 = %+v", oi.Products[0])
	}
	if oi.Products[1].Name != "Furadeira" || oi.Products[1].UnitValue != 29990 {
		t.Errorf("produto 1 = %+v", oi.Products[1])
	}
	if oi.ShippingValue != 2000 {
		t.Errorf("shipping = %d, esperava 2000", oi.ShippingValue)
	}
	if oi.DiscountValue != 0 {
		t.Errorf("discount = %d, esperava 0", oi.DiscountValue)
	}
}

// Fluxo completo: CreatePayment com itens envia o pedido ITEMIZADO ao /v1/orders,
// com frete, e a cobrança bate com o amount autoritativo.
func TestCreatePayment_SendsItemizedOrder(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		switch r.URL.Path {
		case "/v1/customers":
			return jsonRespond(w, 201, `{"data":{"customer":{"id":1}}}`)
		case "/v1/orders":
			var in OrderInput
			_ = json.Unmarshal(body, &in)
			if len(in.Products) != 2 {
				t.Errorf("esperava 2 produtos itemizados, veio %d: %s", len(in.Products), body)
			}
			if in.ShippingValue != 1500 {
				t.Errorf("shipping = %d, esperava 1500", in.ShippingValue)
			}
			if in.ProductsValue+in.ShippingValue-in.DiscountValue != 11500 {
				t.Errorf("cobrança != total autoritativo (11500): %+v", in)
			}
			return jsonRespond(w, 201, `{"data":{"order":{"id":42,"status":"pendente"}}}`)
		case "/v1/payments/pix":
			return jsonRespond(w, 200, `{"data":{"order":{"id":42,"status":"pendente"},"payment":{"method":"pix"}}}`)
		}
		return false
	})
	c, _ := s.client(t)
	g := &Gateway{client: c}
	_, err := g.CreatePayment(context.Background(), psp.CreateRequest{
		OrderID: "o-1", Amount: 115.00, Method: psp.MethodPix,
		PayerName: "Maria", PayerEmail: "m@x.com", PayerCPF: "12345678909", PayerPhone: "11999998888",
		Shipping: 15.00,
		Items: []psp.LineItem{
			{Ref: "p1", Name: "A", Quantity: 2, UnitPrice: 40.00}, // 80,00
			{Ref: "p2", Name: "B", Quantity: 1, UnitPrice: 20.00}, // 20,00
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Item sem sku/nome ganha fallback (não manda campo obrigatório vazio → 422).
func TestBuildOrderInput_EmptyRefAndNameFallback(t *testing.T) {
	oi := buildOrderInput(1, psp.CreateRequest{
		OrderID: "abcdef12",
		Items:   []psp.LineItem{{Ref: "  ", Name: "", Quantity: 1, UnitPrice: 10.00}},
	}, 1000)
	if oi.Products[0].SKU == "" {
		t.Errorf("sku vazio não recebeu fallback: %+v", oi.Products[0])
	}
	if oi.Products[0].Name == "" {
		t.Errorf("nome vazio não recebeu fallback: %+v", oi.Products[0])
	}
}
