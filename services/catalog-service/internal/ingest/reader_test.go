package ingest

// Testes de leitura multi-formato: CSV, XLSX e JSON convergem para a MESMA
// Table. É essa convergência que faz as regras de negócio valerem igual nos
// três formatos.

import (
	"strings"
	"testing"
)

func TestDetectFormat_PeloConteudoNaoPelaExtensao(t *testing.T) {
	// ".xlsx" renomeado de .csv chega toda semana. Confiar na extensão faria o
	// parser errado produzir lixo binário como nome de produto.
	xlsx, err := WriteXLSX([]XLSXSheet{{Name: "S", Rows: [][]string{{"a"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := DetectFormat("planilha.csv", xlsx); got != "xlsx" {
		t.Errorf("XLSX com nome .csv detectado como %q", got)
	}
	if got := DetectFormat("dados.xlsx", []byte("sku,name\nA,B\n")); got != "csv" {
		t.Errorf("CSV com nome .xlsx detectado como %q", got)
	}
	if got := DetectFormat("x.txt", []byte(`[{"sku":"A"}]`)); got != "json" {
		t.Errorf("JSON detectado como %q", got)
	}
}

func TestReadCSV_DelimitadorPontoEVirgula(t *testing.T) {
	// ";" é o padrão do Excel brasileiro, porque a vírgula já é decimal.
	csv := "SKU;NOME;PRECO\nP-1;Parafuso;1.234,56\nP-2;Prego;9,90\n"
	tbl, err := Read("x.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Header) != 3 {
		t.Fatalf("cabeçalho = %v", tbl.Header)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("linhas = %d", len(tbl.Rows))
	}
	if tbl.Rows[0][2] != "1.234,56" {
		t.Errorf("célula = %q — o valor cru deve ser preservado", tbl.Rows[0][2])
	}
}

func TestReadCSV_Latin1NaoCorrompeAcento(t *testing.T) {
	// Excel brasileiro exporta em windows-1252 quando ninguém escolhe UTF-8.
	// Sem a conversão, "CONSTRUÇÃO" chega como "CONSTRU├З├ГO" no banco e
	// corrigir depois exige reimportar tudo.
	latin1 := []byte{
		'S', 'K', 'U', ',', 'N', 'O', 'M', 'E', '\n',
		'P', '1', ',', 'C', 'O', 'N', 'S', 'T', 'R', 'U', 0xC7, 0xC3, 'O', '\n',
	}
	tbl, err := Read("x.csv", latin1, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tbl.Rows[0][1], "Ç") {
		t.Errorf("acento latin-1 não convertido: %q", tbl.Rows[0][1])
	}
}

func TestReadCSV_LinhaMalformadaNaoAbortaOArquivo(t *testing.T) {
	// A regra "linha inválida não aborta o lote" começa aqui, na leitura.
	csv := "SKU,NOME,PRECO\nP-1,Parafuso,1\n\"aspas ruins,,,\nP-3,Prego,3\n"
	tbl, err := Read("x.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatalf("linha malformada não deveria abortar a leitura: %v", err)
	}
	if len(tbl.Rows) < 2 {
		t.Errorf("linhas boas deveriam sobreviver; veio %d", len(tbl.Rows))
	}
}

func TestReadCSV_CabecalhoDuplicadoEDesambiguado(t *testing.T) {
	// Planilha real tem duas colunas "PRECO" (custo e venda). Sem desambiguar,
	// o mapa nome→índice perde uma delas em silêncio.
	csv := "SKU,PRECO,PRECO\nP-1,10,20\n"
	tbl, err := Read("x.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Header[1] == tbl.Header[2] {
		t.Errorf("colunas duplicadas não desambiguadas: %v", tbl.Header)
	}
}

func TestReadCSV_NumeroDaLinhaApontaOArquivoOriginal(t *testing.T) {
	// O operador precisa achar o erro na planilha DELE.
	csv := "SKU,NOME\nP-1,A\nP-2,B\n"
	tbl, _ := Read("x.csv", []byte(csv), "", 0)
	if tbl.RowNumbers[0] != 2 {
		t.Errorf("1ª linha de dados = %d, quer 2 (linha 1 é o cabeçalho)", tbl.RowNumbers[0])
	}
	if tbl.RowNumbers[1] != 3 {
		t.Errorf("2ª linha de dados = %d, quer 3", tbl.RowNumbers[1])
	}
}

func TestReadJSON_ArrayEEnvelope(t *testing.T) {
	arr := `[{"sku":"P-1","name":"Parafuso","price":1.5},{"sku":"P-2","name":"Prego","price":2}]`
	tbl, err := Read("x.json", []byte(arr), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("linhas = %d", len(tbl.Rows))
	}

	env := `{"products":` + arr + `}`
	tbl2, err := Read("x.json", []byte(env), "", 0)
	if err != nil {
		t.Fatalf("envelope não reconhecido: %v", err)
	}
	if len(tbl2.Rows) != 2 {
		t.Errorf("envelope: linhas = %d", len(tbl2.Rows))
	}
}

func TestReadJSON_ColunasDeterministicas(t *testing.T) {
	// Iteração de map em Go é aleatória: sem ordenação, as colunas mudariam a
	// cada leitura e o mesmo arquivo geraria lotes diferentes.
	js := `[{"sku":"A","name":"N","price":1,"brand":"B","stock":5}]`
	first, err := Read("x.json", []byte(js), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, _ := Read("x.json", []byte(js), "", 0)
		for j := range first.Header {
			if got.Header[j] != first.Header[j] {
				t.Fatalf("ordem de colunas não-determinística: %v vs %v", first.Header, got.Header)
			}
		}
	}
}

func TestReadJSON_NumeroGrandeNaoViraNotacaoCientifica(t *testing.T) {
	// O mesmo estrago que o Excel faz com código de barras, agora vindo de API.
	js := `[{"sku":"A","name":"N","barcode":7891234567890}]`
	tbl, err := Read("x.json", []byte(js), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	idx := -1
	for i, h := range tbl.Header {
		if h == "barcode" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("coluna barcode ausente")
	}
	got := tbl.Rows[0][idx]
	if strings.ContainsAny(got, "eE") {
		t.Errorf("código virou notação científica: %q", got)
	}
	if got != "7891234567890" {
		t.Errorf("código = %q, quer 7891234567890", got)
	}
}

func TestReadXLSX_CelulasAusentesNaoDeslocamColunas(t *testing.T) {
	// O Excel OMITE célula vazia. Ler sequencialmente colocaria o valor de D na
	// posição de B — e o preço iria pro campo errado.
	rows := [][]string{
		{"SKU", "NOME", "MARCA", "PRECO"},
		{"P-1", "Parafuso", "", "10.50"}, // MARCA vazia
		{"P-2", "Prego", "Gerdau", "9.90"},
	}
	data, err := WriteXLSX([]XLSXSheet{{Name: "Dados", Rows: rows}})
	if err != nil {
		t.Fatal(err)
	}
	tbl, err := Read("x.xlsx", data, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("linhas = %d", len(tbl.Rows))
	}
	if tbl.Rows[0][3] != "10.50" {
		t.Errorf("preço da linha 1 = %q — coluna deslocada pela célula vazia", tbl.Rows[0][3])
	}
	if tbl.Rows[1][2] != "Gerdau" {
		t.Errorf("marca da linha 2 = %q", tbl.Rows[1][2])
	}
}

func TestReadXLSX_CelulasMescladasPropagamValor(t *testing.T) {
	// Cabeçalho mesclado deixaria a 2ª coluna sem nome, e o mapeamento a
	// ignoraria em silêncio.
	rows := [][]string{
		{"RELATORIO DE PRECOS"},
		{"SKU", "NOME", "PRECO"},
		{"P-1", "Parafuso", "1,50"},
	}
	data, err := WriteXLSX([]XLSXSheet{{Name: "S", Rows: rows, Merges: []string{"A1:C1"}}})
	if err != nil {
		t.Fatal(err)
	}
	grid, err := ReadXLSX(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if grid[0][1] != "RELATORIO DE PRECOS" {
		t.Errorf("célula mesclada não propagada: %q", grid[0][1])
	}
}

func TestReadXLSX_EscolheAbaPorNomeSemAcento(t *testing.T) {
	// A aba do SINAPI aparece como "Analítico", "ANALITICO" e "Analitico"
	// conforme o mês.
	data, err := WriteXLSX([]XLSXSheet{
		{Name: "ISD", Rows: [][]string{{"a"}}},
		{Name: "Analítico", Rows: [][]string{{"b"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	grid, err := ReadXLSX(data, "ANALITICO")
	if err != nil {
		t.Fatalf("aba não encontrada sem acento: %v", err)
	}
	if grid[0][0] != "b" {
		t.Errorf("aba errada: %v", grid)
	}
}

func TestReadXLSX_AbaInexistenteListaAsDisponiveis(t *testing.T) {
	data, _ := WriteXLSX([]XLSXSheet{{Name: "ISD", Rows: [][]string{{"a"}}}})
	_, err := ReadXLSX(data, "NAOEXISTE")
	if err == nil {
		t.Fatal("aba inexistente deveria falhar")
	}
	if !strings.Contains(err.Error(), "ISD") {
		t.Errorf("erro deveria listar as abas disponíveis: %v", err)
	}
}

func TestSheetNames(t *testing.T) {
	data, _ := WriteXLSX([]XLSXSheet{
		{Name: "ISD", Rows: [][]string{{"a"}}},
		{Name: "CSD", Rows: [][]string{{"b"}}},
		{Name: "Analítico", Rows: [][]string{{"c"}}},
	})
	names, err := SheetNames(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "ISD" || names[2] != "Analítico" {
		t.Errorf("abas = %v", names)
	}
}

func TestRead_DetectaCabecalhoComPreambulo(t *testing.T) {
	csv := "RELATORIO DE PRECOS\n\nFornecedor: ACME\n\nSKU,NOME,PRECO\nP-1,Parafuso,1\nP-2,Prego,2\n"
	tbl, err := Read("x.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Header[0] != "SKU" {
		t.Errorf("cabeçalho detectado = %v, quer [SKU NOME PRECO]", tbl.Header)
	}
	if len(tbl.Rows) != 2 {
		t.Errorf("linhas = %d, quer 2 (preâmbulo não é dado)", len(tbl.Rows))
	}
}

func TestRead_HeaderHintExplicitoVenceADeteccao(t *testing.T) {
	csv := "A,B,C\nSKU,NOME,PRECO\nP-1,Parafuso,1\n"
	tbl, err := Read("x.csv", []byte(csv), "", 2) // cabeçalho na linha 2
	if err != nil {
		t.Fatal(err)
	}
	if tbl.Header[0] != "SKU" {
		t.Errorf("cabeçalho = %v", tbl.Header)
	}
}

// Os três formatos têm que produzir a MESMA Table — é o que garante que as
// regras de negócio valem igual em CSV, XLSX e JSON.
func TestFormatosConvergemParaAMesmaTable(t *testing.T) {
	csv := "sku,name,price\nP-1,Parafuso,10.5\n"
	js := `[{"sku":"P-1","name":"Parafuso","price":10.5}]`
	xlsxData, err := WriteXLSX([]XLSXSheet{{Name: "S", Rows: [][]string{
		{"sku", "name", "price"}, {"P-1", "Parafuso", "10.5"},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	prof := &Profile{Name: "t", Columns: map[string]ColumnMapping{
		"sku":   {Field: FieldSKU},
		"name":  {Field: FieldName},
		"price": {Field: FieldPrice},
	}, Defaults: map[Field]string{FieldCategory: "fixacao"}}

	var results []map[string]any
	for _, in := range []struct {
		name string
		data []byte
	}{{"csv", []byte(csv)}, {"json", []byte(js)}, {"xlsx", xlsxData}} {
		tbl, err := Read("x."+in.name, in.data, "", 0)
		if err != nil {
			t.Fatalf("%s: %v", in.name, err)
		}
		plan, err := (&Planner{Profile: prof, Catalog: newFakeCatalog()}).Plan(tbl)
		if err != nil {
			t.Fatalf("%s: %v", in.name, err)
		}
		if len(plan.Rows) != 1 {
			t.Fatalf("%s: linhas = %d", in.name, len(plan.Rows))
		}
		results = append(results, plan.Rows[0].Mapped)
	}

	for i := 1; i < len(results); i++ {
		for k, v := range results[0] {
			if results[i][k] != v {
				t.Errorf("formatos divergem no campo %q: %v vs %v", k, v, results[i][k])
			}
		}
	}
}
