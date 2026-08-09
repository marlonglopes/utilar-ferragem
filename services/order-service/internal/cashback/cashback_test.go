package cashback

import (
	"testing"
	"time"
)

func cfg() Config {
	return Config{EarnRatePct: 5, RedeemMaxPct: 50, ValidityDays: 90, Active: true}
}

var refNow = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

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
		if got := Earn(c, tc.basis, refNow); got != tc.want {
			t.Errorf("%s: Earn=%v, quero %v", tc.nome, got, tc.want)
		}
	}

	// Programa desligado não acumula.
	off := c
	off.Active = false
	if got := Earn(off, 100, refNow); got != 0 {
		t.Errorf("programa desligado: Earn=%v, quero 0", got)
	}
}

// Pedido mínimo pra acumular: abaixo do mínimo, não gera cashback.
func TestEarn_MinEarnSubtotal(t *testing.T) {
	c := cfg()
	c.MinEarnSubtotal = 100
	if got := Earn(c, 99.99, refNow); got != 0 {
		t.Errorf("abaixo do mínimo de acúmulo: Earn=%v, quero 0", got)
	}
	if got := Earn(c, 100, refNow); got != 5 {
		t.Errorf("no mínimo de acúmulo: Earn=%v, quero 5", got)
	}
}

// Campanha: taxa turbinada dentro da janela; fora dela, volta pra taxa base.
func TestEarn_Campaign(t *testing.T) {
	start := refNow.Add(-24 * time.Hour)
	end := refNow.Add(24 * time.Hour)
	c := cfg() // base 5%
	c.CampaignRatePct = 10
	c.CampaignStartsAt = &start
	c.CampaignEndsAt = &end

	// Dentro da janela → 10% de 100 = 10.
	if got := Earn(c, 100, refNow); got != 10 {
		t.Errorf("campanha ativa: Earn=%v, quero 10", got)
	}
	// Antes de começar → taxa base 5%.
	if got := Earn(c, 100, start.Add(-time.Hour)); got != 5 {
		t.Errorf("antes da campanha: Earn=%v, quero 5", got)
	}
	// Depois de acabar → taxa base 5%.
	if got := Earn(c, 100, end.Add(time.Hour)); got != 5 {
		t.Errorf("depois da campanha: Earn=%v, quero 5", got)
	}
	// Taxa de campanha 0 → ignora, usa a base.
	c.CampaignRatePct = 0
	if got := Earn(c, 100, refNow); got != 5 {
		t.Errorf("campanha com taxa 0: Earn=%v, quero 5 (base)", got)
	}
}

// Pedido mínimo pra resgatar: abaixo do mínimo, não deixa usar cashback.
func TestClampRedeem_MinRedeemSubtotal(t *testing.T) {
	c := cfg()
	c.MinRedeemSubtotal = 50
	if got := ClampRedeem(c, 20, 100, 49.99); got != 0 {
		t.Errorf("pedido abaixo do mínimo de resgate: ClampRedeem=%v, quero 0", got)
	}
	if got := ClampRedeem(c, 20, 100, 60); got != 20 {
		t.Errorf("acima do mínimo de resgate: ClampRedeem=%v, quero 20", got)
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
