package catalogclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regressão de contrato entre serviços.
//
// A migration 005 do catalog-service trocou `products.stock` de INT para
// NUMERIC(14,3), pra permitir vender fração (2,5 m de cabo, 1,5 m³ de areia).
// A API passou a poder serializar `"stock": 847.5`.
//
// O order-service decodificava em `Stock int`, e json.Unmarshal recusa número
// fracionário em inteiro. O efeito em produção seria: todo produto com estoque
// fracionado quebrava a validação de estoque na criação do pedido — e como o
// erro vem do decode e não da regra de negócio, viraria "erro ao criar pedido"
// genérico, sem dizer que o problema é o estoque. Build passa; só quebra em
// runtime, com dado real.
//
// Estes testes travam a compatibilidade nos dois sentidos: o inteiro continua
// funcionando (a maioria dos produtos é unidade) e o fracionário passa a
// funcionar.
func TestProduct_AceitaEstoqueFracionado(t *testing.T) {
	cases := []struct {
		name string
		json string
		want float64
	}{
		{"inteiro (produto por unidade)", `{"id":"p1","name":"Cimento","price":34.90,"stock":200}`, 200},
		{"fracionado (cabo por metro)", `{"id":"p2","name":"Cabo 2,5mm","price":1.89,"stock":847.5}`, 847.5},
		{"fracionado com 3 casas (areia m³)", `{"id":"p3","name":"Areia","price":128,"stock":1.375}`, 1.375},
		{"zero", `{"id":"p4","name":"Tinta","price":264,"stock":0}`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p Product
			if err := json.Unmarshal([]byte(tc.json), &p); err != nil {
				t.Fatalf("decode falhou (contrato quebrado com o catalog-service): %v", err)
			}
			if p.Stock != tc.want {
				t.Errorf("Stock = %v, quero %v", p.Stock, tc.want)
			}
		})
	}
}

// O `details` do 409 de estoque insuficiente carrega o saldo disponível, que
// pelo mesmo motivo pode ser fracionário. Se o decode falhar aqui, o cliente
// perde a mensagem que diz QUANTO tem — que é justamente a informação útil.
func TestShortage_AceitaSaldoFracionado(t *testing.T) {
	body := `{"error":"insufficient stock","code":"insufficient_stock",
	          "details":{"productId":"p2","requested":900,"available":847.5}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var env struct {
		Details Shortage `json:"details"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&env); err != nil {
		t.Fatalf("decode do details falhou: %v", err)
	}
	if env.Details.Available != 847.5 {
		t.Errorf("Available = %v, quero 847.5", env.Details.Available)
	}
	if env.Details.Requested != 900 {
		t.Errorf("Requested = %v, quero 900", env.Details.Requested)
	}
}
