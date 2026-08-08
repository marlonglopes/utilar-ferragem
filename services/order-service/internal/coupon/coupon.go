// Package coupon é a regra PURA do desconto de cupom — sem banco, testável
// isolada, igual ao pacote balcao. O valor sai daqui a partir do subtotal
// autoritativo; o cliente nunca dita valor (manda só o código).
package coupon

import (
	"errors"
	"math"
	"strings"
	"time"
)

const (
	TypePercent = "percent"
	TypeFixed   = "fixed"
)

// Coupon espelha o subset da linha de `coupons` que a regra precisa.
type Coupon struct {
	Code        string
	Type        string
	Value       float64
	MinSubtotal float64
	MaxUses     *int // nil = ilimitado
	Uses        int
	Active      bool
	ExpiresAt   *time.Time // nil = não expira
}

var (
	ErrNotFound    = errors.New("coupon: não encontrado")
	ErrInactive    = errors.New("coupon: inativo")
	ErrExpired     = errors.New("coupon: expirado")
	ErrMinSubtotal = errors.New("coupon: pedido mínimo não atingido")
	ErrExhausted   = errors.New("coupon: limite de usos atingido")
)

// NormalizeCode padroniza o código (maiúsculas, sem espaços) — "obra10" e
// " OBRA10 " são o mesmo cupom.
func NormalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Validate diz se o cupom pode ser aplicado ao subtotal no instante now. A ordem
// dos checks é a das mensagens mais úteis primeiro (inativo/expirado antes de
// "pedido mínimo").
func Validate(c Coupon, subtotal float64, now time.Time) error {
	if !c.Active {
		return ErrInactive
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(now) {
		return ErrExpired
	}
	if c.MaxUses != nil && c.Uses >= *c.MaxUses {
		return ErrExhausted
	}
	if subtotal < c.MinSubtotal {
		return ErrMinSubtotal
	}
	return nil
}

// Apply devolve o valor do desconto (em reais) de um cupom VÁLIDO sobre o
// subtotal. Percentual: subtotal*value/100; fixo: value. Nunca passa do subtotal
// (desconto não deixa o pedido negativo). Arredonda a 2 casas.
func Apply(c Coupon, subtotal float64) float64 {
	var amount float64
	switch c.Type {
	case TypePercent:
		amount = subtotal * c.Value / 100
	case TypeFixed:
		amount = c.Value
	}
	if amount > subtotal {
		amount = subtotal
	}
	if amount < 0 {
		amount = 0
	}
	return math.Round(amount*100) / 100
}
