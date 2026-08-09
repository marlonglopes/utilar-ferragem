package cashback

import "testing"

func cfg() Config {
	return Config{EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90, Active: true}
}

func TestEarn(t *testing.T) {
	c := cfg()
	cases := []struct {
		nome  string
		basis float64
		want  float64
	}{
		{"5% de 100", 100, 5},
		{"arredonda 2 casas (5% de 33.33 = 1.6665→1.67)", 33.33, 1.67},
		{"base zero", 0, 0},
		{"base negativa", -10, 0},
	}
	for _, tc := range cases {
		if got := Earn(c, tc.basis); got != tc.want {
			t.Errorf("%s: Earn=%v, quero %v", tc.nome, got, tc.want)
		}
	}

	// Programa desligado não acumula.
	off := c
	off.Active = false
	if got := Earn(off, 100); got != 0 {
		t.Errorf("programa desligado: Earn=%v, quero 0", got)
	}
}

func TestClampRedeem(t *testing.T) {
	c := cfg() // teto 50%
	cases := []struct {
		nome                         string
		requested, balance, subtotal float64
		want                         float64
	}{
		// Pedido 200, teto 50% = 100. Saldo 30 → limita pelo saldo.
		{"limita pelo saldo", 200, 30, 200, 30},
		// Saldo 500, teto 50% de 200 = 100 → limita pelo teto do pedido.
		{"limita pelo teto do pedido", 500, 500, 200, 100},
		// Cliente pede 40, tem 500, teto 100 → respeita o pedido.
		{"respeita o pedido do cliente", 40, 500, 200, 40},
		// Sem saldo.
		{"sem saldo", 100, 0, 200, 0},
		// Pedido zero.
		{"pedido zero", 100, 100, 0, 0},
		// Requisição negativa (tamper) → 0.
		{"requisição negativa", -50, 100, 200, 0},
	}
	for _, tc := range cases {
		if got := ClampRedeem(c, tc.requested, tc.balance, tc.subtotal); got != tc.want {
			t.Errorf("%s: ClampRedeem=%v, quero %v", tc.nome, got, tc.want)
		}
	}

	// Programa desligado não deixa resgatar nem com saldo.
	off := c
	off.Active = false
	if got := ClampRedeem(off, 50, 100, 200); got != 0 {
		t.Errorf("programa desligado: ClampRedeem=%v, quero 0", got)
	}
}
