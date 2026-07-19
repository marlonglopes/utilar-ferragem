package ingest

// Testes do importador SINAPI.
//
// ⚠️ PROCEDÊNCIA DOS DADOS DESTE ARQUIVO — leia antes de confiar nos números.
//
// O arquivo oficial da Caixa NÃO pôde ser baixado no ambiente de
// desenvolvimento: o CDN de caixa.gov.br responde 302 em laço e depois 429/403
// para requisições de fora do Brasil (bloqueio de origem, não URL errada — o
// próprio 302 devolve a URL pedida). Ver docs/base-de-produtos.md.
//
// Portanto: o XLSX usado aqui é uma AMOSTRA CONSTRUÍDA POR NÓS que replica o
// LAYOUT documentado do arquivo real (abas ISD/CSD/Analítico, linhas de
// preâmbulo antes do cabeçalho, células mescladas, números em formato
// brasileiro, descrições em maiúsculas sem acento, estrutura pai/filho no
// Analítico com o código da composição em branco nas linhas filhas).
//
// OS VALORES NÃO SÃO DADOS DO SINAPI. Os códigos, descrições e preços aqui são
// inventados para exercitar o parser. NENHUM deles deve ser tratado como preço
// oficial de referência, e nada neste arquivo é carregado em produção.
// O que é fiel é a FORMA; o conteúdo é fixture.

import (
	"strings"
	"testing"
)

// sampleSINAPIWorkbook monta um .xlsx com o layout do arquivo oficial.
//
// Reproduz deliberadamente as armadilhas do arquivo real:
//   - 5 linhas de preâmbulo (título, marca, mês, UF) antes do cabeçalho
//   - título mesclado ocupando a linha inteira
//   - número em formato brasileiro como TEXTO ("1.234,56")
//   - código com decimal fantasma
//   - mão de obra (unidade H) misturada com material
//   - linhas filhas do Analítico sem o código da composição
func sampleSINAPIWorkbook(t *testing.T) []byte {
	t.Helper()

	insumos := [][]string{
		{"CAIXA ECONOMICA FEDERAL"},
		{"SINAPI - SISTEMA NACIONAL DE PESQUISA DE CUSTOS E INDICES DA CONSTRUCAO CIVIL"},
		{"PRECOS DE INSUMOS - AMOSTRA DE LAYOUT (NAO E DADO OFICIAL)"},
		{"Referencia: 04/2026"},
		{"SP"},
		{}, // linha em branco, como no arquivo real
		{"CODIGO DO INSUMO", "DESCRICAO DO INSUMO", "UNIDADE", "ORIGEM DE PRECO", "SP"},
		// Material legítimo — preço BR como texto.
		{"1379", "CIMENTO PORTLAND COMPOSTO CP II-32", "SC25KG", "COTACAO", "32,50"},
		{"370", "AREIA MEDIA - POSTO JAZIDA/FORNECEDOR (RETIRADO NA JAZIDA, SEM TRANSPORTE)", "M3", "COTACAO", "1.234,56"},
		{"7356", "TIJOLO CERAMICO MACICO 5 X 10 X 20CM", "MIL", "COTACAO", "890,00"},
		{"4720", "CABO DE COBRE FLEXIVEL ISOLADO, 2,5 MM2, ANTI-CHAMA 450/750 V", "M", "COTACAO", "2,45"},
		{"11946", "TUBO PVC SOLDAVEL PARA AGUA FRIA, DN 25 MM", "M", "COTACAO", "5,80"},
		{"7361", "ELETRODUTO DE PVC RIGIDO ROSCAVEL, DN 25 MM", "M", "COTACAO", "4,10"},
		{"36136", "LUVA DE RASPA CANO CURTO", "PAR", "COTACAO", "12,90"},
		{"4083", "PARAFUSO ZINCADO ROSCA SOBERBA 4,2 X 32 MM", "UN", "COTACAO", "0,35"},
		{"7286", "TINTA ACRILICA PREMIUM, COR BRANCO FOSCO", "L", "COTACAO", "28,70"},
		// MÃO DE OBRA — tem que ser descartado, não virar produto.
		{"88316", "SERVENTE COM ENCARGOS COMPLEMENTARES", "H", "MAO DE OBRA", "22,45"},
		{"88309", "PEDREIRO COM ENCARGOS COMPLEMENTARES", "H", "MAO DE OBRA", "30,12"},
		// Locação — também descartado.
		{"5678", "LOCACAO DE ANDAIME METALICO", "M2XMES", "COTACAO", "15,00"},
		// Código-sentinela 0 e linha de rodapé — ignorados.
		{"0", "", "", "", ""},
		{"Fonte: Caixa Economica Federal / IBGE"},
	}

	analitico := [][]string{
		{"CAIXA ECONOMICA FEDERAL"},
		{"SINAPI - RELATORIO ANALITICO DE COMPOSICOES (AMOSTRA DE LAYOUT)"},
		{"Referencia: 04/2026 - SP"},
		{},
		{"CODIGO DA COMPOSICAO", "DESCRICAO DA COMPOSICAO", "UNIDADE",
			"TIPO ITEM", "CODIGO ITEM", "DESCRICAO ITEM", "UNIDADE ITEM", "COEFICIENTE", "PRECO UNITARIO", "CUSTO TOTAL"},

		// --- Composição 1: linha PAI tem TIPO vazio ------------------------
		{"87473", "ALVENARIA DE VEDACAO DE BLOCOS CERAMICOS FURADOS NA VERTICAL", "M2", "", "", "", "", "", "", "68,42"},
		// Linhas FILHAS — sem o código da composição, como no arquivo real.
		{"", "", "", "INSUMO", "7356", "TIJOLO CERAMICO MACICO 5 X 10 X 20CM", "MIL", "0,0135", "890,00", "12,02"},
		{"", "", "", "INSUMO", "1379", "CIMENTO PORTLAND COMPOSTO CP II-32", "SC25KG", "0,0086", "32,50", "0,28"},
		{"", "", "", "INSUMO", "370", "AREIA MEDIA", "M3", "0,0043", "1.234,56", "5,31"},
		{"", "", "", "COMPOSICAO", "88309", "PEDREIRO COM ENCARGOS", "H", "0,73", "30,12", "21,99"},
		// Duplicata: ocorre no arquivo oficial, tem que ser deduplicada.
		{"", "", "", "INSUMO", "7356", "TIJOLO CERAMICO MACICO 5 X 10 X 20CM", "MIL", "0,0135", "890,00", "12,02"},

		// --- Composição 2 --------------------------------------------------
		{"87879", "CHAPISCO APLICADO EM ALVENARIA COM COLHER DE PEDREIRO", "M2", "", "", "", "", "", "", "4,21"},
		{"", "", "", "INSUMO", "1379", "CIMENTO PORTLAND COMPOSTO CP II-32", "SC25KG", "0,0182", "32,50", "0,59"},
		{"", "", "", "INSUMO", "370", "AREIA MEDIA", "M3", "0,0031", "1.234,56", "3,83"},
		// Coeficiente zero: item inútil pro cálculo, tem que ser ignorado.
		{"", "", "", "INSUMO", "4083", "PARAFUSO", "UN", "0", "0,35", "0,00"},
	}

	data, err := WriteXLSX(sheetSpec(t, insumos, analitico))
	if err != nil {
		t.Fatalf("gerar XLSX de amostra: %v", err)
	}
	return data
}

func sheetSpec(t *testing.T, insumos, analitico [][]string) []XLSXSheet {
	t.Helper()
	return []XLSXSheet{
		{
			Name: "ISD", Rows: insumos,
			// Título mesclado na linha inteira, como no arquivo real — o leitor
			// tem que propagar o valor e ainda assim achar o cabeçalho certo.
			Merges: []string{"A2:E2", "A3:E3"},
		},
		{Name: "Analítico", Rows: analitico, Merges: []string{"A2:J2"}},
	}
}

// ============================================================================
// ⚠️ O TESTE MAIS IMPORTANTE DESTE ARQUIVO
// ============================================================================
// O preço do SINAPI é custo de referência para obra pública. Se ele aparecer
// como preço da Utilar na vitrine, é erro grave. Este teste trava as três
// propriedades que impedem isso.
func TestSINAPI_NuncaPublicaAutomaticamente(t *testing.T) {
	data := sampleSINAPIWorkbook(t)

	res, err := ParseSINAPIWorkbook(data, "SP", "04/2026", false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	profile := SINAPIProfile()

	// 1) O PERFIL NÃO PODE TER MAPEAMENTO PARA `price`. Nem um.
	for col, m := range profile.Columns {
		if m.Field == FieldPrice {
			t.Fatalf("REGRESSÃO CRÍTICA: a coluna %q do perfil SINAPI mapeia `price`. "+
				"O valor do SINAPI é CUSTO DE OBRA PÚBLICA, nunca preço de venda.", col)
		}
		if m.Field == FieldStatus {
			t.Fatalf("REGRESSÃO CRÍTICA: o perfil SINAPI mapeia `status` — o arquivo "+
				"não pode decidir o que vai pra vitrine")
		}
	}

	// 2) O perfil NÃO pode publicar na importação.
	if profile.Options.PublishOnImport {
		t.Fatal("REGRESSÃO CRÍTICA: perfil SINAPI com publishOnImport=true")
	}

	// 3) NENHUMA linha planejada pode carregar preço de venda, e o valor do
	//    SINAPI tem que estar em `cost`.
	cat := newFakeCatalog()
	planner := &Planner{Profile: profile, Catalog: cat}
	plan, err := planner.Plan(res.ToTable())
	if err != nil {
		t.Fatalf("plano: %v", err)
	}

	if plan.Creates == 0 {
		t.Fatal("nenhum insumo planejado — o teste não estaria verificando nada")
	}

	for _, r := range plan.Rows {
		if r.Action == ActionReject {
			continue
		}
		if v, ok := r.Mapped[string(FieldPrice)]; ok {
			t.Errorf("REGRESSÃO CRÍTICA: linha %d (SKU %s) tem preço de venda %v vindo do SINAPI",
				r.RowNumber, r.SKU, v)
		}
		if st, ok := r.Mapped[string(FieldStatus)].(string); ok && st == "published" {
			t.Errorf("REGRESSÃO CRÍTICA: linha %d (SKU %s) seria publicada automaticamente",
				r.RowNumber, r.SKU)
		}
		// E o custo de referência precisa ter chegado.
		if _, ok := r.Mapped[string(FieldCost)]; !ok {
			t.Errorf("linha %d (SKU %s) sem custo de referência — o dado do SINAPI se perdeu",
				r.RowNumber, r.SKU)
		}
	}
}

// A trava do commit: `effectiveStatus` nunca marca item SINAPI como revisado.
func TestSINAPI_CommitNuncaMarcaPrecoComoRevisado(t *testing.T) {
	c := &Committer{Source: "sinapi", Profile: SINAPIProfile()}

	// Mesmo que a linha traga preço E peça publicação explicitamente.
	r := &RowResult{
		SKU: "SINAPI-1379",
		Mapped: map[string]any{
			"price":  99.90,
			"status": "published",
		},
	}
	status, reviewed := c.effectiveStatus(r, true, "")
	if reviewed {
		t.Error("REGRESSÃO CRÍTICA: item SINAPI marcado como preço revisado")
	}
	if status == "published" {
		t.Error("REGRESSÃO CRÍTICA: item SINAPI publicado no commit")
	}

	// Já um import normal com preço válido pode publicar quando o perfil manda.
	c2 := &Committer{Source: "import", Profile: &Profile{
		Name: "x", Columns: map[string]ColumnMapping{"N": {Field: FieldName}},
		Options: Options{PublishOnImport: true},
	}}
	r2 := &RowResult{SKU: "P-1", Mapped: map[string]any{"price": 42.90}}
	if status, reviewed := c2.effectiveStatus(r2, true, ""); status != "published" || !reviewed {
		t.Errorf("import normal com preço deveria poder publicar; status=%q reviewed=%v", status, reviewed)
	}
}

// ============================================================================
// Parsing do layout
// ============================================================================

func TestSINAPI_LeInsumosComPreambuloEFormatoBR(t *testing.T) {
	data := sampleSINAPIWorkbook(t)
	res, err := ParseSINAPIWorkbook(data, "SP", "04/2026", false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	byCode := map[string]SINAPIInsumo{}
	for _, in := range res.Insumos {
		byCode[in.Code] = in
	}

	// Número brasileiro como texto: "1.234,56" NÃO pode virar 1,23.
	areia, ok := byCode["370"]
	if !ok {
		t.Fatal("insumo 370 (areia) não importado")
	}
	if areia.ReferenceCost < 1000 {
		t.Errorf("REGRESSÃO: custo da areia = %.2f, esperado 1234.56 — erro de vírgula", areia.ReferenceCost)
	}

	// Unidade com peso embutido, específica do SINAPI.
	cimento, ok := byCode["1379"]
	if !ok {
		t.Fatal("insumo 1379 (cimento) não importado")
	}
	if cimento.Unit != "SC25KG" {
		t.Errorf("unidade do cimento = %q, quer SC25KG", cimento.Unit)
	}
	if w := UnitWeightKg(cimento.Unit); w != 25 {
		t.Errorf("peso extraído de SC25KG = %v, quer 25", w)
	}
}

func TestSINAPI_DescartaMaoDeObraELocacao(t *testing.T) {
	// Servente, pedreiro e locação de andaime não são itens de prateleira.
	data := sampleSINAPIWorkbook(t)
	res, err := ParseSINAPIWorkbook(data, "SP", "04/2026", false)
	if err != nil {
		t.Fatal(err)
	}

	if res.SkippedLabor != 3 {
		t.Errorf("mão de obra/locação descartada = %d, quer 3", res.SkippedLabor)
	}
	for _, in := range res.Insumos {
		if strings.Contains(in.Description, "SERVENTE") || strings.Contains(in.Description, "PEDREIRO") {
			t.Errorf("mão de obra entrou como produto: %s", in.Description)
		}
		if IsLaborUnit(in.Unit) {
			t.Errorf("insumo com unidade de serviço importado: %s (%s)", in.Description, in.Unit)
		}
	}
}

func TestSINAPI_CabecalhoLocalizadoPorPalavraChave(t *testing.T) {
	// O nº de linhas de preâmbulo muda entre publicações mensais. Posição fixa
	// quebraria em silêncio, importando notas de rodapé como produtos.
	grid := [][]string{
		{"CAIXA"},
		{"TITULO"},
		{},
		{},
		{},
		{},
		{},
		{"MAIS UMA LINHA DE NOTA"},
		{"CODIGO DO INSUMO", "DESCRICAO DO INSUMO", "UNIDADE", "ORIGEM DE PRECO", "SP"},
		{"123", "CIMENTO PORTLAND", "SC25KG", "COTACAO", "32,50"},
	}
	res, err := ParseSINAPIInsumos(grid, "SP")
	if err != nil {
		t.Fatalf("cabeçalho deslocado deveria ser localizado: %v", err)
	}
	if len(res.Insumos) != 1 || res.Insumos[0].Code != "123" {
		t.Errorf("insumos = %+v", res.Insumos)
	}
}

func TestSINAPI_LayoutMudadoFalhaComMensagemClara(t *testing.T) {
	// Falhar alto é melhor que importar lixo: se a Caixa mudar o layout,
	// queremos um erro, não 5.000 produtos com nome de rodapé.
	grid := [][]string{
		{"ALGUMA", "COISA", "TOTALMENTE", "DIFERENTE"},
		{"1", "2", "3", "4"},
	}
	_, err := ParseSINAPIInsumos(grid, "SP")
	if err == nil {
		t.Fatal("layout irreconhecível deveria falhar")
	}
	if !strings.Contains(err.Error(), "layout") && !strings.Contains(err.Error(), "cabeçalho") {
		t.Errorf("mensagem de erro deveria explicar que o layout mudou: %v", err)
	}
}

// ============================================================================
// Composições — a base de conhecimento
// ============================================================================

func TestSINAPI_ComposicoesComCoeficientes(t *testing.T) {
	data := sampleSINAPIWorkbook(t)
	res, err := ParseSINAPIWorkbook(data, "SP", "04/2026", false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(res.Compositions) != 2 {
		t.Fatalf("composições = %d, quer 2: %+v", len(res.Compositions), res.Compositions)
	}

	alvenaria := res.Compositions[0]
	if alvenaria.Code != "87473" {
		t.Errorf("código = %q, quer 87473", alvenaria.Code)
	}
	if alvenaria.Unit != "M2" {
		t.Errorf("unidade = %q, quer M2", alvenaria.Unit)
	}

	// O carry-forward funcionou: as linhas filhas não traziam o código do pai.
	if len(alvenaria.Items) != 4 {
		t.Fatalf("itens da alvenaria = %d, quer 4 (a duplicata deve ser removida): %+v",
			len(alvenaria.Items), alvenaria.Items)
	}

	// O COEFICIENTE é o dado que justifica importar composições.
	var tijolo *SINAPICompositionItem
	for i := range alvenaria.Items {
		if alvenaria.Items[i].Code == "7356" {
			tijolo = &alvenaria.Items[i]
		}
	}
	if tijolo == nil {
		t.Fatal("item tijolo não encontrado na composição")
	}
	if tijolo.Coefficient != 0.0135 {
		t.Errorf("coeficiente do tijolo = %v, quer 0.0135", tijolo.Coefficient)
	}
	if tijolo.Type != "insumo" {
		t.Errorf("tipo = %q, quer insumo", tijolo.Type)
	}

	// Sub-composição (recursiva) distinguida de insumo.
	var temComposicao bool
	for _, it := range alvenaria.Items {
		if it.Type == "composicao" {
			temComposicao = true
		}
	}
	if !temComposicao {
		t.Error("sub-composição deveria ser distinguida de insumo")
	}

	// Coeficiente zero é descartado: item inútil pro cálculo de material.
	chapisco := res.Compositions[1]
	for _, it := range chapisco.Items {
		if it.Coefficient <= 0 {
			t.Errorf("item com coeficiente zero não deveria entrar: %+v", it)
		}
	}
	if len(chapisco.Items) != 2 {
		t.Errorf("itens do chapisco = %d, quer 2", len(chapisco.Items))
	}
}

// ============================================================================
// Mapeamento SINAPI → categorias da Utilar
// ============================================================================

func TestMapSINAPICategory(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"CIMENTO PORTLAND COMPOSTO CP II-32", "construcao"},
		{"AREIA MEDIA - POSTO JAZIDA", "construcao"},
		{"TIJOLO CERAMICO MACICO 5 X 10 X 20CM", "construcao"},
		{"CABO DE COBRE FLEXIVEL ISOLADO, 2,5 MM2", "eletrica"},
		{"DISJUNTOR TIPO DIN, MONOPOLAR 10 ATE 32A", "eletrica"},
		{"TUBO PVC SOLDAVEL PARA AGUA FRIA, DN 25 MM", "hidraulica"},
		{"TORNEIRA CROMADA DE MESA PARA LAVATORIO", "hidraulica"},
		{"TINTA ACRILICA PREMIUM, COR BRANCO FOSCO", "pintura"},
		{"PARAFUSO ZINCADO ROSCA SOBERBA 4,2 X 32 MM", "fixacao"},
		{"FURADEIRA DE IMPACTO 700W", "ferramentas"},
		{"GRAMA ESMERALDA EM PLACAS", "jardim"},
		{"CAPACETE DE SEGURANCA ABA FRONTAL", "seguranca"},
		// Desconhecido cai no balde genérico — recuperável, porque entra
		// como rascunho e a curadoria corrige.
		{"ITEM COMPLETAMENTE DESCONHECIDO XYZ", "construcao"},
	}
	for _, tt := range tests {
		if got := MapSINAPICategory(tt.desc, ""); got != tt.want {
			t.Errorf("MapSINAPICategory(%q) = %q, quer %q", tt.desc, got, tt.want)
		}
	}
}

// A ordem das regras é o que faz o mapeamento funcionar. Estes três casos são
// exatamente os conflitos que a precedência resolve — se alguém reordenar as
// regras, este teste aponta o quê quebrou.
func TestMapSINAPICategory_PrecedenciaResolveConflitos(t *testing.T) {
	// "ELETRODUTO DE PVC" contém "PVC": elétrica tem que vir antes de hidráulica.
	if got := MapSINAPICategory("ELETRODUTO DE PVC RIGIDO ROSCAVEL, DN 25 MM", ""); got != "eletrica" {
		t.Errorf("eletroduto = %q, quer eletrica (regra de elétrica precede a de tubo PVC)", got)
	}
	// "LUVA DE RASPA" é EPI; "LUVA SOLDAVEL" é conexão hidráulica.
	if got := MapSINAPICategory("LUVA DE RASPA CANO CURTO", ""); got != "seguranca" {
		t.Errorf("luva de raspa = %q, quer seguranca (EPI precede conexão)", got)
	}
	if got := MapSINAPICategory("LUVA SOLDAVEL PVC, DN 25 MM", ""); got != "hidraulica" {
		t.Errorf("luva soldável = %q, quer hidraulica", got)
	}
}

func TestMapSINAPICategory_SoUsaCategoriasQueExistem(t *testing.T) {
	// Categoria inventada faria toda linha ser rejeitada por FK no commit.
	validas := map[string]bool{
		"ferramentas": true, "construcao": true, "eletrica": true, "hidraulica": true,
		"pintura": true, "jardim": true, "seguranca": true, "fixacao": true,
	}
	for cat := range SINAPICategoryRules() {
		if !validas[cat] {
			t.Errorf("regra aponta pra categoria %q, que não existe na taxonomia da Utilar", cat)
		}
	}
}

func TestSINAPI_SKUPrefixadoNaoColideComFornecedor(t *testing.T) {
	// O código "1379" do SINAPI e o código "1379" de um fornecedor são coisas
	// diferentes. Sem prefixo, um sobrescreveria o outro no upsert por SKU.
	res := &SINAPIResult{Insumos: []SINAPIInsumo{{Code: "1379", Description: "CIMENTO", Unit: "SC25KG"}}}
	tbl := res.ToTable()
	if got := tbl.Rows[0][0]; got != "SINAPI-1379" {
		t.Errorf("SKU = %q, quer SINAPI-1379", got)
	}
}
