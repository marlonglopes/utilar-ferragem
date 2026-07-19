package ingest

// Escritor XLSX mínimo.
//
// PORQUÊ existe: pra TESTAR o leitor precisamos de arquivos .xlsx de verdade —
// com zip, sharedStrings, células mescladas e linhas de preâmbulo. Commitar
// binários de fixture no repositório é pior: ninguém consegue revisar num diff
// o que mudou dentro de um zip, e "o teste quebrou" vira arqueologia.
// Gerando o arquivo em código, o layout que estamos assumindo fica LEGÍVEL na
// revisão — que é exatamente o ponto quando o layout é o de um arquivo oficial
// que não conseguimos baixar.
//
// Também é o que gera a amostra do SINAPI em scripts/ingestao/.
//
// Deliberadamente sem estilos, sem fórmulas, sem tipos: escreve tudo como
// inlineStr (texto) ou número. É o suficiente pro leitor exercitar seus
// caminhos, e o mínimo de código pra manter.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// XLSXSheet é uma aba a escrever. `Merges` recebe referências no formato
// "A1:D1" pra exercitar o caminho de células mescladas do leitor.
type XLSXSheet struct {
	Name   string
	Rows   [][]string
	Merges []string
}

// WriteXLSX monta um .xlsx em memória.
//
// Uma célula é escrita como NÚMERO quando o texto é um número em formato
// canônico (ponto decimal, sem separador de milhar) — porque é assim que o
// Excel real grava, e o leitor precisa ser testado contra isso. Todo o resto
// vira inlineStr, preservando exatamente o texto, inclusive "1.234,56" e
// "7,89123E+12", que é como a célula de texto de uma planilha real se parece.
func WriteXLSX(sheets []XLSXSheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("nenhuma aba a escrever")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}

	var types, wbSheets, wbRels strings.Builder
	types.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)

	for i, s := range sheets {
		n := i + 1
		types.WriteString(fmt.Sprintf(
			`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, n))
		wbSheets.WriteString(fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEsc(s.Name), n, n))
		wbRels.WriteString(fmt.Sprintf(
			`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, n, n))

		if err := add(fmt.Sprintf("xl/worksheets/sheet%d.xml", n), sheetXML(s)); err != nil {
			return nil, err
		}
	}
	types.WriteString(`</Types>`)

	if err := add("[Content_Types].xml", types.String()); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>`+
		`</Relationships>`); err != nil {
		return nil, err
	}
	if err := add("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" `+
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets>`+wbSheets.String()+`</sheets></workbook>`); err != nil {
		return nil, err
	}
	if err := add("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		wbRels.String()+`</Relationships>`); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sheetXML(s XLSXSheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for ri, row := range s.Rows {
		b.WriteString(fmt.Sprintf(`<row r="%d">`, ri+1))
		for ci, cell := range row {
			if cell == "" {
				continue // o Excel omite célula vazia — o leitor tem que aguentar
			}
			ref := colName(ci) + strconv.Itoa(ri+1)
			if isCanonicalNumber(cell) {
				b.WriteString(fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, cell))
			} else {
				b.WriteString(fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlEsc(cell)))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData>`)
	if len(s.Merges) > 0 {
		b.WriteString(fmt.Sprintf(`<mergeCells count="%d">`, len(s.Merges)))
		for _, m := range s.Merges {
			b.WriteString(fmt.Sprintf(`<mergeCell ref="%s"/>`, xmlEsc(m)))
		}
		b.WriteString(`</mergeCells>`)
	}
	b.WriteString(`</worksheet>`)
	return b.String()
}

// isCanonicalNumber: só o que o Excel gravaria como número puro. "1.234,56"
// NÃO entra — numa planilha real esse texto está numa célula de texto, e é
// justamente esse caso que o motor de parsing precisa enfrentar.
func isCanonicalNumber(s string) bool {
	if s == "" || strings.ContainsAny(s, ",R$% ") {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func colName(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
