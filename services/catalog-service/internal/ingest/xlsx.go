package ingest

// Leitor XLSX mínimo, sem dependência externa.
//
// PORQUÊ não usar uma biblioteca pronta (excelize & cia): um arquivo enviado
// por fornecedor é ENTRADA HOSTIL. Uma lib completa de XLSX carrega parser de
// fórmula, de gráfico, de VBA, de imagem — superfície enorme pra processar um
// arquivo que só precisamos ler como grade de texto. Aqui lemos exatamente
// isto: zip → XML de planilha → células como string. É auditável em uma
// sentada e não traz árvore de dependências pro serviço.
//
// O que este leitor cobre (e um XLSX real de fornecedor usa tudo):
//   - sharedStrings (o Excel deduplica todo texto num dicionário separado)
//   - inlineStr (o que geradores server-side costumam emitir)
//   - células ausentes (o Excel omite célula vazia; a coluna não "anda")
//   - células mescladas (valor só no canto superior esquerdo)
//   - valor cacheado de fórmula
//   - datas como número serial (deixadas como número; ParseDate resolve)
//
// O que NÃO cobre, de propósito: fórmulas (lemos o valor cacheado), estilos,
// gráficos, macros. Nada disso vira produto.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Limites anti-zip-bomb. Um .xlsx de 200 KB pode expandir pra gigabytes de XML
// e derrubar o serviço por OOM — o ataque clássico contra qualquer parser de
// formato comprimido.
const (
	maxXLSXEntries    = 512               // arquivos dentro do zip
	maxXLSXEntrySize  = 128 << 20         // 128 MB descomprimidos por entrada
	maxXLSXTotalSize  = 256 << 20         // 256 MB descomprimidos no total
	maxCompressRatio  = 200               // razão de compressão suspeita
	maxXLSXSheetCells = 4 << 20           // 4M células por aba
)

type xlsxCell struct {
	Ref  string `xml:"r,attr"`
	Type string `xml:"t,attr"`
	V    string `xml:"v"`
	IS   struct {
		T string `xml:"t"`
		R []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

// ReadXLSX lê a primeira aba (ou a aba de nome `sheet`, se não vazio) e devolve
// a grade como [][]string. Toda célula vira texto — a tipagem é decisão do
// perfil de mapeamento, não do leitor. Um leitor que "adivinha" tipo é
// exatamente o comportamento do Excel que estamos tentando desfazer.
func ReadXLSX(data []byte, sheet string) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("arquivo não é um XLSX válido: %w", err)
	}
	if len(zr.File) > maxXLSXEntries {
		return nil, fmt.Errorf("XLSX com %d entradas internas excede o limite de %d", len(zr.File), maxXLSXEntries)
	}

	files := map[string]*zip.File{}
	var total uint64
	for _, f := range zr.File {
		// Zip-slip: nome de entrada com "../" só importa se extraíssemos pra
		// disco. Não extraímos — mas rejeitamos assim mesmo, porque um zip com
		// path traversal não é um arquivo de fornecedor distraído.
		if strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") {
			return nil, fmt.Errorf("XLSX com caminho interno suspeito: %q", f.Name)
		}
		if f.UncompressedSize64 > maxXLSXEntrySize {
			return nil, fmt.Errorf("entrada %q descomprimida excede o limite", f.Name)
		}
		if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > maxCompressRatio {
			return nil, fmt.Errorf("entrada %q tem razão de compressão suspeita (possível zip bomb)", f.Name)
		}
		total += f.UncompressedSize64
		if total > maxXLSXTotalSize {
			return nil, fmt.Errorf("XLSX descomprimido excede o limite total")
		}
		files[strings.TrimPrefix(f.Name, "/")] = f
	}

	shared, err := readSharedStrings(files)
	if err != nil {
		return nil, err
	}

	target, err := pickSheet(files, sheet)
	if err != nil {
		return nil, err
	}

	raw, err := readEntry(files[target])
	if err != nil {
		return nil, err
	}
	return parseSheet(raw, shared)
}

// SheetNames lista as abas. Útil na tela de upload (o SINAPI vem com várias) e
// na mensagem de erro quando a aba pedida não existe.
func SheetNames(data []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("arquivo não é um XLSX válido: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}
	wb, err := readWorkbook(files)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(wb))
	for _, s := range wb {
		names = append(names, s.name)
	}
	return names, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("entrada ausente no XLSX")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// LimitReader mesmo já tendo checado UncompressedSize64: o cabeçalho do zip
	// é controlado pelo atacante e pode mentir sobre o tamanho.
	return io.ReadAll(io.LimitReader(rc, maxXLSXEntrySize+1))
}

func readSharedStrings(files map[string]*zip.File) ([]string, error) {
	f, ok := files["xl/sharedStrings.xml"]
	if !ok {
		return nil, nil // planilha só com inlineStr/números — legítimo
	}
	raw, err := readEntry(f)
	if err != nil {
		return nil, err
	}
	var sst struct {
		SI []struct {
			T string `xml:"t"`
			// Rich text: o Excel quebra a string em runs quando parte dela tem
			// formatação diferente. Sem concatenar, "CIMENTO **CP II**" vira
			// só "CIMENTO " e o produto perde metade do nome.
			R []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, fmt.Errorf("sharedStrings.xml ilegível: %w", err)
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		if len(si.R) > 0 {
			var b strings.Builder
			for _, r := range si.R {
				b.WriteString(r.T)
			}
			out[i] = b.String()
		} else {
			out[i] = si.T
		}
	}
	return out, nil
}

type sheetRef struct{ name, target string }

func readWorkbook(files map[string]*zip.File) ([]sheetRef, error) {
	raw, err := readEntry(files["xl/workbook.xml"])
	if err != nil {
		return nil, fmt.Errorf("workbook.xml ausente: %w", err)
	}
	var wb struct {
		Sheets struct {
			Sheet []struct {
				Name string `xml:"name,attr"`
				RID  string `xml:"id,attr"`
			} `xml:"sheet"`
		} `xml:"sheets"`
	}
	if err := xml.Unmarshal(raw, &wb); err != nil {
		return nil, fmt.Errorf("workbook.xml ilegível: %w", err)
	}

	// Mapa rId → arquivo da aba. A ordem em workbook.xml é a ordem das abas na
	// interface; o caminho do XML vem do arquivo de relacionamentos.
	rels := map[string]string{}
	if relRaw, err := readEntry(files["xl/_rels/workbook.xml.rels"]); err == nil {
		var rl struct {
			R []struct {
				ID     string `xml:"Id,attr"`
				Target string `xml:"Target,attr"`
			} `xml:"Relationship"`
		}
		if err := xml.Unmarshal(relRaw, &rl); err == nil {
			for _, r := range rl.R {
				t := strings.TrimPrefix(r.Target, "/xl/")
				t = strings.TrimPrefix(t, "/")
				if !strings.HasPrefix(t, "xl/") {
					t = "xl/" + t
				}
				rels[r.ID] = t
			}
		}
	}

	out := make([]sheetRef, 0, len(wb.Sheets.Sheet))
	for i, s := range wb.Sheets.Sheet {
		target := rels[s.RID]
		if target == "" {
			// Fallback pro layout convencional quando os rels faltam.
			target = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		out = append(out, sheetRef{name: s.Name, target: target})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("XLSX sem nenhuma aba")
	}
	return out, nil
}

func pickSheet(files map[string]*zip.File, want string) (string, error) {
	sheets, err := readWorkbook(files)
	if err != nil {
		return "", err
	}
	if want == "" {
		if _, ok := files[sheets[0].target]; ok {
			return sheets[0].target, nil
		}
		return "", fmt.Errorf("aba %q declarada mas ausente do arquivo", sheets[0].name)
	}
	// Comparação sem acento e sem caixa: a aba do SINAPI aparece como
	// "Analítico", "ANALITICO" e "Analitico" conforme o mês.
	norm := func(s string) string { return strings.ToLower(stripAccents(strings.TrimSpace(s))) }
	for _, s := range sheets {
		if norm(s.name) == norm(want) {
			if _, ok := files[s.target]; !ok {
				return "", fmt.Errorf("aba %q declarada mas ausente do arquivo", s.name)
			}
			return s.target, nil
		}
	}
	names := make([]string, len(sheets))
	for i, s := range sheets {
		names[i] = s.name
	}
	return "", fmt.Errorf("aba %q não encontrada; abas disponíveis: %s", want, strings.Join(names, ", "))
}

var cellRefRe = regexp.MustCompile(`^([A-Z]+)([0-9]+)$`)

// colIndex converte "A"→0, "Z"→25, "AA"→26. Necessário porque o Excel OMITE
// células vazias: uma linha com A e D traz dois <c>, e ler sequencialmente
// colocaria o valor de D na posição de B.
func colIndex(ref string) (int, int, bool) {
	m := cellRefRe.FindStringSubmatch(ref)
	if m == nil {
		return 0, 0, false
	}
	col := 0
	for _, r := range m[1] {
		col = col*26 + int(r-'A') + 1
	}
	row, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, false
	}
	return col - 1, row - 1, true
}

func parseSheet(raw []byte, shared []string) ([][]string, error) {
	grid := map[int]map[int]string{}
	maxRow, maxCol := -1, -1
	cells := 0

	dec := xml.NewDecoder(bytes.NewReader(raw))
	// Linha corrente: usada como fallback quando a célula não traz `r`
	// (geradores minimalistas omitem a referência).
	curRow, curCol := -1, 0

	var merges []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML da planilha ilegível: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "row":
			curRow++
			curCol = 0
			for _, a := range se.Attr {
				if a.Name.Local == "r" {
					if n, err := strconv.Atoi(a.Value); err == nil {
						curRow = n - 1
					}
				}
			}
		case "c":
			var c xlsxCell
			if err := dec.DecodeElement(&c, &se); err != nil {
				return nil, fmt.Errorf("célula ilegível: %w", err)
			}
			col, row := curCol, curRow
			if ci, ri, ok := colIndex(c.Ref); ok {
				col, row = ci, ri
			}
			curCol = col + 1
			if row < 0 {
				continue
			}
			cells++
			if cells > maxXLSXSheetCells {
				return nil, fmt.Errorf("planilha excede o limite de %d células", maxXLSXSheetCells)
			}

			val := cellValue(c, shared)
			if val == "" {
				continue
			}
			if grid[row] == nil {
				grid[row] = map[int]string{}
			}
			grid[row][col] = val
			if row > maxRow {
				maxRow = row
			}
			if col > maxCol {
				maxCol = col
			}
		case "mergeCell":
			for _, a := range se.Attr {
				if a.Name.Local == "ref" {
					merges = append(merges, a.Value)
				}
			}
		}
	}

	if maxRow < 0 {
		return [][]string{}, nil
	}

	// Células mescladas: o valor mora só no canto superior esquerdo e as outras
	// posições vêm vazias. Sem propagar, um cabeçalho mesclado ("PREÇO" sobre
	// duas colunas) deixa a segunda coluna sem nome e o mapeamento a ignora.
	applyMerges(grid, merges, &maxRow, &maxCol)

	out := make([][]string, maxRow+1)
	for r := 0; r <= maxRow; r++ {
		row := make([]string, maxCol+1)
		for c := 0; c <= maxCol; c++ {
			row[c] = grid[r][c]
		}
		out[r] = row
	}
	return out, nil
}

func applyMerges(grid map[int]map[int]string, merges []string, maxRow, maxCol *int) {
	// Ordena pra que o resultado não dependa da ordem do XML (determinismo é
	// requisito: o mesmo arquivo tem que produzir o mesmo lote sempre).
	sort.Strings(merges)
	for _, ref := range merges {
		parts := strings.SplitN(ref, ":", 2)
		if len(parts) != 2 {
			continue
		}
		c1, r1, ok1 := colIndex(parts[0])
		c2, r2, ok2 := colIndex(parts[1])
		if !ok1 || !ok2 {
			continue
		}
		v := grid[r1][c1]
		if v == "" {
			continue
		}
		// Teto defensivo: "A1:XFD1048576" mescla a planilha inteira e faria
		// este laço alocar bilhões de células.
		if (r2-r1+1)*(c2-c1+1) > 100000 {
			continue
		}
		for r := r1; r <= r2; r++ {
			for c := c1; c <= c2; c++ {
				if grid[r] == nil {
					grid[r] = map[int]string{}
				}
				if grid[r][c] == "" {
					grid[r][c] = v
				}
				if r > *maxRow {
					*maxRow = r
				}
				if c > *maxCol {
					*maxCol = c
				}
			}
		}
	}
}

func cellValue(c xlsxCell, shared []string) string {
	switch c.Type {
	case "s": // índice no dicionário de strings compartilhadas
		i, err := strconv.Atoi(strings.TrimSpace(c.V))
		if err != nil || i < 0 || i >= len(shared) {
			return ""
		}
		return shared[i]
	case "inlineStr":
		if len(c.IS.R) > 0 {
			var b strings.Builder
			for _, r := range c.IS.R {
				b.WriteString(r.T)
			}
			return b.String()
		}
		return c.IS.T
	case "b":
		if strings.TrimSpace(c.V) == "1" {
			return "1"
		}
		return "0"
	case "e": // erro de fórmula: #REF!, #VALUE! — IsEmpty trata
		return c.V
	default: // "n", "str" (valor cacheado de fórmula) ou sem tipo
		return c.V
	}
}

// stripAccents — comparação de nome de aba e de coluna sem depender de acento.
// Tabela explícita em vez de normalização Unicode completa: o conjunto de
// caracteres que aparece em cabeçalho de planilha brasileira é pequeno e
// conhecido, e assim não entra dependência de golang.org/x/text.
func stripAccents(s string) string {
	return strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "ë", "e",
		"í", "i", "î", "i", "ì", "i", "ï", "i",
		"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o",
		"ú", "u", "û", "u", "ù", "u", "ü", "u",
		"ç", "c", "ñ", "n",
		"Á", "A", "À", "A", "Ã", "A", "Â", "A", "Ä", "A",
		"É", "E", "Ê", "E", "È", "E", "Ë", "E",
		"Í", "I", "Î", "I", "Ì", "I", "Ï", "I",
		"Ó", "O", "Ô", "O", "Õ", "O", "Ò", "O", "Ö", "O",
		"Ú", "U", "Û", "U", "Ù", "U", "Ü", "U",
		"Ç", "C", "Ñ", "N",
	).Replace(s)
}
