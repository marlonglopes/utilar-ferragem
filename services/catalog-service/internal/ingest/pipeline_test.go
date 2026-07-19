package ingest

// Testes do dry-run: as regras que evitam os desastres conhecidos.
//
// Nenhum destes precisa de Postgres — o Planner fala com a interface `Catalog`.
// Isso é deliberado: um teste de trava de preço que depende de banco no ar é um
// teste que vai ser desligado no primeiro CI vermelho, e é justamente o que
// mais precisa rodar sempre.

import (
	"strings"
	"testing"
)

// fakeCatalog é o catálogo em memória.
type fakeCatalog struct {
	products   map[string]ExistingProduct
	categories map[string]bool
	lookups    int // conta as chamadas: o dry-run tem que ser 1 query, não N
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{
		products: map[string]ExistingProduct{},
		categories: map[string]bool{
			"ferramentas": true, "construcao": true, "eletrica": true,
			"hidraulica": true, "pintura": true, "jardim": true,
			"seguranca": true, "fixacao": true,
		},
	}
}

func (f *fakeCatalog) LookupBySKU(skus []string) (map[string]ExistingProduct, error) {
	f.lookups++
	out := map[string]ExistingProduct{}
	for _, s := range skus {
		if p, ok := f.products[s]; ok {
			out[s] = p
		}
	}
	return out, nil
}

func (f *fakeCatalog) CategoryExists(id string) (bool, error) { return f.categories[id], nil }

func (f *fakeCatalog) ListSupplierSKUs(supplierID string) ([]string, error) {
	var out []string
	for sku, p := range f.products {
		if p.Status != "archived" {
			out = append(out, sku)
		}
	}
	return out, nil
}

// testProfile é o perfil genérico usado na maioria dos testes.
func testProfile() *Profile {
	return &Profile{
		Name: "teste", Version: 1,
		Columns: map[string]ColumnMapping{
			"SKU":       {Field: FieldSKU},
			"NOME":      {Field: FieldName},
			"CATEGORIA": {Field: FieldCategory},
			"PRECO":     {Field: FieldPrice},
			"CUSTO":     {Field: FieldCost},
			"ESTOQUE":   {Field: FieldStock},
			"UNIDADE":   {Field: FieldUnitOfMeasure},
		},
	}
}

func planCSV(t *testing.T, cat Catalog, prof *Profile, csv string) *Plan {
	t.Helper()
	tbl, err := Read("teste.csv", []byte(csv), "", 0)
	if err != nil {
		t.Fatalf("leitura: %v", err)
	}
	p := &Planner{Profile: prof, Catalog: cat}
	plan, err := p.Plan(tbl)
	if err != nil {
		t.Fatalf("plano: %v", err)
	}
	return plan
}

// ============================================================================
// REGRA: linha inválida NÃO aborta o lote
// ============================================================================
func TestDryRun_LinhaInvalidaNaoAbortaOLote(t *testing.T) {
	cat := newFakeCatalog()
	csv := `SKU,NOME,CATEGORIA,PRECO,ESTOQUE
P-1,Parafuso 3/8,fixacao,"R$ 1,50",100
P-2,Bucha 8mm,fixacao,"R$ 0,45",500
,Sem SKU,fixacao,"R$ 9,90",10
P-4,Prego 18x30,categoria-que-nao-existe,"R$ 12,00",80
P-5,Arruela,fixacao,"R$ -3,00",10
P-6,Porca M8,fixacao,"R$ 0,80",300
`
	plan := planCSV(t, cat, testProfile(), csv)

	if plan.Total != 6 {
		t.Fatalf("total = %d, quer 6", plan.Total)
	}
	// 3 boas (P-1, P-2, P-6) + 3 rejeitadas.
	if plan.Creates != 3 {
		t.Errorf("creates = %d, quer 3 — linhas boas devem passar apesar das ruins", plan.Creates)
	}
	if plan.Rejects != 3 {
		t.Errorf("rejects = %d, quer 3", plan.Rejects)
	}

	// Cada rejeição precisa apontar a LINHA e o MOTIVO — senão o operador não
	// consegue achar o problema na planilha dele.
	for _, r := range plan.Rows {
		if r.Action != ActionReject {
			continue
		}
		if r.RowNumber == 0 {
			t.Error("linha rejeitada sem número de linha")
		}
		if len(r.Errors) == 0 {
			t.Errorf("linha %d rejeitada sem motivo", r.RowNumber)
		}
	}
}

func TestDryRun_SemSKUERejeitada(t *testing.T) {
	// O doc é explícito: "Sem SKU, a linha é rejeitada — não adivinhamos".
	// Casar por nome criaria duplicata quando o fornecedor mudar a grafia.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\n,Cimento CP-II,construcao,\"R$ 42,90\"\n")

	if plan.Rejects != 1 {
		t.Fatalf("linha sem SKU deveria ser rejeitada; plano=%+v", plan)
	}
	if !strings.Contains(strings.ToLower(plan.Rows[0].Errors[0].Message), "sku") {
		t.Errorf("motivo da rejeição deveria citar SKU: %v", plan.Rows[0].Errors)
	}
}

// ============================================================================
// REGRA: idempotência — rodar duas vezes dá o mesmo resultado
// ============================================================================
func TestDryRun_Idempotente(t *testing.T) {
	cat := newFakeCatalog()
	csv := "SKU,NOME,CATEGORIA,PRECO,ESTOQUE,UNIDADE\nCIM-50,Cimento CP-II 50kg,construcao,\"R$ 42,90\",120,sc\n"

	// 1ª passada: cria.
	plan1 := planCSV(t, cat, testProfile(), csv)
	if plan1.Creates != 1 {
		t.Fatalf("1ª passada: creates = %d, quer 1", plan1.Creates)
	}

	// Simula o commit.
	cat.products["CIM-50"] = ExistingProduct{
		ID: "id-1", SKU: "CIM-50", Name: "Cimento CP-II 50kg",
		Price: 42.90, Stock: 120, Status: "draft", UnitOfMeasure: "sc",
	}

	// 2ª passada com o MESMO arquivo: nada mudou → skip, não update.
	plan2 := planCSV(t, cat, testProfile(), csv)
	if plan2.Skips != 1 {
		t.Errorf("2ª passada: skips = %d, quer 1 (idempotência); creates=%d updates=%d",
			plan2.Skips, plan2.Creates, plan2.Updates)
	}
	if plan2.Updates != 0 {
		t.Errorf("2ª passada não deveria gerar UPDATE — poluiria a auditoria com mudanças vazias")
	}

	// 3ª passada com preço alterado: agora é update.
	csv3 := "SKU,NOME,CATEGORIA,PRECO,ESTOQUE,UNIDADE\nCIM-50,Cimento CP-II 50kg,construcao,\"R$ 44,90\",120,sc\n"
	plan3 := planCSV(t, cat, testProfile(), csv3)
	if plan3.Updates != 1 {
		t.Errorf("mudança real de preço deveria gerar update, veio %+v", plan3)
	}
}

func TestDryRun_UmaConsultaParaOLoteInteiro(t *testing.T) {
	cat := newFakeCatalog()
	var b strings.Builder
	b.WriteString("SKU,NOME,CATEGORIA,PRECO\n")
	for i := 0; i < 500; i++ {
		b.WriteString("SKU-x,Produto,fixacao,\"R$ 1,00\"\n")
	}
	planCSV(t, cat, testProfile(), b.String())

	if cat.lookups != 1 {
		t.Errorf("lookups = %d, quer 1 — uma query por linha faria o dry-run "+
			"de 4.000 linhas levar minutos", cat.lookups)
	}
}

// ============================================================================
// REGRA: queda de preço acima do limite segura para revisão
// ============================================================================
func TestDryRun_TravaDeQuedaDePreco(t *testing.T) {
	cat := newFakeCatalog()
	cat.products["CIM-50"] = ExistingProduct{
		ID: "id-1", SKU: "CIM-50", Name: "Cimento", Price: 1234.56, Status: "published",
	}

	// O erro de vírgula clássico: "1.234,56" digitado como "1,23".
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\nCIM-50,Cimento,construcao,\"R$ 1,23\"\n")

	if plan.Reviews != 1 {
		t.Fatalf("queda de 99,9%% deveria ser retida para revisão; plano = %+v", plan)
	}
	r := plan.Rows[0]
	if r.Action != ActionReview {
		t.Errorf("ação = %q, quer 'review'", r.Action)
	}
	if r.DropPct < 99 {
		t.Errorf("dropPct = %.1f, esperado ~99,9", r.DropPct)
	}
	if len(r.Warnings) == 0 {
		t.Error("linha retida deveria explicar o motivo ao operador")
	}
}

func TestDryRun_QuedaPequenaPassaSemRevisao(t *testing.T) {
	cat := newFakeCatalog()
	cat.products["P-1"] = ExistingProduct{ID: "1", SKU: "P-1", Name: "Parafuso", Price: 10.00}

	// -10%: promoção normal, não pode travar o fluxo do lojista.
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 9,00\"\n")

	if plan.Updates != 1 || plan.Reviews != 0 {
		t.Errorf("queda de 10%% deveria passar direto; plano = %+v", plan)
	}
}

func TestDryRun_AltaAbsurdaTambemERetida(t *testing.T) {
	cat := newFakeCatalog()
	cat.products["P-1"] = ExistingProduct{ID: "1", SKU: "P-1", Name: "Parafuso", Price: 1.00}

	// O mesmo bug ao contrário, ou custo mapeado na coluna de preço.
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 900,00\"\n")

	if plan.Reviews != 1 {
		t.Errorf("alta de 90.000%% deveria ser retida; plano = %+v", plan)
	}
}

func TestDryRun_LimiteDeQuedaConfiguravel(t *testing.T) {
	cat := newFakeCatalog()
	cat.products["P-1"] = ExistingProduct{ID: "1", SKU: "P-1", Name: "P", Price: 100.00}

	prof := testProfile()
	prof.Options.MaxPriceDropPct = 5 // fornecedor conservador

	plan := planCSV(t, cat, prof, "SKU,NOME,CATEGORIA,PRECO\nP-1,P,fixacao,\"R$ 90,00\"\n")
	if plan.Reviews != 1 {
		t.Errorf("queda de 10%% com limite de 5%% deveria reter; plano = %+v", plan)
	}
}

func TestOptions_LimitePadraoSeguroQuandoOmitido(t *testing.T) {
	// Perfil que não configura nada NÃO pode ficar sem trava.
	var o Options
	if o.dropLimit() != DefaultMaxPriceDropPct {
		t.Errorf("limite de queda padrão = %v, quer %v", o.dropLimit(), DefaultMaxPriceDropPct)
	}
	if o.riseLimit() != DefaultMaxPriceRisePct {
		t.Errorf("limite de alta padrão = %v", o.riseLimit())
	}
}

// ============================================================================
// REGRA: nunca apagar por ausência — vira `archived`
// ============================================================================
func TestDryRun_ArquivaAusentesNuncaApaga(t *testing.T) {
	cat := newFakeCatalog()
	cat.products["P-1"] = ExistingProduct{ID: "1", SKU: "P-1", Name: "Parafuso", Price: 1, Status: "published"}
	cat.products["P-2"] = ExistingProduct{ID: "2", SKU: "P-2", Name: "Prego", Price: 2, Status: "published"}
	cat.products["P-3"] = ExistingProduct{ID: "3", SKU: "P-3", Name: "Bucha", Price: 3, Status: "published"}

	prof := testProfile()
	prof.Options.ArchiveMissing = true

	tbl, err := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 1,00\"\n"), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// SupplierID é obrigatório: sem escopo, arquivar por ausência atingiria o
	// catálogo inteiro de outros fornecedores.
	planner := &Planner{Profile: prof, Catalog: cat, SupplierID: "forn-1"}
	plan, err := planner.Plan(tbl)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.MissingSKUs) != 2 {
		t.Fatalf("missingSKUs = %v, quer [P-2 P-3]", plan.MissingSKUs)
	}
	if plan.MissingSKUs[0] != "P-2" || plan.MissingSKUs[1] != "P-3" {
		t.Errorf("missingSKUs = %v (ordem determinística esperada)", plan.MissingSKUs)
	}
}

func TestDryRun_NaoArquivaQuandoDesligado(t *testing.T) {
	// PADRÃO SEGURO: fornecedor manda planilha parcial o tempo todo. Sem esta
	// regra, o catálogo inteiro evaporaria numa importação parcial.
	cat := newFakeCatalog()
	cat.products["P-2"] = ExistingProduct{ID: "2", SKU: "P-2", Name: "Prego", Price: 2, Status: "published"}

	tbl, _ := Read("x.csv", []byte("SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 1,00\"\n"), "", 0)
	planner := &Planner{Profile: testProfile(), Catalog: cat, SupplierID: "forn-1"}
	plan, _ := planner.Plan(tbl)

	if len(plan.MissingSKUs) != 0 {
		t.Errorf("sem archiveMissing, nada deveria ser arquivado; veio %v", plan.MissingSKUs)
	}
}

// ============================================================================
// Outras regras
// ============================================================================

func TestDryRun_SKUDuplicadoNoArquivo(t *testing.T) {
	// Sem detectar, a segunda ocorrência sobrescreve a primeira em silêncio e
	// ninguém sabe qual preço venceu.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 1,00\"\nP-1,Parafuso B,fixacao,\"R$ 2,00\"\n")

	if plan.Rejects != 1 || plan.Creates != 1 {
		t.Fatalf("duplicata deveria rejeitar a 2ª linha; plano = %+v", plan)
	}
	var found bool
	for _, r := range plan.Rows {
		if r.Action == ActionReject {
			for _, e := range r.Errors {
				if strings.Contains(e.Message, "duplicado") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("erro deveria explicar que é SKU duplicado e apontar a linha original")
	}
}

func TestDryRun_StagingPreservaLinhaCrua(t *testing.T) {
	// "De onde veio esse preço?" é a pergunta que aparece três meses depois.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 1.234,56\"\n")

	raw := plan.Rows[0].Raw
	if raw["PRECO"] != "R$ 1.234,56" {
		t.Errorf("linha crua não preservada como veio: %q", raw["PRECO"])
	}
	if plan.Rows[0].Mapped["price"] != 1234.56 {
		t.Errorf("valor mapeado = %v, quer 1234.56", plan.Rows[0].Mapped["price"])
	}
}

func TestDryRun_ColunaDoPerfilAusenteAvisaNoLote(t *testing.T) {
	// O fornecedor renomeou a coluna: a importação roda inteira e não atualiza
	// preço nenhum. Sem aviso, ninguém percebe.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA\nP-1,Parafuso,fixacao\n")

	if len(plan.Warnings) == 0 {
		t.Fatal("colunas do perfil ausentes do arquivo deveriam gerar aviso de lote")
	}
	joined := strings.Join(plan.Warnings, " ")
	if !strings.Contains(joined, "PRECO") {
		t.Errorf("aviso deveria citar a coluna ausente; veio: %v", plan.Warnings)
	}
}

func TestDryRun_UnidadeDesconhecidaViraUnComAviso(t *testing.T) {
	// Rejeitar aqui reprovaria metade do catálogo pela grafia do fornecedor.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,UNIDADE\nP-1,Parafuso,fixacao,\"R$ 1,00\",xyz\n")

	r := plan.Rows[0]
	if r.Action == ActionReject {
		t.Fatal("unidade desconhecida não deveria rejeitar a linha")
	}
	if r.Mapped["unitOfMeasure"] != "un" {
		t.Errorf("unidade = %v, quer 'un'", r.Mapped["unitOfMeasure"])
	}
	if len(r.Warnings) == 0 {
		t.Error("assumir 'un' deveria ser avisado, não silencioso")
	}
}

func TestDryRun_MaoDeObraNaoEProduto(t *testing.T) {
	// O SINAPI mistura pedreiro (H) e locação (MES) com cimento e areia.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,UNIDADE\nS-1,Servente com encargos,construcao,\"R$ 20,00\",H\n")

	if plan.Rejects != 1 {
		t.Errorf("item com unidade de mão de obra não deveria virar produto; plano = %+v", plan)
	}
}

func TestDryRun_ToneladaConverteParaQuiloComPrecoAjustado(t *testing.T) {
	// "2 T" virando "2 kg" é erro de mil vezes, silencioso, descoberto só na
	// primeira venda.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,ESTOQUE,UNIDADE\nACO-1,Vergalhao CA-50,construcao,\"R$ 5.000,00\",2,T\n")

	r := plan.Rows[0]
	if r.Mapped["unitOfMeasure"] != "kg" {
		t.Errorf("unidade = %v, quer 'kg'", r.Mapped["unitOfMeasure"])
	}
	if st, _ := r.Mapped["stock"].(float64); st != 2000 {
		t.Errorf("estoque = %v, quer 2000 (2 T = 2000 kg)", st)
	}
	if pr, _ := r.Mapped["price"].(float64); pr != 5.00 {
		t.Errorf("preço = %v, quer 5.00 (R$ 5.000/T = R$ 5/kg)", pr)
	}
}

func TestDryRun_CodigoDeBarrasRuimNaoInvalidaOProduto(t *testing.T) {
	// O item ainda vende; só não é lido por scanner.
	cat := newFakeCatalog()
	prof := testProfile()
	prof.Columns["EAN"] = ColumnMapping{Field: FieldBarcode}

	tbl, _ := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,PRECO,EAN\nP-1,Parafuso,fixacao,\"R$ 1,00\",7.89123E+12\n"), "", 0)
	planner := &Planner{Profile: prof, Catalog: cat}
	plan, _ := planner.Plan(tbl)

	r := plan.Rows[0]
	if r.Action == ActionReject {
		t.Fatal("EAN destruído pelo Excel não deveria rejeitar o produto")
	}
	if len(r.Warnings) == 0 {
		t.Error("EAN suspeito deveria avisar")
	}
}

func TestDryRun_ProdutoNovoSemPrecoAvisaEEntraComoRascunho(t *testing.T) {
	// É o caso do SINAPI: só custo de referência, sem preço de venda.
	cat := newFakeCatalog()
	prof := testProfile()
	delete(prof.Columns, "PRECO")

	tbl, _ := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,CUSTO\nS-1,Cimento,construcao,\"R$ 30,00\"\n"), "", 0)
	planner := &Planner{Profile: prof, Catalog: cat}
	plan, _ := planner.Plan(tbl)

	r := plan.Rows[0]
	if r.Action != ActionCreate {
		t.Fatalf("ação = %q, quer create", r.Action)
	}
	var warned bool
	for _, w := range r.Warnings {
		if strings.Contains(w.Message, "precificação") || strings.Contains(w.Message, "preço de venda") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("produto sem preço de venda deveria avisar sobre precificação; avisos: %v", r.Warnings)
	}
}

// ============================================================================
// REGRA: preço ausente/zero e preço abaixo do custo são RETIDOS, não aceitos
//
// Estes dois furos foram encontrados rodando uma planilha real pelo dry-run: as
// duas linhas voltaram com `create` e teriam ido para a vitrine. Reter (e não
// rejeitar) é deliberado — o dado pode estar certo, e recusar a linha faz o
// operador perder o produto sem entender o motivo.
// ============================================================================

func TestDryRun_PrecoAusenteEmCriacaoERetidoNaoCriado(t *testing.T) {
	// REGRESSÃO: a linha voltava `create` com price=null e o produto nascia
	// valendo R$ 0,00 no catálogo.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,CUSTO,ESTOQUE\nP-1,Produto sem preço nenhum,ferramentas,,,10\n")

	if plan.Creates != 0 {
		t.Fatalf("produto novo sem preço NÃO pode ser criado em silêncio; plano = %+v", plan)
	}
	if plan.Reviews != 1 {
		t.Fatalf("reviews = %d, quer 1 — a decisão é humana, não do importador; plano = %+v",
			plan.Reviews, plan)
	}
	if plan.Rejects != 0 {
		t.Errorf("rejeitar seria errado: o operador perderia o produto sem entender; plano = %+v", plan)
	}
}

func TestDryRun_PrecoZeroEmCriacaoERetido(t *testing.T) {
	// Célula preenchida com 0 é diferente de célula vazia, mas o resultado no
	// catálogo é o mesmo: item na vitrine valendo nada.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,ESTOQUE\nP-1,Parafuso,fixacao,\"R$ 0,00\",10\n")

	if plan.Reviews != 1 {
		t.Fatalf("preço zero deveria ser retido; plano = %+v", plan)
	}
	if !strings.Contains(strings.Join(warningMessages(plan.Rows[0]), " "), "R$ 0,00") {
		t.Errorf("a mensagem deveria dizer o valor lido; avisos: %v", plan.Rows[0].Warnings)
	}
}

func TestDryRun_PrecoAbaixoDoCustoERetido(t *testing.T) {
	// REGRESSÃO: "Cimento CP-V ARI saco 50kg" a R$ 1,23 com custo R$ 31,40 —
	// o erro de vírgula ("123,40" → "1,23") na PRIMEIRA importação do item, onde
	// a trava de queda percentual não enxerga nada porque não há preço anterior.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,CUSTO,ESTOQUE\nCIM-V,Cimento CP-V ARI saco 50kg,construcao,\"R$ 1,23\",\"R$ 31,40\",80\n")

	if plan.Creates != 0 || plan.Reviews != 1 {
		t.Fatalf("preço abaixo do custo deveria ser retido, não criado; plano = %+v", plan)
	}
	if plan.Rejects != 0 {
		t.Errorf("preço abaixo do custo pode ser legítimo (item de isca) — reter, nunca rejeitar")
	}
}

func TestDryRun_MensagemDePrecoAbaixoDoCustoDizOsDoisValoresEALinha(t *testing.T) {
	// Quem lê é o comprador da loja: sem o valor lido, o valor comparado e a
	// linha da planilha, a mensagem não é acionável e vira ruído ignorado.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,CUSTO\nCIM-V,Cimento,construcao,\"R$ 1,23\",\"R$ 31,40\"\n")

	r := plan.Rows[0]
	msg := strings.Join(warningMessages(r), " ")
	for _, want := range []string{"1,23", "31,40", "linha 2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("mensagem sem %q — o comprador não consegue agir sobre ela: %q", want, msg)
		}
	}
	if r.RowNumber != 2 {
		t.Errorf("rowNumber = %d, quer 2 (a linha da planilha, com cabeçalho)", r.RowNumber)
	}
}

func TestDryRun_PrecoAbaixoDoCustoPermitidoPorPerfil(t *testing.T) {
	// O caso legítimo: loja que vende item de isca no prejuízo de propósito.
	// Sem esta saída, a trava seria contornada desligando a validação inteira.
	cat := newFakeCatalog()
	prof := testProfile()
	prof.Options.AllowPriceBelowCost = true

	plan := planCSV(t, cat, prof,
		"SKU,NOME,CATEGORIA,PRECO,CUSTO\nISCA-1,Furadeira de isca,ferramentas,\"R$ 99,00\",\"R$ 140,00\"\n")

	if plan.Creates != 1 || plan.Reviews != 0 {
		t.Errorf("com allowPriceBelowCost o item de isca deve passar direto; plano = %+v", plan)
	}
}

func TestDryRun_PrecoAbaixoDoCustoEmAtualizacaoUsaOCustoCadastrado(t *testing.T) {
	// A planilha que só atualiza preço não traz custo — e é justamente a que
	// derruba a margem sem ninguém ver. A queda de 20% cabe dentro do limite de
	// 30%, então só a comparação com o custo pega este caso.
	cat := newFakeCatalog()
	cost := 90.00
	cat.products["P-1"] = ExistingProduct{
		ID: "1", SKU: "P-1", Name: "Furadeira", Price: 100.00, Cost: &cost, Status: "published",
	}
	prof := testProfile()
	delete(prof.Columns, "CUSTO")

	tbl, _ := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Furadeira,ferramentas,\"R$ 80,00\"\n"), "", 0)
	planner := &Planner{Profile: prof, Catalog: cat}
	plan, err := planner.Plan(tbl)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Reviews != 1 {
		t.Fatalf("preço abaixo do custo cadastrado deveria reter mesmo dentro do limite de queda; plano = %+v", plan)
	}
	if !strings.Contains(strings.Join(warningMessages(plan.Rows[0]), " "), "90,00") {
		t.Errorf("mensagem deveria citar o custo cadastrado; avisos: %v", plan.Rows[0].Warnings)
	}
}

func TestDryRun_MargemZeroNaoERetida(t *testing.T) {
	// Falso positivo é pior que o bug: se a trava reter demais, o operador
	// aprende a ignorá-la. Vender pelo custo é decisão comercial banal.
	cat := newFakeCatalog()
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,CUSTO\nP-1,Parafuso,fixacao,\"R$ 10,00\",\"R$ 10,00\"\n")

	if plan.Creates != 1 || plan.Reviews != 0 {
		t.Errorf("preço igual ao custo não é erro de vírgula; plano = %+v", plan)
	}
}

func TestDryRun_SemColunaDeCustoNaoRetemNada(t *testing.T) {
	// Custo desconhecido não é sinal de nada. Reter por desconhecimento seguraria
	// o catálogo inteiro de todo fornecedor que não manda coluna de custo.
	cat := newFakeCatalog()
	prof := testProfile()
	delete(prof.Columns, "CUSTO")

	tbl, _ := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,PRECO\nP-1,Parafuso,fixacao,\"R$ 1,00\"\n"), "", 0)
	planner := &Planner{Profile: prof, Catalog: cat}
	plan, _ := planner.Plan(tbl)

	if plan.Creates != 1 || plan.Reviews != 0 {
		t.Errorf("sem custo não há comparação a fazer; plano = %+v", plan)
	}
}

func TestDryRun_PlanilhaSemColunaDePrecoNaoRetemOLoteInteiro(t *testing.T) {
	// REGRESSÃO ao contrário: o SINAPI só traz CUSTO de referência. Reter linha a
	// linha mandaria 4.000 itens para revisão e ensinaria o operador a aprovar
	// tudo no automático — pior que o bug que a trava conserta.
	cat := newFakeCatalog()
	prof := testProfile()
	delete(prof.Columns, "PRECO")

	tbl, _ := Read("x.csv", []byte(
		"SKU,NOME,CATEGORIA,CUSTO\nS-1,Cimento,construcao,\"R$ 30,00\"\nS-2,Areia,construcao,\"R$ 80,00\"\n"), "", 0)
	planner := &Planner{Profile: prof, Catalog: cat}
	plan, _ := planner.Plan(tbl)

	if plan.Creates != 2 || plan.Reviews != 0 {
		t.Errorf("planilha só de custo não pode reter o lote inteiro; plano = %+v", plan)
	}
}

func TestDryRun_ColunaDePrecoVaziaNaoZeraOPrecoExistente(t *testing.T) {
	// Mesmo princípio de TestImport_NaoApagaCustoQuandoAPlanilhaNaoTrazAColuna:
	// ausência significa "não sei", nunca "zere". Uma planilha que só atualiza
	// estoque não pode derrubar o preço de quem já está na vitrine.
	cat := newFakeCatalog()
	cat.products["P-1"] = ExistingProduct{
		ID: "1", SKU: "P-1", Name: "Parafuso", Price: 42.90, Stock: 10, Status: "published",
	}
	plan := planCSV(t, cat, testProfile(),
		"SKU,NOME,CATEGORIA,PRECO,ESTOQUE\nP-1,Parafuso,fixacao,,99\n")

	r := plan.Rows[0]
	if r.Action != ActionUpdate {
		t.Fatalf("ação = %q, quer update (só o estoque mudou); plano = %+v", r.Action, plan)
	}
	if _, ok := r.Mapped["price"]; ok {
		t.Errorf("preço ausente na planilha não pode entrar no UPDATE: mapped[price] = %v", r.Mapped["price"])
	}
}

func warningMessages(r RowResult) []string {
	out := make([]string, 0, len(r.Warnings))
	for _, w := range r.Warnings {
		out = append(out, w.Message)
	}
	return out
}

func TestSuggestMapping_SugereMasNaoDecide(t *testing.T) {
	header := []string{"CODIGO", "DESCRICAO DO PRODUTO", "VLR VENDA", "ESTOQUE", "UNIDADE"}
	m := SuggestMapping(header)

	if m["CODIGO"].Field != FieldSKU {
		t.Errorf("CODIGO → %v, quer sku", m["CODIGO"].Field)
	}
	if m["VLR VENDA"].Field != FieldPrice {
		t.Errorf("VLR VENDA → %v, quer price", m["VLR VENDA"].Field)
	}
	if m["ESTOQUE"].Field != FieldStock {
		t.Errorf("ESTOQUE → %v, quer stock", m["ESTOQUE"].Field)
	}
}

func TestSuggestMapping_Deterministico(t *testing.T) {
	// Iteração de mapa em Go é aleatória: sem ordenação, a sugestão mudaria a
	// cada chamada e o operador veria mapeamentos diferentes ao recarregar.
	header := []string{"CODIGO", "DESCRICAO", "PRECO", "CUSTO", "ESTOQUE"}
	first := SuggestMapping(header)
	for i := 0; i < 30; i++ {
		got := SuggestMapping(header)
		for k, v := range first {
			if got[k].Field != v.Field {
				t.Fatalf("sugestão não-determinística em %q: %v vs %v", k, v.Field, got[k].Field)
			}
		}
	}
}

// ============================================================================
// Inferência de colunas: os sinônimos que aparecem de verdade em planilha de
// fornecedor brasileiro, e o grau de confiança que a tela usa pra destacar o
// que precisa de conferência humana.
// ============================================================================

func TestSuggestColumns_SinonimosDeFornecedorBrasileiro(t *testing.T) {
	// Cabeçalhos reais: acento inconsistente, caixa alta, pontuação, espaço
	// sobrando. Comparar a string crua não reconheceria nada.
	cases := []struct {
		header string
		want   Field
	}{
		{"SKU", FieldSKU},
		{"CÓDIGO", FieldSKU},
		{"REFERENCIA", FieldSKU},
		{"DESCRICAO DO PRODUTO", FieldName},
		{"Descrição do Produto", FieldName},
		{"VLR VENDA", FieldPrice},
		{"PREÇO VENDA", FieldPrice},
		{"VLR CUSTO", FieldCost},
		{"CUSTO", FieldCost},
		{"QTD", FieldStock},
		{"SALDO", FieldStock},
		{"QUANTIDADE", FieldStock},
		{"GRUPO", FieldCategory},
		{"DEPARTAMENTO", FieldCategory},
		{"FABRICANTE", FieldBrand},
		{"MARCA", FieldBrand},
		{"UNID", FieldUnitOfMeasure},
		{"CODIGO DE BARRAS", FieldBarcode},
		{"GTIN", FieldBarcode},
		{"NCM", FieldNCM},
		{"PESO", FieldWeightKg},
	}

	for _, tc := range cases {
		// Uma coluna por vez: campos são exclusivos entre si, e um cabeçalho
		// único isola o reconhecimento do sinônimo da disputa por campo.
		got := SuggestColumns([]string{tc.header})
		if len(got) != 1 {
			t.Fatalf("%q: esperado 1 sugestão, veio %d", tc.header, len(got))
		}
		if !got[0].Recognized || got[0].Field != tc.want {
			t.Errorf("%q → %v (reconhecida=%v), quer %v",
				tc.header, got[0].Field, got[0].Recognized, tc.want)
		}
		if got[0].Confidence == "" {
			t.Errorf("%q: sugestão sem grau de confiança — a tela não consegue "+
				"destacar o que precisa de conferência", tc.header)
		}
	}
}

func TestSuggestColumns_ColunaDesconhecidaNaoEErro(t *testing.T) {
	// Fornecedor sempre manda coluna interna. Reprovar o arquivo por causa dela
	// tornaria o importador inutilizável — ela fica sem mapear e é ignorada.
	header := []string{"SKU", "NOME", "COD_ERP_ANTIGO", "XYZ_INTERNO_9", "PRECO"}
	got := SuggestColumns(header)

	if len(got) != len(header) {
		t.Fatalf("SuggestColumns tem que devolver UMA entrada por coluna do "+
			"arquivo (inclusive as ignoradas): %d de %d", len(got), len(header))
	}

	byCol := map[string]Suggestion{}
	for _, s := range got {
		byCol[s.Column] = s
	}
	for _, col := range []string{"COD_ERP_ANTIGO", "XYZ_INTERNO_9"} {
		if byCol[col].Recognized {
			t.Errorf("%q não deveria ser reconhecida, veio %v", col, byCol[col].Field)
		}
	}
	// E as reconhecidas continuam reconhecidas: a coluna estranha no meio não
	// pode atrapalhar as outras.
	if byCol["SKU"].Field != FieldSKU || byCol["PRECO"].Field != FieldPrice {
		t.Errorf("coluna desconhecida atrapalhou o resto: %+v", byCol)
	}

	// E o perfil derivado só contém o que foi reconhecido.
	m := SuggestMapping(header)
	if _, ok := m["COD_ERP_ANTIGO"]; ok {
		t.Error("coluna não reconhecida vazou para o mapeamento do perfil")
	}
}

func TestSuggestColumns_ConfiancaGraduada(t *testing.T) {
	// Casamento exato tem que valer mais que casamento por conteúdo: é isso que
	// permite à tela pedir conferência só onde o palpite é frágil.
	got := SuggestColumns([]string{"PRECO", "VLR VENDA UNITARIO DO ITEM"})
	byCol := map[string]Suggestion{}
	for _, s := range got {
		byCol[s.Column] = s
	}
	if byCol["PRECO"].Confidence != ConfidenceExact {
		t.Errorf("PRECO (alias exato) → confiança %q, quer %q",
			byCol["PRECO"].Confidence, ConfidenceExact)
	}
	if c := byCol["VLR VENDA UNITARIO DO ITEM"].Confidence; c == ConfidenceExact {
		t.Errorf("cabeçalho que apenas CONTÉM o alias não pode ter confiança "+
			"'exact' — veio %q", c)
	}
	// A explicação do palpite tem que existir: "por que ele achou que isso é
	// preço?" precisa ser respondível na própria tela.
	if byCol["PRECO"].MatchedAlias == "" {
		t.Error("sugestão sem MatchedAlias — o humano não tem como confirmar de forma informada")
	}
}

func TestSuggestColumns_CabecalhoGenericoNaoEscorregaParaCampoVizinho(t *testing.T) {
	// REGRESSÃO. "VALOR UNITARIO" é alias exato de `price` e reivindica o campo.
	// Sobra "PRECO", cujo campo natural já foi tomado. O casamento fraco por
	// prefixo aceitava a direção inversa ("preco" é prefixo de "preco de
	// custo") e mapeava PRECO → cost: o preço de venda gravado como CUSTO, em
	// silêncio, sob o rótulo mais óbvio da planilha.
	//
	// O certo é ficar SEM MAPEAR e deixar o humano resolver na tela.
	got := SuggestColumns([]string{"VALOR UNITARIO", "PRECO"})
	for _, s := range got {
		if s.Column == "PRECO" && s.Recognized && s.Field != FieldPrice {
			t.Fatalf("REGRESSÃO CRÍTICA: coluna %q mapeada em %q — um cabeçalho "+
				"genérico nunca pode escorregar para um campo de dinheiro vizinho",
				s.Column, s.Field)
		}
	}
}

func TestSuggestColumns_AbreviacaoColadaAindaCasa(t *testing.T) {
	// A direção segura do casamento fraco: o cabeçalho é uma grafia mais
	// específica do alias, colada sem separador.
	got := SuggestColumns([]string{"PRECOVENDA"})
	if !got[0].Recognized || got[0].Field != FieldPrice {
		t.Errorf("PRECOVENDA → %v (reconhecida=%v), quer price", got[0].Field, got[0].Recognized)
	}
	if got[0].Confidence == ConfidenceExact {
		t.Error("casamento por abreviação não pode se apresentar como 'exact'")
	}
}

func TestSuggestColumns_NaoSugereSpecs(t *testing.T) {
	// Mapear para specs exige um specKey que só o humano escolhe; sugerir isso
	// produziria um perfil que falha em Validate().
	for _, s := range SuggestColumns([]string{"FICHA TECNICA", "ATRIBUTOS"}) {
		if s.Field == FieldSpecs {
			t.Errorf("coluna %q sugerida como specs sem specKey — o perfil "+
				"resultante seria inválido", s.Column)
		}
	}
}

func TestSuggestMapping_PerfilSugeridoEValido(t *testing.T) {
	// O ponto do mapeamento automático: o que ele propõe tem que ser aceitável
	// como perfil. Sugestão que não passa em Validate() não economiza trabalho
	// nenhum do operador.
	header := []string{"CODIGO", "DESCRICAO DO PRODUTO", "VLR VENDA", "VLR CUSTO",
		"ESTOQUE", "UNIDADE", "GRUPO", "FABRICANTE", "COLUNA_ESTRANHA"}

	p := &Profile{Name: "fornecedor-teste", Columns: SuggestMapping(header)}
	if err := p.Validate(); err != nil {
		t.Fatalf("perfil sugerido automaticamente é inválido: %v (colunas: %+v)", err, p.Columns)
	}
	if p.Columns["VLR VENDA"].Field != FieldPrice {
		t.Errorf("VLR VENDA → %v, quer price", p.Columns["VLR VENDA"].Field)
	}
	if p.Columns["VLR CUSTO"].Field != FieldCost {
		t.Errorf("VLR CUSTO → %v, quer cost (mapear custo em preço é o desastre nº 1)",
			p.Columns["VLR CUSTO"].Field)
	}
}
