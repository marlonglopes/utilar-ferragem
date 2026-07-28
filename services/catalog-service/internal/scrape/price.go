package scrape

import (
	"strconv"
	"strings"
)

// ParsePreco lê preço em formato brasileiro e devolve reais como float.
// Aceita "R$ 1.234,56", "12,90", "1234.56", "R$12". Vazio/inválido/≤0 => nil
// (sem preço — o schema distingue "sem preço" de "preço zero"). Preço não é
// obrigatório: nem toda página de catálogo mostra preço sem login.
func ParsePreco(s string) *float64 {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == ',' || r == '.' {
			b.WriteRune(r)
		}
	}
	t := b.String()
	if t == "" {
		return nil
	}
	// Formato BR: se há vírgula, ela é o separador decimal e o ponto é milhar.
	if strings.Contains(t, ",") {
		t = strings.ReplaceAll(t, ".", "")
		t = strings.ReplaceAll(t, ",", ".")
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || v <= 0 {
		return nil
	}
	return &v
}
