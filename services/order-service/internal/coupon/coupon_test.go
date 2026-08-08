package coupon

import (
	"testing"
	"time"
)

func TestApply(t *testing.T) {
	cases := []struct {
		name     string
		c        Coupon
		subtotal float64
		want     float64
	}{
		{"percent 10%", Coupon{Type: TypePercent, Value: 10}, 200, 20},
		{"percent arredonda", Coupon{Type: TypePercent, Value: 10}, 33.33, 3.33},
		{"fixo abaixo do subtotal", Coupon{Type: TypeFixed, Value: 20}, 200, 20},
		{"fixo maior que o subtotal é limitado", Coupon{Type: TypeFixed, Value: 500}, 200, 200},
		{"percent 100 zera", Coupon{Type: TypePercent, Value: 100}, 80, 80},
	}
	for _, tc := range cases {
		if got := Apply(tc.c, tc.subtotal); got != tc.want {
			t.Errorf("%s: Apply = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestValidate(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	two := 2

	ok := Coupon{Type: TypePercent, Value: 10, Active: true, MinSubtotal: 100, ExpiresAt: &future, MaxUses: &two, Uses: 1}
	if err := Validate(ok, 150, now); err != nil {
		t.Fatalf("cupom válido recusado: %v", err)
	}

	tests := []struct {
		name     string
		c        Coupon
		subtotal float64
		wantErr  error
	}{
		{"inativo", Coupon{Active: false}, 100, ErrInactive},
		{"expirado", Coupon{Active: true, ExpiresAt: &past}, 100, ErrExpired},
		{"esgotado", Coupon{Active: true, Uses: 2, MaxUses: &two}, 100, ErrExhausted},
		{"pedido mínimo", Coupon{Active: true, MinSubtotal: 100}, 50, ErrMinSubtotal},
	}
	for _, tc := range tests {
		if err := Validate(tc.c, tc.subtotal, now); err != tc.wantErr {
			t.Errorf("%s: Validate = %v, want %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	if NormalizeCode(" obra10 ") != "OBRA10" {
		t.Fatal("normalize deve aparar e maiúsculas")
	}
}
