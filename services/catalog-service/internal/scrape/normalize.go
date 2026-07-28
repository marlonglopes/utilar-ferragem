package scrape

import "strings"

// Normalização de categoria. A nomenclatura de ferragem varia MUITO entre
// fornecedores ("dobradiça" vs "charneira"), então mapeamos o texto bruto (a
// categoria do site + o nome do produto) para o vocabulário CONTROLADO do
// catálogo antes de gravar. As categorias válidas são as 8 do catalog:
//   construcao, eletrica, ferramentas, fixacao, hidraulica, jardim, pintura, seguranca
//
// O escopo do scraper é FERRAGEM (fixação/marcenaria/serralheria), então a
// esmagadora maioria cai em `fixacao` ou `ferramentas`. Locks/dobradiças/
// puxadores são ferragem de fixação — NÃO `seguranca`, que aqui é EPI.
const CategoriaDesconhecida = "" // vazio => item vai para revisão (não inventa)

// keywordCategoria: primeira palavra-chave encontrada (no texto sem acento,
// minúsculo) decide. Ordem importa: a mais específica/frequente primeiro.
var keywordCategoria = []struct {
	kw  string
	cat string
}{
	// Fixação / ferragem de marcenaria e serralheria
	{"dobradica", "fixacao"}, {"charneira", "fixacao"},
	{"fechadura", "fixacao"}, {"trinco", "fixacao"}, {"ferrolho", "fixacao"},
	{"cadeado", "fixacao"}, {"puxador", "fixacao"}, {"macaneta", "fixacao"},
	{"parafuso", "fixacao"}, {"prego", "fixacao"}, {"bucha", "fixacao"},
	{"arruela", "fixacao"}, {"porca", "fixacao"}, {"rebite", "fixacao"},
	{"cantoneira", "fixacao"}, {"suporte", "fixacao"}, {"mao francesa", "fixacao"},
	{"corredica", "fixacao"}, {"trilho", "fixacao"}, {"roldana", "fixacao"},
	{"fita dupla face", "fixacao"}, {"abracadeira", "fixacao"},
	{"fecho", "fixacao"}, {"tarraxa", "fixacao"}, {"grampo", "fixacao"},
	// Ferramentas
	{"alicate", "ferramentas"}, {"chave de fenda", "ferramentas"},
	{"chave philips", "ferramentas"}, {"chave allen", "ferramentas"},
	{"furadeira", "ferramentas"}, {"parafusadeira", "ferramentas"},
	{"broca", "ferramentas"}, {"serra", "ferramentas"}, {"martelo", "ferramentas"},
	{"trena", "ferramentas"}, {"nivel", "ferramentas"}, {"esquadro", "ferramentas"},
	{"lixadeira", "ferramentas"}, {"esmerilhadeira", "ferramentas"},
	{"morsa", "ferramentas"}, {"jogo de", "ferramentas"}, {"kit ferramenta", "ferramentas"},
	// EPI / segurança
	{"oculos de protecao", "seguranca"}, {"luva", "seguranca"},
	{"protetor auricular", "seguranca"}, {"capacete", "seguranca"},
	{"mascara", "seguranca"}, {"bota", "seguranca"},
	// Correlatos de outras categorias (raros no escopo, mas mapeados)
	{"eletroduto", "eletrica"}, {"disjuntor", "eletrica"}, {"tomada", "eletrica"},
	{"tubo", "hidraulica"}, {"conexao", "hidraulica"}, {"registro", "hidraulica"},
	{"tinta", "pintura"}, {"pincel", "pintura"}, {"rolo de la", "pintura"},
	{"cimento", "construcao"}, {"argamassa", "construcao"},
}

// NormalizeCategoria mapeia (categoria bruta do site + nome do produto) para o
// vocabulário controlado. Devolve "" quando não reconhece — e aí o item é
// SINALIZADO para revisão, nunca chutado para uma categoria errada.
func NormalizeCategoria(categoriaBruta, nome string) string {
	hay := unaccentLower(categoriaBruta + " " + nome)
	for _, k := range keywordCategoria {
		if strings.Contains(hay, k.kw) {
			return k.cat
		}
	}
	return CategoriaDesconhecida
}

// unaccentLower deixa minúsculo e remove acentos comuns do pt-BR, sem depender
// de x/text — o dicionário é pequeno e o custo de uma tabela é nada.
func unaccentLower(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'á', 'à', 'â', 'ã', 'ä':
			b.WriteRune('a')
		case 'é', 'è', 'ê', 'ë':
			b.WriteRune('e')
		case 'í', 'ì', 'î', 'ï':
			b.WriteRune('i')
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			b.WriteRune('o')
		case 'ú', 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		case 'ñ':
			b.WriteRune('n')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
