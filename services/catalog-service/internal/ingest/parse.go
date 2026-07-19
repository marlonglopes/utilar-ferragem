// Package ingest — pipeline de ingestão de produtos (CSV / XLSX / JSON / SINAPI).
//
// Este arquivo é o MOTOR DE PARSING BRASILEIRO, e é a parte mais chata e a mais
// cara de errar de todo o pipeline.
//
// PORQUÊ ele existe separado: `strconv.ParseFloat` resolve o caso feliz e
// destrói o resto. Uma planilha real brasileira chega com "R$ 1.234,56",
// "1,234.56" (quando alguém abriu no Excel em locale en-US), "1234,56", o
// número já convertido em float pelo próprio Excel, um EAN virado
// "7,89123E+12", uma célula com espaço não-quebrável invisível no fim, e um
// "#REF!" onde deveria haver preço.
//
// A regra que orienta tudo aqui: **ambiguidade vira aviso, não palpite
// silencioso**. O modo de falha mais caro do catálogo é "1.234,56" virar
// "1,23" — um erro de vírgula que passa em qualquer validação de "é número
// positivo?" e coloca cimento a R$ 1,23 na vitrine. Quando o valor é
// genuinamente ambíguo, devolvemos o resultado E um aviso, e o aviso sobe até
// a tela de revisão humana.
package ingest

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sentinelas de "sem valor" que planilha brasileira usa. São TRATADAS COMO
// VAZIO, não como zero: célula vazia significa "não informado" e zero significa
// "custa zero" — confundir os dois zera preço de produto em massa.
//
// "0" NÃO está aqui de propósito: zero é um valor legítimo (estoque zerado).
var emptyMarkers = map[string]bool{
	"":     true,
	"-":    true,
	"--":   true,
	"—":    true, // travessão: o Excel autocorrige "-" pra isto
	"–":    true,
	"n/a":  true,
	"na":   true,
	"nd":   true,
	"n/d":  true,
	"null": true,
	"nulo": true,
	// Erros de fórmula do Excel. Chegam como texto e NÃO são zero — são
	// "a planilha está quebrada nesta célula".
	"#ref!":   true,
	"#value!": true,
	"#n/a":    true,
	"#name?":  true,
	"#div/0!": true,
	"#null!":  true,
	"#num!":   true,
}

// CleanText normaliza texto de célula. Aplicado ANTES de qualquer parse.
//
// O espaço não-quebrável (U+00A0) é o vilão silencioso: vem de copiar/colar de
// PDF e de site, é invisível, e faz `"CIMENTO " == "CIMENTO"` dar false — ou
// seja, cria produto duplicado a cada importação sem que ninguém veja o motivo.
func CleanText(s string) string {
	r := strings.NewReplacer(
		" ", " ", // NBSP
		" ", " ", // narrow NBSP
		" ", " ", // figure space
		"\uFEFF", "", // BOM no meio do texto
		"\u200B", "", // zero-width space
		"\t", " ",
		"\r", " ",
		"\n", " ",
	)
	s = r.Replace(s)
	// Colapsa espaços internos: "CIMENTO   CP II" e "CIMENTO CP II" são o mesmo
	// produto, e a planilha traz os dois.
	return strings.Join(strings.Fields(s), " ")
}

// IsEmpty diz se a célula deve ser tratada como ausente.
func IsEmpty(s string) bool {
	return emptyMarkers[strings.ToLower(CleanText(s))]
}

// ParseWarning é uma ambiguidade resolvida por convenção. Não impede o import;
// sobe até a revisão humana pra que a decisão seja VISÍVEL.
type ParseWarning struct {
	Field   string `json:"field"`
	Raw     string `json:"raw"`
	Message string `json:"message"`
}

var sciRe = regexp.MustCompile(`^([+-]?[0-9]+(?:[.,][0-9]+)?)[eE]([+-]?[0-9]+)$`)

// ParseNumber converte texto de célula em float64, resolvendo o formato
// brasileiro, o americano e o que o Excel produziu sozinho.
//
// Devolve (valor, ok, aviso). `ok=false` significa "célula vazia/sem valor" —
// que é diferente de erro e diferente de zero. Erro de formato de verdade
// (texto que não é número) devolve ok=false com aviso preenchido.
//
// REGRAS DE DESAMBIGUAÇÃO, em ordem:
//
//  1. Ambos `.` e `,` presentes → o ÚLTIMO é o separador decimal.
//     "1.234,56" → 1234.56 (BR)   "1,234.56" → 1234.56 (US)
//     Esta regra é segura: nenhum locale usa o mesmo caractere pros dois.
//
//  2. Só `.`, no padrão `^\d{1,3}(\.\d{3})+$` → separador de MILHAR.
//     "11.000" → 11000, "1.234.567" → 1234567.
//     Sem isto, "11.000" viraria 11,0 e o estoque de 11 mil parafusos vira 11.
//
//  3. Só `,`, com exatamente 3 dígitos depois e nenhum outro separador →
//     AMBÍGUO ("1,234" pode ser 1234 en-US ou 1,234 pt-BR). Resolvemos como
//     DECIMAL (o sistema é brasileiro) e emitimos aviso. É o único caso onde
//     não dá pra ter certeza pelo conteúdo.
//
//  4. Qualquer outro `,` → decimal. "1234,56" → 1234.56.
func ParseNumber(field, raw string) (float64, bool, *ParseWarning) {
	s := CleanText(raw)
	if IsEmpty(s) {
		return 0, false, nil
	}

	// Símbolos de moeda e sufixos de unidade colados no número.
	s = strings.NewReplacer("R$", "", "r$", "", "%", "", " ", "").Replace(s)
	if s == "" {
		return 0, false, nil
	}

	// Notação científica — o Excel faz isto com número grande. Precisa vir
	// ANTES da lógica de separador, porque "7.89123E+12" tem ponto que NÃO é
	// nem decimal nem milhar no sentido usual.
	if m := sciRe.FindStringSubmatch(s); m != nil {
		mant := strings.Replace(m[1], ",", ".", 1)
		v, err := strconv.ParseFloat(mant+"e"+m[2], 64)
		if err != nil {
			return 0, false, &ParseWarning{field, raw, "notação científica ilegível"}
		}
		return v, true, &ParseWarning{field, raw,
			"valor em notação científica — o Excel provavelmente destruiu um código longo"}
	}

	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	// Contábil: "(1.234,56)" é negativo.
	case strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"):
		neg, s = true, s[1:len(s)-1]
	}

	var warn *ParseWarning
	lastDot, lastComma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")

	switch {
	case lastDot >= 0 && lastComma >= 0:
		// Regra 1: o último separador é o decimal.
		if lastComma > lastDot {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}

	case lastComma >= 0:
		digitsAfter := len(s) - lastComma - 1
		if strings.Count(s, ",") > 1 {
			// "1,234,567" — vírgula repetida só existe como milhar en-US.
			s = strings.ReplaceAll(s, ",", "")
		} else {
			if digitsAfter == 3 {
				// Regra 3: genuinamente ambíguo.
				warn = &ParseWarning{field, raw,
					"valor ambíguo: interpretado como decimal brasileiro (vírgula = decimal); " +
						"se a planilha usa vírgula de milhar, o valor está 1000× menor"}
			}
			s = strings.Replace(s, ",", ".", 1)
		}

	case lastDot >= 0:
		// Regra 2: ponto de milhar.
		if regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{3})+$`).MatchString(s) {
			s = strings.ReplaceAll(s, ".", "")
		}
		// Senão, ponto decimal — deixa como está.
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, &ParseWarning{field, raw, "não é um número reconhecível"}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, &ParseWarning{field, raw, "número fora de faixa"}
	}
	if neg {
		v = -v
	}
	return v, true, warn
}

// ParseMoney é ParseNumber com as regras de dinheiro: arredonda a 2 casas e
// rejeita negativo.
//
// Arredondar aqui e não na escrita é deliberado: NUMERIC(12,2) do banco
// arredondaria de qualquer jeito, mas silenciosamente — e aí o valor comparado
// no dry-run (que decide "mudou ou não mudou") difere do valor gravado, e a
// importação fica eternamente reportando "atualizado" pro mesmo arquivo.
// Idempotência exige comparar o valor que vai ser gravado.
func ParseMoney(field, raw string) (float64, bool, *ParseWarning) {
	v, ok, w := ParseNumber(field, raw)
	if !ok {
		return 0, false, w
	}
	if v < 0 {
		return 0, false, &ParseWarning{field, raw, "valor monetário negativo"}
	}
	return math.Round(v*100) / 100, true, w
}

// ParsePercent aceita "10%", "10", "0,1" e devolve a FRAÇÃO (0.1).
//
// A ambiguidade real: "0,1" é 10% ou 0,1%? Convenção: valor > 1 sem símbolo é
// tratado como percentual inteiro (10 → 0.1); valor ≤ 1 é tratado como fração
// já pronta. Com o símbolo "%" não há dúvida. O caso ambíguo emite aviso.
func ParsePercent(field, raw string) (float64, bool, *ParseWarning) {
	s := CleanText(raw)
	if IsEmpty(s) {
		return 0, false, nil
	}
	hadSymbol := strings.Contains(s, "%")

	v, ok, w := ParseNumber(field, s)
	if !ok {
		return 0, false, w
	}
	if hadSymbol {
		return v / 100, true, w
	}
	if v > 1 {
		return v / 100, true, w
	}
	return v, true, &ParseWarning{field, raw,
		"percentual sem símbolo e ≤ 1 — interpretado como fração já pronta (0,1 = 10%)"}
}

// Formatos de data que aparecem em planilha brasileira, em ordem de tentativa.
// dd/mm/aaaa vem primeiro: é o formato do país, e "03/04/2026" é 3 de abril
// aqui e 4 de março nos EUA. Errar isso inverte um terço das datas do ano sem
// dar erro nenhum.
var dateLayouts = []string{
	"02/01/2006", "2/1/2006", "02-01-2006",
	"2006-01-02", // ISO — inequívoco
	"02/01/06", "02.01.2006",
	"2006-01-02T15:04:05Z07:00",
	"02/01/2006 15:04:05",
}

// excelEpoch — o Excel conta dias desde 30/12/1899 (o deslocamento de 2 dias
// embute o bug histórico do ano bissexto de 1900, que a Microsoft manteve por
// compatibilidade com o Lotus 1-2-3). Célula formatada como data chega ao XML
// como esse número serial.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// ParseDate converte data de célula. Cobre o caso que o doc chama de "Excel
// adora fazer isso com códigos": a célula foi formatada como data por engano e
// chega como número serial.
func ParseDate(field, raw string) (time.Time, bool, *ParseWarning) {
	s := CleanText(raw)
	if IsEmpty(s) {
		return time.Time{}, false, nil
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true, nil
		}
	}
	// Serial do Excel. Faixa defensiva: 1 (31/12/1899) a 100000 (~ano 2173).
	// Fora disso é número que por acaso parece serial, não data.
	if n, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil && n >= 1 && n <= 100000 {
		return excelEpoch.AddDate(0, 0, int(n)), true, &ParseWarning{field, raw,
			"data lida como número serial do Excel"}
	}
	return time.Time{}, false, &ParseWarning{field, raw, "data em formato não reconhecido"}
}

// ParseCode normaliza CÓDIGO — SKU, EAN, NCM, código SINAPI. Código é
// IDENTIDADE, e a identidade não pode depender de como o Excel resolveu exibir.
//
// Os dois desastres que isto resolve:
//
//  1. Notação científica: "7,89123E+12" é o que o Excel mostra de um EAN-13.
//     Reconstruímos os dígitos. Ainda assim emitimos aviso, porque o Excel
//     PERDE precisão nessa conversão (float64 guarda ~15 dígitos significativos
//     e um GTIN-14 tem 14) — o código pode estar simplesmente errado, e o
//     operador precisa saber pra pedir o arquivo em CSV.
//
//  2. Decimal fantasma: "123456.0" é o mesmo código que "123456". O Excel
//     adiciona o ".0" ao tratar código como número.
func ParseCode(field, raw string) (string, bool, *ParseWarning) {
	s := CleanText(raw)
	if IsEmpty(s) {
		return "", false, nil
	}
	s = strings.ReplaceAll(s, " ", "")

	if m := sciRe.FindStringSubmatch(s); m != nil {
		v, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1)+"e"+m[2], 64)
		if err != nil || v < 0 || v > 1e18 {
			return "", false, &ParseWarning{field, raw, "código em notação científica ilegível"}
		}
		return strconv.FormatFloat(v, 'f', 0, 64), true, &ParseWarning{field, raw,
			"código veio em notação científica — o Excel converteu pra número e PODE ter " +
				"perdido dígitos; confira contra o arquivo original (peça CSV UTF-8)"}
	}

	// "123456.0" / "123456,0" → "123456". Só quando a parte decimal é toda
	// zeros: "12.5" é um número de verdade, não um código maltratado.
	if i := strings.LastIndexAny(s, ".,"); i >= 0 {
		intPart, decPart := s[:i], s[i+1:]
		if decPart != "" && strings.Trim(decPart, "0") == "" && isAllDigits(intPart) {
			return intPart, true, nil
		}
	}
	return s, true, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ParseBool aceita as grafias que aparecem em planilha brasileira.
func ParseBool(field, raw string) (bool, bool, *ParseWarning) {
	s := strings.ToLower(CleanText(raw))
	if IsEmpty(s) {
		return false, false, nil
	}
	switch s {
	case "1", "true", "verdadeiro", "sim", "s", "y", "yes", "x", "ativo":
		return true, true, nil
	case "0", "false", "falso", "nao", "não", "n", "no", "inativo":
		return false, true, nil
	}
	return false, false, &ParseWarning{field, raw, "valor booleano não reconhecido"}
}

// Parser identifica a função de conversão de uma coluna no perfil de
// mapeamento. É string porque o perfil é DADO (JSONB), não código.
type Parser string

const (
	ParserText    Parser = "text"
	ParserMoneyBR Parser = "money_br"
	ParserNumber  Parser = "number"
	ParserPercent Parser = "percent"
	ParserDate    Parser = "date"
	ParserCode    Parser = "code"
	ParserBool    Parser = "bool"
)

// KnownParsers é a whitelist. Perfil que pede parser desconhecido é REJEITADO
// na criação — não silenciosamente tratado como texto. Perfil é entrada de
// admin, mas admin também erra de digitação, e "parser: mony_br" tratado como
// texto colocaria a string "1.234,56" no campo de preço.
var KnownParsers = map[Parser]bool{
	ParserText: true, ParserMoneyBR: true, ParserNumber: true,
	ParserPercent: true, ParserDate: true, ParserCode: true, ParserBool: true,
}

// Apply roda o parser nomeado sobre a célula crua e devolve o valor tipado.
func (p Parser) Apply(field, raw string) (any, bool, *ParseWarning) {
	switch p {
	case ParserMoneyBR:
		v, ok, w := ParseMoney(field, raw)
		return v, ok, w
	case ParserNumber:
		v, ok, w := ParseNumber(field, raw)
		return v, ok, w
	case ParserPercent:
		v, ok, w := ParsePercent(field, raw)
		return v, ok, w
	case ParserDate:
		t, ok, w := ParseDate(field, raw)
		if !ok {
			return nil, false, w
		}
		return t.Format("2006-01-02"), true, w
	case ParserCode:
		v, ok, w := ParseCode(field, raw)
		return v, ok, w
	case ParserBool:
		v, ok, w := ParseBool(field, raw)
		return v, ok, w
	case ParserText, "":
		s := CleanText(raw)
		if IsEmpty(s) {
			return nil, false, nil
		}
		return SanitizeCell(s), true, nil
	}
	return nil, false, &ParseWarning{field, raw, fmt.Sprintf("parser %q desconhecido", p)}
}
