package handler

import (
	"strings"
	"testing"

	"github.com/utilar/catalog-service/internal/model"
)

// Testes de UNIDADE da busca — não precisam de banco. Cobrem a parte do
// saneamento que é responsabilidade nossa (o que chega ao to_tsquery) e a
// sugestão "você quis dizer", que é 100% Go.

// ============================================================================
// Entrada hostil — a superfície de CT1-C1 depois do tsvector
// ============================================================================

// REGRESSÃO: `to_tsquery` LANÇA EXCEÇÃO com sintaxe inválida, e exceção no
// banco vira 500 na vitrine. Este teste trava a invariante que impede isso:
// o que sai de tsPrefixQuery só contém tokens alfanuméricos, ` & ` e o sufixo
// `:*` — NUNCA um caractere de sintaxe vindo do usuário.
//
// Sem isso, `?q=%26%26%26` (`&&&`) derrubaria a busca com uma query string.
func TestSegurancaBusca_EntradaHostilNuncaGeraSintaxeDeTsquery(t *testing.T) {
	hostis := []string{
		"&&&",
		"|||",
		"!!!",
		"((((((((((",
		"a & b | !c",
		"'; DROP TABLE products; --",
		`\\\\\\`,
		"furadeira:*:*:*",
		"a:*&b:*",
		"%_%_%_%_%_%_%_%_%_%", // o payload de ReDoS do pg_trgm (audit CT1-C1)
		"<script>alert(1)</script>",
		"\x00\x01\x02",
		strings.Repeat("&", 500),
		strings.Repeat("a b ", 200),
		"—–…«»",
	}

	// Só isto pode aparecer na saída, além de letras e dígitos.
	const permitidos = " &:*"

	for _, h := range hostis {
		got := tsPrefixQuery(h)
		for _, r := range got {
			if strings.ContainsRune(permitidos, r) {
				continue
			}
			if !isAlnum(r) {
				t.Errorf("tsPrefixQuery(%q) = %q — vazou o caractere de sintaxe %q", h, got, r)
			}
		}
		// A tsquery gerada não pode ter operador solto (`& &`, começar ou
		// terminar com `&`) — isso também é erro de sintaxe no to_tsquery.
		if strings.Contains(got, "& &") || strings.HasPrefix(got, "&") || strings.HasSuffix(got, "&") {
			t.Errorf("tsPrefixQuery(%q) = %q — operador solto vira exceção no to_tsquery", h, got)
		}
	}
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r > 127 // letra acentuada é alfanumérica pro nosso propósito
}

// REGRESSÃO: sem teto de palavras, `?q=` com 100 termos vira uma tsquery de
// 100 ramos `&` com prefixo — cara mesmo com índice. Mesmo DoS que CT1-M1
// tratou no ILIKE.
func TestSegurancaBusca_LimitaNumeroDeTermos(t *testing.T) {
	got := tsPrefixQuery(strings.Repeat("furadeira ", 100))
	if n := strings.Count(got, "&") + 1; n > maxSearchWords {
		t.Errorf("gerou %d termos, teto é %d — tsquery: %q", n, maxSearchWords, got)
	}
}

// REGRESSÃO: entrada que vira vazia depois do saneamento tem que devolver ""
// e não `":*"` — que é sintaxe inválida e viraria exceção.
func TestSegurancaBusca_TermoSoDePontuacaoDevolveVazio(t *testing.T) {
	for _, in := range []string{"", "   ", "!!!", "&&&", "---", "...", "@#$%"} {
		if got := tsPrefixQuery(in); got != "" {
			t.Errorf("tsPrefixQuery(%q) = %q, queria vazio", in, got)
		}
	}
}

// ============================================================================
// Prefixo / autocomplete
// ============================================================================

func TestPrefixo_SoUltimoTermoGanhaEstrela(t *testing.T) {
	casos := map[string]string{
		"furad":             "furad:*",
		"furadeira":         "furadeira:*",
		"furadeira imp":     "furadeira & imp:*",
		"tinta acrilica 18": "tinta & acrilica & 18:*",
	}
	for in, want := range casos {
		if got := tsPrefixQuery(in); got != want {
			t.Errorf("tsPrefixQuery(%q) = %q, queria %q", in, got, want)
		}
	}
}

// O ramo de prefixo é um OR. Se o usuário usou operador (aspas ou `-negação`),
// acrescentar esse OR DESFARIA a restrição que ele pediu: `-bosch` excluiria a
// Bosch pelo websearch e o prefixo a traria de volta.
func TestPrefixo_RespeitaOperadoresDoUsuario(t *testing.T) {
	for _, in := range []string{`"furadeira de impacto"`, `-bosch`, `furadeira -bosch`, `"tinta"`} {
		if got := tsPrefixQuery(in); got != "" {
			t.Errorf("tsPrefixQuery(%q) = %q — com operador o ramo de prefixo tem que sair", in, got)
		}
	}
}

// ============================================================================
// Sugestão "você quis dizer"
// ============================================================================

func produtos(nomes ...string) []model.Product {
	out := make([]model.Product, 0, len(nomes))
	for _, n := range nomes {
		out = append(out, model.Product{Name: n})
	}
	return out
}

func TestSugestao_CorrigeApenasAPalavraErrada(t *testing.T) {
	casos := []struct {
		q     string
		nomes []string
		want  string
	}{
		{"furadera", []string{`Furadeira de Impacto 1/2" 750W`}, "furadeira"},
		{"argamasa", []string{"Argamassa Colante AC-I Interno Saco 20kg"}, "argamassa"},
		{"paraffuso", []string{"Parafuso Rosca Soberba 4,8 x 50mm Cento"}, "parafuso"},
		// Só a 1ª palavra está errada: "impacto" já está certo e não pode ser
		// trocado por outra coisa.
		{"furadera impacto", []string{`Furadeira de Impacto 750W`}, "furadeira impacto"},
	}
	for _, c := range casos {
		if got := suggestFromNames(c.q, produtos(c.nomes...)); got != c.want {
			t.Errorf("suggestFromNames(%q) = %q, queria %q", c.q, got, c.want)
		}
	}
}

// REGRESSÃO: "Mostrando resultados para **cimento**" quando o usuário digitou
// exatamente "cimento" faz a loja parecer quebrada. Sem correção, sem sugestão.
func TestSugestao_VaziaQuandoNaoHaNadaACorrigir(t *testing.T) {
	if got := suggestFromNames("parafuso", produtos("Parafuso Rosca Soberba 4,8 x 50mm")); got != "" {
		t.Errorf("suggestFromNames devolveu %q para termo já correto", got)
	}
	if got := suggestFromNames("qualquer", nil); got != "" {
		t.Errorf("sem produtos a sugestão tem que ser vazia, veio %q", got)
	}
}

// A sugestão tem que usar o MESMO corte que selecionou as linhas. Se divergir,
// a loja mostra "você quis dizer X" e devolve uma lista sem nada de X.
func TestSugestao_NaoInventaCorrecaoAbaixoDoLimiar(t *testing.T) {
	// "bucha" e "Parafuso"/"Rosca" não têm parentesco trigram nenhum.
	if got := suggestFromNames("bucha", produtos("Parafuso Rosca Soberba 4,8 x 50mm")); got != "" {
		t.Errorf("suggestFromNames inventou %q — nada ali passa de %.2f", got, fuzzyThresholdFloat)
	}
}

// trigramSimilarity é usada só pra ESCOLHER o texto da sugestão, mas se ela
// divergir do pg_trgm a sugestão exibida deixa de casar com o resultado.
// Os valores esperados vêm do próprio Postgres (`SELECT similarity(a,b)`).
func TestTrigramSimilarity_AcompanhaOPostgres(t *testing.T) {
	casos := []struct {
		a, b string
		want float64
	}{
		{"furadera", "furadeira", 0.6},
		{"cimento", "cimento", 1.0},
		{"bucha", "parafuso", 0.0},
	}
	for _, c := range casos {
		got := trigramSimilarity(c.a, c.b)
		if got < c.want-0.12 || got > c.want+0.12 {
			t.Errorf("trigramSimilarity(%q,%q) = %.3f, esperado ~%.2f", c.a, c.b, got, c.want)
		}
	}
}

// normalizeWord é o que permite comparar "Acrílica" com "acrilico" — o erro
// que o cliente comete de verdade (sem acento e no gênero errado).
func TestNormalizeWord_TiraAcentoCaixaEPontuacao(t *testing.T) {
	casos := map[string]string{
		"Acrílica":     "acrilica",
		"ELÉTRICA":     "eletrica",
		`1/2"`:         "12",
		"Máscara,":     "mascara",
		"Sifão":        "sifao",
		"Descartável.": "descartavel",
	}
	for in, want := range casos {
		if got := normalizeWord(in); got != want {
			t.Errorf("normalizeWord(%q) = %q, queria %q", in, got, want)
		}
	}
}
