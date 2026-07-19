package ingest

// Testes do motor de parsing brasileiro.
//
// Cobre TODOS os casos listados em docs/ingestao-de-produtos.md, seção "O
// parsing brasileiro é onde se perde dinheiro". Cada caso do documento tem um
// caso de teste nomeado aqui — se um deles for removido do código, o teste
// aponta qual regra do documento deixou de valer.

import (
	"math"
	"testing"
)

func TestParseNumber_FormatoBrasileiro(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    float64
		wantOK  bool
		wantWarn bool
	}{
		// --- o caso central: BR vs US -------------------------------------
		{"BR com milhar e decimal", "1.234,56", 1234.56, true, false},
		{"US com milhar e decimal", "1,234.56", 1234.56, true, false},
		{"BR só decimal", "1234,56", 1234.56, true, false},
		{"canônico", "1234.56", 1234.56, true, false},
		{"inteiro", "1234", 1234, true, false},

		// --- ponto de milhar sem decimal ----------------------------------
		// "11.000" virando 11,0 é o que transforma 11 mil parafusos em 11.
		{"milhar BR sem decimal", "11.000", 11000, true, false},
		{"milhar duplo", "1.234.567", 1234567, true, false},
		{"decimal de verdade", "11.5", 11.5, true, false},

		// --- vírgula de milhar americana ----------------------------------
		{"milhar US duplo", "1,234,567", 1234567, true, false},

		// --- o caso genuinamente ambíguo (deve AVISAR) ---------------------
		{"ambíguo 1,234", "1,234", 1.234, true, true},

		// --- moeda e espaço -----------------------------------------------
		{"com R$", "R$ 1.234,56", 1234.56, true, false},
		{"com R$ colado", "R$1.234,56", 1234.56, true, false},
		{"espaço não-quebrável", "1.234,56 ", 1234.56, true, false},
		{"NBSP no meio", "R$ 1.234,56", 1234.56, true, false},

		// --- negativo e contábil ------------------------------------------
		{"negativo", "-42,90", -42.90, true, false},
		{"contábil", "(1.234,56)", -1234.56, true, false},

		// --- notação científica (Excel destruindo código) ------------------
		{"científica", "7.89123E+12", 7.89123e12, true, true},
		{"científica BR", "7,89123E+12", 7.89123e12, true, true},

		// --- sentinelas de vazio ------------------------------------------
		{"vazio", "", 0, false, false},
		{"traço", "-", 0, false, false},
		{"travessão", "—", 0, false, false},
		{"N/A", "N/A", 0, false, false},
		{"REF!", "#REF!", 0, false, false},
		{"DIV/0", "#DIV/0!", 0, false, false},

		// --- zero é valor, não ausência -----------------------------------
		{"zero", "0", 0, true, false},
		{"zero decimal", "0,00", 0, true, false},

		// --- lixo ----------------------------------------------------------
		{"texto", "consultar", 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, warn := ParseNumber("campo", tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ParseNumber(%q) ok = %v, quer %v (aviso: %v)", tt.in, ok, tt.wantOK, warn)
			}
			if ok && math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("ParseNumber(%q) = %v, quer %v", tt.in, got, tt.want)
			}
			if (warn != nil) != tt.wantWarn {
				t.Errorf("ParseNumber(%q) aviso = %v, queria aviso? %v", tt.in, warn, tt.wantWarn)
			}
		})
	}
}

// O erro de vírgula é o modo de falha mais caro do catálogo. Este teste trava a
// regra que o evita: "1.234,56" NUNCA pode virar 1,23.
func TestParseMoney_NaoConfundeMilharComDecimal(t *testing.T) {
	v, ok, _ := ParseMoney("price", "1.234,56")
	if !ok {
		t.Fatal("não parseou")
	}
	if v < 1000 {
		t.Fatalf("REGRESSÃO CRÍTICA: R$ 1.234,56 virou R$ %.2f — é o erro de vírgula que "+
			"coloca cimento a R$ 1,23 na vitrine", v)
	}
	if math.Abs(v-1234.56) > 0.005 {
		t.Errorf("= %v, quer 1234.56", v)
	}
}

func TestParseMoney_RejeitaNegativo(t *testing.T) {
	if _, ok, w := ParseMoney("price", "-10,00"); ok || w == nil {
		t.Error("dinheiro negativo deveria ser rejeitado com aviso")
	}
}

func TestParseMoney_ArredondaParaCentavos(t *testing.T) {
	// Sem arredondar aqui, o valor comparado no dry-run difere do gravado no
	// NUMERIC(12,2) e o lote reporta "atualizado" eternamente pro mesmo arquivo.
	v, _, _ := ParseMoney("price", "10,999")
	if v != 11.00 {
		t.Errorf("= %v, quer 11.00 (arredondamento de centavo)", v)
	}
}

func TestParsePercent(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"10%", 0.10},
		{"10", 0.10},
		{"0,1", 0.1},
		{"7,5%", 0.075},
		{"100%", 1.0},
	}
	for _, tt := range tests {
		got, ok, _ := ParsePercent("desconto", tt.in)
		if !ok || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("ParsePercent(%q) = %v (ok=%v), quer %v", tt.in, got, ok, tt.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	// dd/mm/aaaa é o formato do país: "03/04/2026" é 3 de ABRIL.
	// Ler como mm/dd inverteria um terço das datas do ano sem erro nenhum.
	d, ok, _ := ParseDate("validade", "03/04/2026")
	if !ok {
		t.Fatal("não parseou data BR")
	}
	if d.Day() != 3 || d.Month() != 4 {
		t.Errorf("03/04/2026 lido como %v — deveria ser 3 de abril (dd/mm)", d.Format("2006-01-02"))
	}

	// ISO é inequívoco.
	if d, ok, _ := ParseDate("v", "2026-04-03"); !ok || d.Month() != 4 || d.Day() != 3 {
		t.Errorf("ISO mal parseado: %v", d)
	}

	// Serial do Excel — a célula foi formatada como data por engano.
	d, ok, warn := ParseDate("v", "45000")
	if !ok {
		t.Fatal("serial do Excel não reconhecido")
	}
	if warn == nil {
		t.Error("serial do Excel deveria emitir aviso")
	}
	if d.Year() < 2020 || d.Year() > 2030 {
		t.Errorf("serial 45000 → %v, esperado ~2023", d.Format("2006-01-02"))
	}
}

// ParseCode protege a IDENTIDADE do produto. Um EAN destruído pelo Excel não
// pode virar SKU silenciosamente.
func TestParseCode(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		want     string
		wantWarn bool
	}{
		{"código normal", "ABC-123", "ABC-123", false},
		{"EAN íntegro", "7891234567890", "7891234567890", false},
		{"decimal fantasma", "123456.0", "123456", false},
		{"decimal fantasma BR", "123456,00", "123456", false},
		{"científica (Excel destruiu)", "7.89123E+12", "7891230000000", true},
		{"número decimal real não é código truncado", "12.5", "12.5", false},
		{"espaços internos", " 789 123 ", "789123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, warn := ParseCode("sku", tt.in)
			if !ok {
				t.Fatalf("ParseCode(%q) não retornou valor", tt.in)
			}
			if got != tt.want {
				t.Errorf("ParseCode(%q) = %q, quer %q", tt.in, got, tt.want)
			}
			if (warn != nil) != tt.wantWarn {
				t.Errorf("ParseCode(%q) aviso = %v, queria? %v", tt.in, warn, tt.wantWarn)
			}
		})
	}
}

func TestParseCode_CientificaAvisaSobrePerdaDeDigito(t *testing.T) {
	// O aviso importa mais que o valor: o Excel PERDE dígitos ao converter
	// GTIN-14 pra float, e o operador precisa saber pra pedir CSV.
	_, _, warn := ParseCode("barcode", "7.89123E+12")
	if warn == nil {
		t.Fatal("código em notação científica DEVE avisar sobre possível perda de dígitos")
	}
}

func TestCleanText_EspacoNaoQuebravel(t *testing.T) {
	// O NBSP é invisível e faz "CIMENTO " != "CIMENTO", criando produto
	// duplicado a cada importação sem que ninguém veja o motivo.
	if got := CleanText("CIMENTO "); got != "CIMENTO" {
		t.Errorf("NBSP não removido: %q", got)
	}
	if got := CleanText("CIMENTO   CP  II"); got != "CIMENTO CP II" {
		t.Errorf("espaços internos não colapsados: %q", got)
	}
	if got := CleanText("\uFEFFCIMENTO"); got != "CIMENTO" {
		t.Errorf("BOM não removido: %q", got)
	}
	if CleanText("A B") != CleanText("A B") {
		t.Error("NBSP e espaço normal deveriam normalizar igual")
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"1", "sim", "SIM", "true", "Verdadeiro", "x", "ativo"} {
		if v, ok, _ := ParseBool("f", s); !ok || !v {
			t.Errorf("ParseBool(%q) deveria ser true", s)
		}
	}
	for _, s := range []string{"0", "nao", "não", "false", "Falso", "inativo"} {
		if v, ok, _ := ParseBool("f", s); !ok || v {
			t.Errorf("ParseBool(%q) deveria ser false", s)
		}
	}
	if _, ok, w := ParseBool("f", "talvez"); ok || w == nil {
		t.Error("valor não-booleano deveria falhar com aviso")
	}
}

func TestParser_Apply_WhitelistDeParser(t *testing.T) {
	// Parser desconhecido não pode cair silenciosamente em "texto": isso
	// colocaria a string "1.234,56" no campo de preço.
	if _, ok, w := Parser("mony_br").Apply("price", "1.234,56"); ok || w == nil {
		t.Error("parser desconhecido deveria falhar com aviso, não virar texto")
	}
}

func TestParser_Apply_TextoSaneiaFormula(t *testing.T) {
	v, ok, _ := ParserText.Apply("name", "=cmd|'/c calc'!A1")
	if !ok {
		t.Fatal("texto deveria ser aceito")
	}
	if s, _ := v.(string); s[0] != '\'' {
		t.Errorf("fórmula não saneada pelo parser de texto: %q", s)
	}
}
