package ingest

// Leitura multi-formato: CSV, XLSX e JSON → a MESMA estrutura intermediária.
//
// A estrutura é `Table`: um cabeçalho (nomes de coluna) + linhas de texto. Todo
// o resto do pipeline (mapeamento, validação, dry-run, commit) enxerga só isso
// e não sabe de que formato veio. É o que permite acrescentar um formato novo
// sem tocar em nenhuma regra de negócio.

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// Limites de arquivo. Entrada de fornecedor é entrada hostil: sem teto, um
// upload derruba o serviço por memória antes de qualquer validação de negócio.
const (
	MaxFileBytes = 32 << 20 // 32 MB — cobre planilha de ~200k linhas
	MaxRows      = 50000    // acima disso o fluxo certo é integração, não upload
	MaxColumns   = 200
	MaxCellBytes = 8192 // célula maior que isto é lixo ou payload
)

// Table é a grade normalizada. `Header` já vem limpo (sem NBSP, sem espaço
// duplo); `Rows` preserva o texto das células como veio, exceto pela limpeza
// de caracteres invisíveis — o valor cru fica no staging (import_rows.raw) pra
// auditoria.
type Table struct {
	Header  []string
	Rows    [][]string
	// RowNumbers guarda a linha ORIGINAL no arquivo de cada entrada de Rows.
	// Sem isto, pular preâmbulo faz o relatório de erro apontar "linha 5"
	// quando o operador precisa procurar na linha 12 da planilha dele.
	RowNumbers []int
	Format     string
	Sheet      string
}

// Format detecta o formato pelo CONTEÚDO, não pela extensão. Extensão é
// palpite do usuário: ".xlsx" renomeado de .csv chega toda semana, e um .csv
// que na verdade é XLSX faria o parser de CSV produzir lixo binário como nome
// de produto em vez de um erro claro.
func DetectFormat(filename string, data []byte) string {
	switch {
	case len(data) >= 4 && bytes.Equal(data[:2], []byte("PK")):
		return "xlsx" // zip — todo OOXML é zip
	case looksLikeJSON(data):
		return "json"
	default:
		return "csv"
	}
}

func looksLikeJSON(data []byte) bool {
	t := bytes.TrimLeft(data, " \t\r\n\uFEFF")
	return len(t) > 0 && (t[0] == '[' || t[0] == '{')
}

// Read converte o arquivo na Table normalizada.
//
// `headerHint` permite pular preâmbulo: quando > 0, é o número da linha (1-based
// no arquivo) que contém o cabeçalho. Quando 0, o cabeçalho é DETECTADO — a
// planilha oficial do SINAPI tem várias linhas de título e nota de rodapé
// antes do cabeçalho real, e o número dessas linhas muda entre meses.
func Read(filename string, data []byte, sheet string, headerHint int) (*Table, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("arquivo vazio")
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("arquivo de %d bytes excede o limite de %d", len(data), MaxFileBytes)
	}

	format := DetectFormat(filename, data)
	switch format {
	case "xlsx":
		grid, err := ReadXLSX(data, sheet)
		if err != nil {
			return nil, err
		}
		t, err := fromGrid(grid, headerHint)
		if err != nil {
			return nil, err
		}
		t.Format, t.Sheet = "xlsx", sheet
		return t, nil

	case "json":
		t, err := fromJSON(data)
		if err != nil {
			return nil, err
		}
		t.Format = "json"
		return t, nil

	default:
		grid, err := readCSVGrid(data)
		if err != nil {
			return nil, err
		}
		t, err := fromGrid(grid, headerHint)
		if err != nil {
			return nil, err
		}
		t.Format = "csv"
		return t, nil
	}
}

func readCSVGrid(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // BOM UTF-8

	// Latin-1: planilha exportada do Excel brasileiro sem escolher UTF-8 vem em
	// windows-1252, e "CIMENTO PORTLAND ARGAMASSA" quebra em "Ã‡". Sem esta
	// conversão o nome do produto chega corrompido ao banco — e corrigir depois
	// exige reimportar tudo.
	if !utf8.Valid(data) {
		buf := make([]rune, 0, len(data))
		for _, b := range data {
			buf = append(buf, rune(b))
		}
		data = []byte(string(buf))
	}

	delim := detectDelimiter(data)
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = delim
	cr.FieldsPerRecord = -1 // linha com contagem diferente NÃO aborta o arquivo
	cr.LazyQuotes = true    // aspas soltas no meio do texto são comuns e não são erro fatal
	cr.TrimLeadingSpace = true

	var grid [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Linha malformada vira linha vazia em vez de abortar: a regra do
			// "linha inválida não aborta o lote" começa aqui, na leitura.
			// A validação por linha reporta o problema com o número da linha.
			grid = append(grid, []string{})
			continue
		}
		if len(rec) > MaxColumns {
			return nil, fmt.Errorf("arquivo com %d colunas excede o limite de %d", len(rec), MaxColumns)
		}
		grid = append(grid, rec)
		if len(grid) > MaxRows+50 { // +50 de folga pro preâmbulo
			return nil, fmt.Errorf("arquivo excede o limite de %d linhas", MaxRows)
		}
	}
	return grid, nil
}

// detectDelimiter — ";" é o padrão do Excel brasileiro (porque a vírgula já é
// o separador decimal). Escolhemos pela linha que tem MAIS ocorrências
// consistentes, não pela primeira: a primeira linha costuma ser título.
func detectDelimiter(data []byte) rune {
	sample := data
	if len(sample) > 64<<10 {
		sample = sample[:64<<10]
	}
	lines := strings.SplitN(string(sample), "\n", 30)
	best, bestScore := ',', 0
	for _, d := range []rune{';', ',', '\t', '|'} {
		score := 0
		for _, l := range lines {
			score += strings.Count(l, string(d))
		}
		if score > bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

// fromGrid separa cabeçalho de dados, pulando preâmbulo.
func fromGrid(grid [][]string, headerHint int) (*Table, error) {
	if len(grid) == 0 {
		return nil, fmt.Errorf("arquivo sem linhas")
	}

	hdrIdx := headerHint - 1 // hint é 1-based
	if headerHint <= 0 {
		hdrIdx = detectHeaderRow(grid)
	}
	if hdrIdx < 0 || hdrIdx >= len(grid) {
		return nil, fmt.Errorf("não foi possível localizar a linha de cabeçalho")
	}

	header := make([]string, len(grid[hdrIdx]))
	for i, h := range grid[hdrIdx] {
		header[i] = CleanText(h)
	}
	header = dedupeHeader(header)

	t := &Table{Header: header}
	for i := hdrIdx + 1; i < len(grid); i++ {
		row := grid[i]
		if isBlankRow(row) {
			continue // linha em branco no meio da planilha é formatação, não dado
		}
		cells := make([]string, len(header))
		for j := range header {
			if j < len(row) {
				c := CleanText(row[j])
				if len(c) > MaxCellBytes {
					c = c[:MaxCellBytes]
				}
				cells[j] = c
			}
		}
		t.Rows = append(t.Rows, cells)
		t.RowNumbers = append(t.RowNumbers, i+1) // 1-based, como o operador vê
		if len(t.Rows) > MaxRows {
			return nil, fmt.Errorf("arquivo excede o limite de %d linhas de dados", MaxRows)
		}
	}
	return t, nil
}

// detectHeaderRow acha o cabeçalho real numa planilha com preâmbulo.
//
// Heurística: a linha de cabeçalho é a que tem MAIS células não-vazias e
// distintas, majoritariamente TEXTO (não número), dentro das primeiras 30
// linhas — e que é seguida por pelo menos uma linha com contagem de células
// semelhante. Esse último critério é o que separa "cabeçalho" de "título
// mesclado que ocupa a linha inteira".
//
// PORQUÊ heurística e não posição fixa: a planilha do SINAPI muda o número de
// linhas de preâmbulo entre publicações mensais. Codificar "o cabeçalho é a
// linha 10" garante que a importação quebra em algum mês, provavelmente sem
// erro — apenas importando as notas de rodapé como se fossem produtos.
func detectHeaderRow(grid [][]string) int {
	limit := len(grid)
	if limit > 30 {
		limit = 30
	}
	best, bestScore := 0, -1
	for i := 0; i < limit; i++ {
		row := grid[i]
		filled, textual := 0, 0
		seen := map[string]bool{}
		for _, c := range row {
			c = CleanText(c)
			if c == "" {
				continue
			}
			filled++
			if seen[strings.ToLower(c)] {
				continue // colunas repetidas indicam linha mesclada, não cabeçalho
			}
			seen[strings.ToLower(c)] = true
			if _, ok, _ := ParseNumber("", c); !ok {
				textual++
			}
		}
		if filled < 2 || textual*2 < filled {
			continue // linha de dados numéricos ou título isolado
		}
		// A linha seguinte precisa parecer dado com largura compatível.
		next := 0
		if i+1 < len(grid) {
			for _, c := range grid[i+1] {
				if CleanText(c) != "" {
					next++
				}
			}
		}
		if next < 2 {
			continue
		}
		score := len(seen)*10 + min(next, filled)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if bestScore < 0 {
		return 0
	}
	return best
}

// dedupeHeader resolve colunas com o mesmo nome. Planilha real tem duas
// colunas "PREÇO" (uma de custo, outra de venda) e uma coluna sem nome.
// Sem desambiguar, o mapa nome→índice perde uma delas silenciosamente.
func dedupeHeader(h []string) []string {
	seen := map[string]int{}
	out := make([]string, len(h))
	for i, name := range h {
		if name == "" {
			name = fmt.Sprintf("coluna_%d", i+1)
		}
		key := strings.ToLower(name)
		if n, ok := seen[key]; ok {
			seen[key] = n + 1
			name = fmt.Sprintf("%s (%d)", name, n+1)
		} else {
			seen[key] = 1
		}
		out[i] = name
	}
	return out
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if CleanText(c) != "" {
			return false
		}
	}
	return true
}

// fromJSON aceita array de objetos (`[{...},{...}]`) e o envelope
// `{"products":[...]}` / `{"data":[...]}` / `{"items":[...]}`, que é como API
// de fornecedor costuma responder.
//
// As chaves do PRIMEIRO objeto definem a ordem das colunas; chaves que só
// aparecem em objetos posteriores são acrescentadas ao final, em ordem
// alfabética (determinismo: o mesmo arquivo tem que gerar as mesmas colunas
// sempre, e a iteração de map em Go é aleatória por design).
func fromJSON(data []byte) (*Table, error) {
	var items []map[string]any

	if err := json.Unmarshal(data, &items); err != nil {
		var env map[string]json.RawMessage
		if err2 := json.Unmarshal(data, &env); err2 != nil {
			return nil, fmt.Errorf("JSON inválido: %w", err)
		}
		found := false
		for _, k := range []string{"products", "data", "items", "rows", "produtos"} {
			if raw, ok := env[k]; ok {
				if err := json.Unmarshal(raw, &items); err == nil {
					found = true
					break
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("JSON deve ser um array de objetos ou um envelope com 'products'/'data'/'items'")
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("JSON sem nenhum registro")
	}
	if len(items) > MaxRows {
		return nil, fmt.Errorf("JSON com %d registros excede o limite de %d", len(items), MaxRows)
	}

	var header []string
	inHeader := map[string]bool{}
	for k := range items[0] {
		header = append(header, k)
		inHeader[k] = true
	}
	sort.Strings(header)

	var extra []string
	for _, it := range items[1:] {
		for k := range it {
			if !inHeader[k] {
				inHeader[k] = true
				extra = append(extra, k)
			}
		}
	}
	sort.Strings(extra)
	header = append(header, extra...)

	if len(header) > MaxColumns {
		return nil, fmt.Errorf("JSON com %d campos excede o limite de %d", len(header), MaxColumns)
	}

	t := &Table{Header: header}
	for i, it := range items {
		row := make([]string, len(header))
		for j, k := range header {
			row[j] = jsonScalar(it[k])
			if len(row[j]) > MaxCellBytes {
				row[j] = row[j][:MaxCellBytes]
			}
		}
		t.Rows = append(t.Rows, row)
		t.RowNumbers = append(t.RowNumbers, i+1)
	}
	return t, nil
}

// jsonScalar reduz o valor JSON a texto — a mesma forma que CSV e XLSX
// entregam, pra que o resto do pipeline não precise saber do formato.
//
// Número vira texto SEM notação científica e sem perder casas: `%v` num
// float64 grande produz "1.23456789e+12", o mesmo estrago que o Excel faz com
// código de barras. `strconv.FormatFloat(_, 'f', -1, 64)` preserva os dígitos.
func jsonScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return CleanText(x)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case float64:
		return trimFloat(x)
	case json.Number:
		return x.String()
	default:
		// Objeto/array aninhado (ex.: specs). Preservado como JSON pra que o
		// mapeamento possa jogá-lo direto em `specs`.
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.10f", f)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
