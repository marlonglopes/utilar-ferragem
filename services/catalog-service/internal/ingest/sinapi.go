package ingest

// IMPORTADOR SINAPI — Sistema Nacional de Pesquisa de Custos e Índices da
// Construção Civil (Caixa Econômica Federal + IBGE).
//
// ============================================================================
// ⚠️  A REGRA MAIS IMPORTANTE DESTE ARQUIVO
// ============================================================================
// O preço do SINAPI é CUSTO DE REFERÊNCIA PARA ORÇAMENTO DE OBRA PÚBLICA.
// NÃO é preço de varejo, NÃO é preço da Utilar, e NÃO pode virar preço de
// venda automaticamente. Ele não contém margem, não contém frete de varejo,
// não contém impostos de saída, e é a MEDIANA de uma pesquisa estatística —
// vários itens têm preço de aquisição em volume de obra, muito abaixo do que
// qualquer ferragem consegue praticar.
//
// Por isso, mecanicamente:
//   - o valor do SINAPI é carregado em `cost` (referência), NUNCA em `price`
//   - `price` fica 0 e `price_reviewed` fica FALSE
//   - o produto entra `status='draft'`
//   - a constraint products_published_needs_review (migration 009) IMPEDE, no
//     banco, que um item nesse estado seja publicado
//
// Se um preço do SINAPI aparecer como preço da Utilar na vitrine, é erro
// grave. Há três camadas impedindo: este código, a constraint do banco, e o
// teste TestSINAPI_NuncaPublicaAutomaticamente.
// ============================================================================
//
// A OUTRA METADE, que é a mais valiosa: as COMPOSIÇÕES. Cada composição é um
// serviço de obra com o COEFICIENTE DE CONSUMO de cada insumo por unidade
// ("1 m² de alvenaria consome 13,5 blocos + 0,0086 m³ de argamassa"). É base de
// conhecimento oficial e citável pra responder "quanto material pra um muro de
// 20 m²" — o tipo de pergunta que uma assistente de ferragem precisa acertar e
// que, sem fonte, ela inventa.
//
// ---------------------------------------------------------------------------
// LAYOUT DO ARQUIVO OFICIAL (ver docs/base-de-produtos.md para procedência)
// ---------------------------------------------------------------------------
// Abas:
//   ISD / ICD / ISE — Insumos Sem / Com Desoneração / Sem Encargos
//   CSD / CCD / CSE — Composições Sem / Com Desoneração / Sem Encargos
//   Analítico       — composição + seus itens filhos com coeficiente
//
// Preâmbulo: várias linhas de título/marca/mês antes do cabeçalho real. O
// número de linhas MUDA ENTRE PUBLICAÇÕES MENSAIS — por isso o cabeçalho é
// localizado por palavra-chave (`findHeaderRow`), nunca por posição fixa.
// Codificar "o cabeçalho é a linha 10" garante quebra silenciosa em algum mês:
// a importação roda e carrega as notas de rodapé como se fossem produtos.
//
// Insumos:  CODIGO DO INSUMO | DESCRICAO DO INSUMO | UNIDADE | ORIGEM DE PRECO | <uma coluna por UF>
// Analítico: CODIGO DA COMPOSICAO | DESCRICAO DA COMPOSICAO | UNIDADE |
//            TIPO ITEM | CODIGO ITEM | DESCRICAO ITEM | UNIDADE ITEM | COEFICIENTE | ...
//
// No Analítico a estrutura é PAI/FILHO, não plana: a linha da composição tem a
// coluna TIPO vazia, e as linhas seguintes (TIPO = INSUMO ou COMPOSICAO) são
// seus itens — frequentemente com a célula de código da composição em branco.
// Daí o carry-forward de `currentComposition`.

import (
	"fmt"
	"sort"
	"strings"
)

// SINAPIInsumo é uma linha da aba de insumos.
type SINAPIInsumo struct {
	Code        string
	Description string
	Unit        string
	// ReferenceCost — o preço mediano do SINAPI para a UF. O NOME DO CAMPO É
	// DELIBERADO: chamar isto de `Price` convidaria, na próxima refatoração, a
	// alguém a ligá-lo em `products.price`. É custo de referência.
	ReferenceCost float64
	PriceOrigin   string
	UF            string
}

// SINAPIComposition é um serviço de obra com seus coeficientes de consumo.
type SINAPIComposition struct {
	Code          string
	Description   string
	Unit          string
	ReferenceCost float64
	Items         []SINAPICompositionItem
}

// SINAPICompositionItem é a linha filha: quanto de um insumo (ou de outra
// composição) entra em UMA unidade da composição pai.
type SINAPICompositionItem struct {
	Type        string // "insumo" | "composicao"
	Code        string
	Description string
	Unit        string
	Coefficient float64
}

// SINAPIResult é o retorno do parse do arquivo.
type SINAPIResult struct {
	Insumos      []SINAPIInsumo
	Compositions []SINAPIComposition
	UF           string
	ReferenceMonth string
	Desonerado   bool
	Warnings     []string
	// SkippedLabor conta os insumos descartados por serem mão de obra,
	// locação ou energia. Reportado porque é um número grande e esperado — se
	// aparecer zero, o filtro quebrou e pedreiro está entrando como produto.
	SkippedLabor int
}

// Abas conhecidas. `desonerado` importa: a Caixa publica as duas versões e
// MISTURAR AS DUAS numa mesma base produz orçamento errado em silêncio —
// desoneração muda o custo da mão de obra e, por tabela, das composições.
var sinapiInsumoSheets = map[string]bool{"isd": false, "icd": true, "ise": false}
var sinapiCompositionSheets = map[string]bool{"csd": false, "ccd": true, "cse": false}

// ParseSINAPIInsumos lê uma aba de insumos (ISD/ICD/ISE).
//
// `uf` seleciona a coluna de preço: no arquivo nacional há uma coluna por
// estado; no ZIP por UF há uma coluna única de preço mediano. Ambos são
// suportados porque a Caixa publica os dois formatos.
func ParseSINAPIInsumos(grid [][]string, uf string) (*SINAPIResult, error) {
	hdrIdx, header, err := findHeaderRow(grid, []string{"codigo", "descricao"})
	if err != nil {
		return nil, fmt.Errorf("aba de insumos: %w", err)
	}

	res := &SINAPIResult{UF: strings.ToUpper(strings.TrimSpace(uf))}

	codeCol := findCol(header, "codigo")
	descCol := findCol(header, "descricao")
	unitCol := findCol(header, "unidade")
	originCol := findCol(header, "origem")
	if codeCol < 0 || descCol < 0 || unitCol < 0 {
		return nil, fmt.Errorf("aba de insumos sem as colunas obrigatórias (código, descrição, unidade); cabeçalho lido: %s",
			strings.Join(header, " | "))
	}

	// Coluna de preço: a UF pedida, ou a coluna genérica do arquivo por-UF.
	priceCol := -1
	if res.UF != "" {
		priceCol = findExactCol(header, res.UF)
	}
	if priceCol < 0 {
		for _, cand := range []string{"preco mediano", "preco", "custo"} {
			if c := findCol(header, cand); c >= 0 && c != originCol {
				priceCol = c
				break
			}
		}
	}
	if priceCol < 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"nenhuma coluna de preço encontrada para a UF %q — insumos importados SEM custo de referência", res.UF))
	}

	seen := map[string]bool{}
	for i := hdrIdx + 1; i < len(grid); i++ {
		row := grid[i]
		code, ok, _ := ParseCode("codigo", at(row, codeCol))
		if !ok || code == "" || code == "0" {
			continue // linha de rodapé, nota, ou o código-sentinela 0
		}
		desc := CleanText(at(row, descCol))
		if desc == "" {
			continue
		}
		// Duplicatas existem no arquivo oficial.
		if seen[code] {
			continue
		}
		seen[code] = true

		unit := strings.ToUpper(CleanText(at(row, unitCol)))
		if IsLaborUnit(unit) {
			res.SkippedLabor++
			continue
		}

		in := SINAPIInsumo{
			Code: code, Description: desc, Unit: unit,
			PriceOrigin: CleanText(at(row, originCol)), UF: res.UF,
		}
		if priceCol >= 0 {
			if v, ok, _ := ParseMoney("custo", at(row, priceCol)); ok {
				in.ReferenceCost = v
			}
		}
		res.Insumos = append(res.Insumos, in)
	}

	if len(res.Insumos) == 0 {
		return nil, fmt.Errorf("nenhum insumo reconhecido abaixo do cabeçalho (linha %d) — o layout do arquivo pode ter mudado", hdrIdx+1)
	}
	return res, nil
}

// ParseSINAPIAnalitico lê a aba Analítico e monta as composições com seus
// coeficientes.
func ParseSINAPIAnalitico(grid [][]string) ([]SINAPIComposition, error) {
	hdrIdx, header, err := findHeaderRow(grid, []string{"codigo", "tipo"})
	if err != nil {
		return nil, fmt.Errorf("aba analítico: %w", err)
	}

	compCodeCol := findCol(header, "codigo da composicao")
	if compCodeCol < 0 {
		compCodeCol = findCol(header, "codigo")
	}
	compDescCol := findCol(header, "descricao da composicao")
	if compDescCol < 0 {
		compDescCol = findCol(header, "descricao")
	}
	compUnitCol := findCol(header, "unidade")
	typeCol := findCol(header, "tipo")
	itemCodeCol := findColAfter(header, "codigo", compCodeCol)
	itemDescCol := findColAfter(header, "descricao", compDescCol)
	itemUnitCol := findColAfter(header, "unidade", compUnitCol)
	coefCol := findCol(header, "coeficiente")

	if typeCol < 0 || coefCol < 0 {
		return nil, fmt.Errorf("aba analítico sem coluna de tipo ou coeficiente; cabeçalho lido: %s",
			strings.Join(header, " | "))
	}

	var out []SINAPIComposition
	byCode := map[string]*SINAPIComposition{}
	// Carry-forward: as linhas filhas costumam vir com a célula do código da
	// composição em branco. Sem carregar o valor da última linha-pai, todo item
	// fica órfão e os coeficientes se perdem — que é justamente o dado que
	// justifica importar composições.
	var current *SINAPIComposition

	for i := hdrIdx + 1; i < len(grid); i++ {
		row := grid[i]
		itemType := strings.ToLower(stripAccents(CleanText(at(row, typeCol))))
		code, _, _ := ParseCode("codigo", at(row, compCodeCol))

		// Linha PAI: tipo vazio + código presente.
		if itemType == "" {
			if code == "" || code == "0" {
				continue
			}
			desc := CleanText(at(row, compDescCol))
			if desc == "" {
				continue
			}
			if c, ok := byCode[code]; ok {
				current = c
				continue
			}
			out = append(out, SINAPIComposition{
				Code: code, Description: desc,
				Unit: strings.ToUpper(CleanText(at(row, compUnitCol))),
			})
			current = &out[len(out)-1]
			byCode[code] = current
			continue
		}

		// Linha FILHA.
		if current == nil {
			continue // item antes de qualquer composição: linha de sujeira
		}
		if code != "" && code != "0" && code != current.Code {
			if c, ok := byCode[code]; ok {
				current = c
			}
		}

		var kind string
		switch {
		case strings.HasPrefix(itemType, "insumo"):
			kind = "insumo"
		case strings.HasPrefix(itemType, "composicao"):
			kind = "composicao"
		default:
			continue
		}

		itemCode, ok, _ := ParseCode("codigo item", at(row, itemCodeCol))
		if !ok || itemCode == "" {
			continue
		}
		coef, ok, _ := ParseNumber("coeficiente", at(row, coefCol))
		if !ok || coef <= 0 {
			// Coeficiente ausente ou zero torna o item inútil pro cálculo de
			// material — e um coeficiente errado é pior que item ausente,
			// porque produz uma lista de compras plausível e errada.
			continue
		}

		// Dedup por (composição, tipo, item): duplicatas ocorrem no arquivo.
		dup := false
		for _, it := range current.Items {
			if it.Type == kind && it.Code == itemCode {
				dup = true
				break
			}
		}
		if dup {
			continue
		}

		current.Items = append(current.Items, SINAPICompositionItem{
			Type: kind, Code: itemCode,
			Description: CleanText(at(row, itemDescCol)),
			Unit:        strings.ToUpper(CleanText(at(row, itemUnitCol))),
			Coefficient: coef,
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("nenhuma composição reconhecida — o layout do arquivo pode ter mudado")
	}
	return out, nil
}

// ParseSINAPIWorkbook lê o .xlsx inteiro: escolhe as abas, aplica desoneração
// e devolve insumos + composições.
func ParseSINAPIWorkbook(data []byte, uf, referenceMonth string, desonerado bool) (*SINAPIResult, error) {
	names, err := SheetNames(data)
	if err != nil {
		return nil, err
	}

	pick := func(m map[string]bool) string {
		for _, n := range names {
			key := strings.ToLower(strings.TrimSpace(stripAccents(n)))
			if d, ok := m[key]; ok && d == desonerado {
				return n
			}
		}
		return ""
	}

	insumoSheet := pick(sinapiInsumoSheets)
	if insumoSheet == "" {
		return nil, fmt.Errorf("nenhuma aba de insumos (%s) encontrada; abas do arquivo: %s",
			sheetKeys(sinapiInsumoSheets, desonerado), strings.Join(names, ", "))
	}

	grid, err := ReadXLSX(data, insumoSheet)
	if err != nil {
		return nil, err
	}
	res, err := ParseSINAPIInsumos(grid, uf)
	if err != nil {
		return nil, err
	}
	res.ReferenceMonth = referenceMonth
	res.Desonerado = desonerado

	// Composições: opcionais. Um arquivo só de insumos ainda é um import
	// válido — a base de conhecimento é um bônus, não um pré-requisito.
	for _, n := range names {
		if strings.HasPrefix(strings.ToLower(stripAccents(n)), "analitico") {
			cg, err := ReadXLSX(data, n)
			if err != nil {
				res.Warnings = append(res.Warnings, "aba analítico ilegível: "+err.Error())
				break
			}
			comps, err := ParseSINAPIAnalitico(cg)
			if err != nil {
				res.Warnings = append(res.Warnings, "composições não importadas: "+err.Error())
				break
			}
			res.Compositions = comps
			break
		}
	}
	if len(res.Compositions) == 0 {
		res.Warnings = append(res.Warnings,
			"nenhuma composição importada — a base de conhecimento de coeficientes de consumo ficará vazia")
	}
	return res, nil
}

// ToTable converte os insumos numa Table, pra que sigam pelo MESMO pipeline
// (validação por linha, dry-run, upsert idempotente) que CSV e XLSX.
//
// Reaproveitar o pipeline não é economia de código — é garantia de que as
// travas (limite de queda de preço, nunca publicar sozinho, staging cru,
// auditoria) valem igual para o SINAPI. Um caminho paralelo de escrita seria
// exatamente onde a regra de preço vazaria.
//
// Note a coluna: `CUSTO_REFERENCIA_SINAPI`. Ela é mapeada em `cost`, e o perfil
// do SINAPI (SINAPIProfile) NÃO tem mapeamento para `price`.
func (r *SINAPIResult) ToTable() *Table {
	t := &Table{
		Header: []string{"CODIGO", "DESCRICAO", "UNIDADE", "CUSTO_REFERENCIA_SINAPI", "CATEGORIA_UTILAR", "ORIGEM_PRECO"},
		Format: "sinapi",
	}
	for i, in := range r.Insumos {
		cost := ""
		if in.ReferenceCost > 0 {
			cost = fmt.Sprintf("%.2f", in.ReferenceCost)
		}
		t.Rows = append(t.Rows, []string{
			"SINAPI-" + in.Code, // prefixo: o SKU não pode colidir com código do fornecedor
			in.Description,
			in.Unit,
			cost,
			MapSINAPICategory(in.Description, in.Unit),
			in.PriceOrigin,
		})
		t.RowNumbers = append(t.RowNumbers, i+1)
	}
	return t
}

// SINAPIProfile é o perfil de mapeamento do SINAPI.
//
// NOTE O QUE NÃO ESTÁ AQUI: nenhuma coluna mapeia `price`. Não é esquecimento —
// é a regra de negócio expressa na estrutura de dados. Mesmo que alguém edite
// a planilha e adicione uma coluna "PRECO", ela não tem para onde ir.
func SINAPIProfile() *Profile {
	return &Profile{
		Name:    "sinapi-insumos",
		Version: 1,
		Kind:    "sinapi",
		Columns: map[string]ColumnMapping{
			"CODIGO":                  {Field: FieldSKU, Parser: ParserCode},
			"DESCRICAO":               {Field: FieldName, Parser: ParserText},
			"UNIDADE":                 {Field: FieldUnitOfMeasure, Parser: ParserText},
			"CUSTO_REFERENCIA_SINAPI": {Field: FieldCost, Parser: ParserMoneyBR},
			"CATEGORIA_UTILAR":        {Field: FieldCategory, Parser: ParserText},
			"ORIGEM_PRECO":            {Field: FieldSpecs, SpecKey: "Origem do preço (SINAPI)"},
		},
		Options: Options{
			// Explícito, mesmo sendo o padrão: aqui a intenção precisa estar
			// escrita. Nunca publica, nunca arquiva por ausência (o SINAPI é
			// referência externa, não a planilha de estoque da loja).
			PublishOnImport: false,
			ArchiveMissing:  false,
		},
	}
}

// ---------------------------------------------------------------------------
// MAPEAMENTO SINAPI → CATEGORIAS DA UTILAR
// ---------------------------------------------------------------------------
// O SINAPI não tem categoria: tem uma descrição padronizada em maiúsculas sem
// acento. Classificamos por palavra-chave sobre essa descrição.
//
// As regras estão em ORDEM DE PRECEDÊNCIA e a primeira que casa vence. A ordem
// importa: "TUBO PVC PARA ELETRODUTO" é elétrica, não hidráulica, e só acerta
// porque a regra de eletroduto vem antes da de tubo. Toda regra abaixo tem
// comentário quando a ordem é o que a faz funcionar.
//
// O que NÃO casa vira `construcao` — a categoria mais genérica — e o produto
// entra como rascunho de qualquer jeito, então a curadoria corrige antes de
// publicar. Errar pra "construcao" é recuperável; errar pra uma categoria
// específica esconde o item da revisão.
type sinapiRule struct {
	category string
	keywords []string
}

var sinapiCategoryRules = []sinapiRule{
	// EPI primeiro: "LUVA DE RASPA" e "BOTA DE SEGURANCA" contêm palavras que
	// casariam com outras categorias ("luva" também é conexão hidráulica!).
	// Esta precedência é a que impede luva de proteção virar conexão de esgoto.
	{"seguranca", []string{"EQUIPAMENTO DE PROTECAO", "PROTECAO INDIVIDUAL", "EPI ",
		"CAPACETE", "OCULOS DE", "PROTETOR AURICULAR", "MASCARA", "RESPIRADOR",
		"CINTO DE SEGURANCA", "TALABARTE", "BOTINA", "BOTA DE SEGURANCA",
		"LUVA DE RASPA", "LUVA DE SEGURANCA", "LUVA NITRILICA", "LUVA DE LATEX",
		"COLETE REFLETIVO", "CONE DE SINALIZACAO", "FITA ZEBRADA", "EXTINTOR"}},

	// Elétrica antes de hidráulica: eletroduto é "TUBO", e o conflito real é
	// "ELETRODUTO DE PVC RIGIDO" — que a regra de "TUBO PVC" capturaria.
	{"eletrica", []string{"ELETRODUTO", "CABO DE COBRE", "FIO DE COBRE", "CABO FLEXIVEL",
		"CONDUTOR", "DISJUNTOR", "QUADRO DE DISTRIBUICAO", "TOMADA", "INTERRUPTOR",
		"LAMPADA", "LUMINARIA", "REATOR", "SOQUETE", "FITA ISOLANTE", "TERMINAL ELETRICO",
		"CAIXA DE PASSAGEM", "CABO TELEFONICO", "CABO COAXIAL", "ATERRAMENTO",
		"HASTE DE ATERRAMENTO", "DPS ", "CONTATOR", "TRANSFORMADOR", "ELETRICID"}},

	{"hidraulica", []string{"TUBO PVC", "TUBO DE PVC", "TUBO PPR", "TUBO DE COBRE",
		"TUBO DE FERRO GALVANIZADO", "JOELHO", "COTOVELO", "TE ", "LUVA SOLDAVEL",
		"LUVA ROSQUEAVEL", "ADAPTADOR SOLDAVEL", "REGISTRO DE", "TORNEIRA", "VALVULA",
		"SIFAO", "CAIXA DAGUA", "CAIXA D AGUA", "RESERVATORIO", "HIDROMETRO",
		"VASO SANITARIO", "LAVATORIO", "PIA DE", "CUBA", "DUCHA", "CHUVEIRO",
		"ESGOTO", "CALHA", "TUBO DE QUEDA", "MANGUEIRA"}},

	{"pintura", []string{"TINTA", "VERNIZ", "SELADOR", "MASSA CORRIDA", "MASSA ACRILICA",
		"FUNDO PREPARADOR", "SOLVENTE", "AGUARRAS", "THINNER", "ROLO DE LA",
		"PINCEL", "TRINCHA", "LIXA PARA PAREDE", "ESMALTE SINTETICO", "TEXTURA ACRILICA",
		"IMPERMEABILIZANTE", "PRIMER"}},

	// Fixação antes de construção: "PREGO" e "PARAFUSO" são itens de fixação
	// mesmo aparecendo em descrições de estrutura de madeira.
	{"fixacao", []string{"PARAFUSO", "PREGO", "BUCHA DE NYLON", "BUCHA PLASTICA",
		"ARRUELA", "PORCA", "REBITE", "CHUMBADOR", "GRAMPO", "ABRACADEIRA",
		"BARRA ROSCADA", "PINO DE ACO", "TARUGO", "CAVILHA"}},

	{"ferramentas", []string{"FURADEIRA", "SERRA CIRCULAR", "SERRA MARMORE", "MARTELETE",
		"ESMERILHADEIRA", "LIXADEIRA", "PARAFUSADEIRA", "BETONEIRA", "VIBRADOR",
		"COMPACTADOR", "MARTELO", "TALHADEIRA", "PONTEIRO", "COLHER DE PEDREIRO",
		"DESEMPENADEIRA", "ENXADA", "PA ", "CARRINHO DE MAO", "NIVEL DE", "PRUMO",
		"TRENA", "ESQUADRO", "SERROTE", "ALICATE", "CHAVE DE FENDA", "CHAVE PHILIPS",
		"CHAVE INGLESA", "BROCA", "DISCO DE CORTE", "DISCO DIAMANTADO", "ANDAIME",
		"ESCADA DE", "BALDE", "MASSEIRA", "PENEIRA", "ARCO DE SERRA"}},

	{"jardim", []string{"GRAMA", "TERRA VEGETAL", "ADUBO", "MUDA DE", "SUBSTRATO",
		"REGADOR", "TESOURA DE PODA", "CERCA VIVA", "IRRIGACAO", "ASPERSOR"}},

	// Construção por último: é o balde genérico, e boa parte do SINAPI cai aqui
	// legitimamente (cimento, areia, brita, bloco, aço, madeira).
	{"construcao", []string{"CIMENTO", "AREIA", "BRITA", "PEDRA", "CAL ", "ARGAMASSA",
		"CONCRETO", "BLOCO", "TIJOLO", "TELHA", "LAJE", "VIGA", "ACO CA-", "VERGALHAO",
		"TELA SOLDADA", "ARAME", "MADEIRA", "TABUA", "CAIBRO", "SARRAFO", "COMPENSADO",
		"CHAPA", "GESSO", "DRYWALL", "PLACA CIMENTICIA", "REVESTIMENTO", "PISO",
		"PORCELANATO", "CERAMICA", "AZULEJO", "REJUNTE", "MANTA ASFALTICA",
		"LA DE VIDRO", "LA DE ROCHA", "ISOPOR", "EPS", "PORTA", "JANELA", "BATENTE",
		"FECHADURA", "DOBRADICA", "VIDRO", "PERFIL", "CANTONEIRA", "TUBO DE ACO"}},
}

// MapSINAPICategory classifica um insumo do SINAPI numa das 8 categorias da
// Utilar. Devolve "construcao" quando nada casa — ver comentário acima.
// A comparação exige FRONTEIRA DE PALAVRA à esquerda (o termo tem que começar
// um vocábulo), e não `strings.Contains` puro.
//
// O motivo é concreto: a palavra-chave "TE " (o TÊ, conexão hidráulica) casava
// com "COMPLETAMENTE DESCONHECIDO" pelo sufixo "…TE ", e classificava qualquer
// item cuja descrição contivesse um advérbio como hidráulica. O mesmo valia pra
// "PA " (pá) dentro de "PAREDE" e "CAL " dentro de "DECAL…". Termos curtos são
// justamente os que mais aparecem no meio de outras palavras — e o erro é
// invisível, porque o produto vai parar numa categoria plausível.
func MapSINAPICategory(description, unit string) string {
	d := " " + strings.ToUpper(stripAccents(CleanText(description))) + " "
	for _, rule := range sinapiCategoryRules {
		for _, kw := range rule.keywords {
			if strings.Contains(d, " "+kw) {
				return rule.category
			}
		}
	}
	return "construcao"
}

// SINAPICategoryRules expõe as regras pra documentação e teste — a tabela do
// docs/base-de-produtos.md é gerada a partir daqui pra não divergir do código.
func SINAPICategoryRules() map[string][]string {
	out := map[string][]string{}
	for _, r := range sinapiCategoryRules {
		out[r.category] = append(out[r.category], r.keywords...)
	}
	return out
}

// --- helpers de layout ------------------------------------------------------

// findHeaderRow localiza o cabeçalho por PALAVRA-CHAVE nas primeiras 30 linhas.
// Ver comentário no topo: posição fixa quebra a cada publicação mensal.
func findHeaderRow(grid [][]string, required []string) (int, []string, error) {
	limit := len(grid)
	if limit > 30 {
		limit = 30
	}
	for i := 0; i < limit; i++ {
		norm := make([]string, len(grid[i]))
		for j, c := range grid[i] {
			norm[j] = strings.ToLower(stripAccents(CleanText(c)))
		}
		joined := strings.Join(norm, "|")
		all := true
		for _, req := range required {
			if !strings.Contains(joined, req) {
				all = false
				break
			}
		}
		if all {
			return i, norm, nil
		}
	}
	return 0, nil, fmt.Errorf("cabeçalho com as colunas %v não encontrado nas primeiras %d linhas — o layout do arquivo mudou",
		required, limit)
}

func findCol(header []string, want string) int {
	// Casamento exato primeiro, depois por prefixo, depois por conteúdo — pra
	// que "codigo" não pegue "codigo item" quando existe uma coluna "codigo".
	for _, mode := range []int{0, 1, 2} {
		for i, h := range header {
			switch mode {
			case 0:
				if h == want {
					return i
				}
			case 1:
				if strings.HasPrefix(h, want) {
					return i
				}
			case 2:
				if strings.Contains(h, want) {
					return i
				}
			}
		}
	}
	return -1
}

func findExactCol(header []string, want string) int {
	w := strings.ToLower(strings.TrimSpace(want))
	for i, h := range header {
		if strings.TrimSpace(h) == w {
			return i
		}
	}
	return -1
}

// findColAfter acha a PRÓXIMA coluna que casa, depois de `after`. É como
// distinguimos "CODIGO DA COMPOSICAO" de "CODIGO ITEM" no Analítico, onde os
// dois cabeçalhos contêm "codigo".
func findColAfter(header []string, want string, after int) int {
	for i := after + 1; i < len(header); i++ {
		if strings.Contains(header[i], want) {
			return i
		}
	}
	return findCol(header, want)
}

func at(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func sheetKeys(m map[string]bool, desonerado bool) string {
	var out []string
	for k, d := range m {
		if d == desonerado {
			out = append(out, strings.ToUpper(k))
		}
	}
	sort.Strings(out)
	return strings.Join(out, "/")
}
