package scrape

import "testing"

// A normalização de categoria é o coração do "não chutar": ou reconhece pelo
// vocabulário de ferragem, ou devolve vazio (=> revisão humana). Nunca inventa.
func TestNormalizeCategoria(t *testing.T) {
	cases := []struct{ bruta, nome, want string }{
		{"Dobradiças", "Dobradiça 3 pol Zincada", "fixacao"},
		{"", "Charneira de aço inox", "fixacao"}, // sinônimo
		{"Ferramentas Elétricas", "Furadeira de Impacto", "ferramentas"},
		{"", "Óculos de Proteção Incolor CA 34082", "seguranca"}, // EPI
		{"Fixação", "Parafuso Chipboard 4x40", "fixacao"},
		{"", "Cadeado de Latão 40mm", "fixacao"}, // fechadura/cadeado = fixação, não "seguranca"(EPI)
		{"Hidráulica", "Tubo PVC Soldável 25mm", "hidraulica"},
		{"Diversos", "Item Misterioso XYZ", CategoriaDesconhecida}, // não reconhece => vazio
	}
	for _, c := range cases {
		if got := NormalizeCategoria(c.bruta, c.nome); got != c.want {
			t.Errorf("NormalizeCategoria(%q, %q) = %q, quero %q", c.bruta, c.nome, got, c.want)
		}
	}
}

func TestParsePreco(t *testing.T) {
	val := func(s string) float64 {
		if p := ParsePreco(s); p != nil {
			return *p
		}
		return -1
	}
	cases := []struct {
		in   string
		want float64
	}{
		{"R$ 12,90", 12.90},
		{"R$ 1.234,56", 1234.56}, // ponto de milhar + vírgula decimal
		{"1234.56", 1234.56},     // ponto decimal (sem vírgula)
		{"R$12", 12},
		{"", -1},        // vazio => sem preço
		{"grátis", -1},  // sem dígito
		{"R$ 0,00", -1}, // zero não é preço válido
	}
	for _, c := range cases {
		if got := val(c.in); got != c.want {
			t.Errorf("ParsePreco(%q) = %v, quero %v", c.in, got, c.want)
		}
	}
}
