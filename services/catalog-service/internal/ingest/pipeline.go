package ingest

// O PIPELINE: mapeamento → validação por linha → dry-run.
//
// Este arquivo NÃO escreve no banco. Ele produz um PLANO: o que aconteceria se
// o lote fosse aplicado. É a separação que torna o dry-run possível de verdade
// (e não uma simulação que diverge do commit real): o commit consome exatamente
// este plano, não recalcula nada.
//
// A regra que atravessa tudo aqui: LINHA INVÁLIDA NÃO ABORTA O LOTE. 4.000
// linhas com a 37ª quebrada tem que resultar em 3.999 produtos e um relatório
// apontando a linha 37. Qualquer `return err` que interrompa o laço de linhas
// é um bug — os erros vivem DENTRO da linha (`RowResult.Errors`).

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Action é o veredito do dry-run pra uma linha.
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionSkip   Action = "skip"   // idempotência: nada mudou
	ActionReview Action = "review" // precisa de olho humano (queda de preço)
	ActionReject Action = "reject" // inválida
)

// RowError é um problema em um campo de uma linha.
type RowError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// RowResult é o destino de uma linha da planilha.
type RowResult struct {
	RowNumber int               `json:"rowNumber"`
	Raw       map[string]string `json:"raw"`
	Mapped    map[string]any    `json:"mapped"`
	SKU       string            `json:"sku,omitempty"`
	Action    Action            `json:"action"`
	Errors    []RowError        `json:"errors"`
	Warnings  []RowError        `json:"warnings"`
	ProductID string            `json:"productId,omitempty"`

	// Diff de preço — o que a tela de revisão mostra em âmbar.
	OldPrice  *float64 `json:"oldPrice,omitempty"`
	NewPrice  *float64 `json:"newPrice,omitempty"`
	DropPct   float64  `json:"dropPct,omitempty"`
}

func (r *RowResult) addError(field, msg string) {
	r.Errors = append(r.Errors, RowError{Field: field, Message: msg})
}

func (r *RowResult) addWarning(field, msg string) {
	r.Warnings = append(r.Warnings, RowError{Field: field, Message: msg})
}

// ExistingProduct é o estado atual de um produto, pro dry-run comparar.
type ExistingProduct struct {
	ID            string
	SKU           string
	Name          string
	Price         float64
	Cost          *float64
	Stock         float64
	Status        string
	Source        string
	UnitOfMeasure string
	Brand         string
	Description   string
	Barcode       string
}

// Catalog é o que o planner precisa saber do banco. Interface (e não *sql.DB)
// porque o dry-run é a lógica mais densa do pipeline e precisa ser testável em
// teste unitário, sem Postgres — um teste de trava de preço que depende de
// banco no ar é um teste que vai ser desligado no primeiro CI vermelho.
type Catalog interface {
	LookupBySKU(skus []string) (map[string]ExistingProduct, error)
	CategoryExists(id string) (bool, error)
	// ListSupplierSKUs devolve os SKUs ATIVOS de um fornecedor.
	//
	// Existe separado de LookupBySKU porque o arquivamento por ausência precisa
	// justamente do que NÃO está no arquivo — e LookupBySKU só enxerga os SKUs
	// que o arquivo trouxe. Sem esta consulta, "arquivar quem sumiu" nunca
	// encontraria ninguém e a opção seria um no-op silencioso.
	ListSupplierSKUs(supplierID string) ([]string, error)
}

// Plan é o resultado do dry-run do lote inteiro.
type Plan struct {
	Rows    []RowResult `json:"rows"`
	Total   int         `json:"total"`
	Creates int         `json:"creates"`
	Updates int         `json:"updates"`
	Skips   int         `json:"skips"`
	Reviews int         `json:"reviews"`
	Rejects int         `json:"rejects"`
	// MissingSKUs: produtos do fornecedor que existem no catálogo e NÃO vieram
	// nesta planilha. Só preenchido quando o perfil liga `archiveMissing`.
	// Estes serão ARQUIVADOS, nunca apagados.
	MissingSKUs []string `json:"missingSkus,omitempty"`
	// Warnings de LOTE (não de linha): "a planilha não tem coluna de preço".
	Warnings []string `json:"warnings,omitempty"`
}

// Planner monta o plano.
type Planner struct {
	Profile *Profile
	Catalog Catalog
	// SupplierID limita o escopo do arquivamento por ausência: só produtos
	// DESTE fornecedor podem ser arquivados por não constar da planilha dele.
	// Sem esse escopo, importar a planilha do fornecedor A arquivaria o
	// catálogo inteiro do fornecedor B.
	SupplierID string
}

// Plan roda o dry-run completo. NUNCA devolve erro por causa de uma linha —
// erro aqui é falha estrutural do lote (perfil inválido, banco fora).
func (p *Planner) Plan(t *Table) (*Plan, error) {
	if p.Profile == nil {
		return nil, fmt.Errorf("perfil obrigatório")
	}
	if err := p.Profile.Validate(); err != nil {
		return nil, fmt.Errorf("perfil inválido: %w", err)
	}

	plan := &Plan{Total: len(t.Rows)}

	// Índice coluna→posição. O cabeçalho já veio limpo e desduplicado.
	colIdx := map[string]int{}
	for i, h := range t.Header {
		colIdx[h] = i
	}

	// Colunas do perfil que não existem no arquivo: aviso de LOTE, porque é
	// quase sempre "o fornecedor renomeou a coluna" — e é o que produz uma
	// importação que roda inteira e não atualiza preço nenhum.
	var missingCols []string
	for col := range p.Profile.Columns {
		if _, ok := colIdx[col]; !ok {
			missingCols = append(missingCols, col)
		}
	}
	sort.Strings(missingCols)
	for _, c := range missingCols {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("coluna %q do perfil não existe no arquivo — o campo correspondente não será atualizado", c))
	}

	// 1ª passada: mapear + validar formato. Sem tocar no banco.
	results := make([]RowResult, 0, len(t.Rows))
	skus := make([]string, 0, len(t.Rows))
	for i, row := range t.Rows {
		rowNum := i + 1
		if i < len(t.RowNumbers) {
			rowNum = t.RowNumbers[i]
		}
		r := p.mapRow(t.Header, colIdx, row, rowNum)
		if r.SKU != "" {
			skus = append(skus, r.SKU)
		}
		results = append(results, r)
	}

	// 2ª passada: consulta o estado atual (uma query pro lote inteiro, não uma
	// por linha — 4.000 round-trips transformariam o dry-run em minutos).
	existing := map[string]ExistingProduct{}
	if len(skus) > 0 && p.Catalog != nil {
		var err error
		existing, err = p.Catalog.LookupBySKU(skus)
		if err != nil {
			return nil, fmt.Errorf("consulta de produtos existentes: %w", err)
		}
	}

	// Cache de existência de categoria: a planilha repete a mesma categoria em
	// milhares de linhas.
	catCache := map[string]bool{}

	// A planilha TEM coluna de preço? (mapeada no perfil E presente no arquivo)
	//
	// É o que separa "esta linha veio sem preço" de "esta planilha não fala de
	// preço". No segundo caso — o SINAPI, que só traz custo de referência — reter
	// linha por linha mandaria o lote inteiro para revisão e ensinaria o operador
	// a aprovar tudo no automático, que é pior que o bug original. Exigir a coluna
	// PRESENTE NO ARQUIVO (e não só mapeada) evita o mesmo desastre quando o
	// fornecedor renomeia a coluna: nesse caso o aviso de LOTE já é gerado acima.
	priceColumnInFile := false
	for col, m := range p.Profile.Columns {
		if m.Field == FieldPrice {
			if _, ok := colIdx[col]; ok {
				priceColumnInFile = true
			}
		}
	}

	// SKUs duplicados DENTRO do mesmo arquivo: a segunda ocorrência
	// sobrescreveria a primeira em silêncio e o operador nunca saberia qual
	// preço venceu. Rejeitamos a duplicata e apontamos a linha original.
	firstSeen := map[string]int{}

	for i := range results {
		r := &results[i]
		if len(r.Errors) > 0 {
			r.Action = ActionReject
			continue
		}
		if dup, ok := firstSeen[r.SKU]; ok && r.SKU != "" {
			r.addError("sku", fmt.Sprintf("SKU duplicado no arquivo (já apareceu na linha %d)", dup))
			r.Action = ActionReject
			continue
		}
		if r.SKU != "" {
			firstSeen[r.SKU] = r.RowNumber
		}
		p.decide(r, existing, catCache, priceColumnInFile)
	}

	// Arquivamento por ausência — NUNCA delete.
	if p.Profile.Options.ArchiveMissing && p.Catalog != nil && p.SupplierID != "" {
		inFile := map[string]bool{}
		for _, r := range results {
			// Linha REJEITADA não conta como "presente no arquivo"... mas
			// também não pode causar arquivamento: o produto existe, só a linha
			// desta planilha estava ruim. Marcamos como presente pra que um
			// erro de digitação numa linha não tire o item da loja.
			if r.SKU != "" {
				inFile[r.SKU] = true
			}
		}
		supplierSKUs, err := p.Catalog.ListSupplierSKUs(p.SupplierID)
		if err != nil {
			return nil, fmt.Errorf("consulta de SKUs do fornecedor: %w", err)
		}
		for _, sku := range supplierSKUs {
			if !inFile[sku] {
				plan.MissingSKUs = append(plan.MissingSKUs, sku)
			}
		}
		sort.Strings(plan.MissingSKUs)
	}

	plan.Rows = results
	for _, r := range results {
		switch r.Action {
		case ActionCreate:
			plan.Creates++
		case ActionUpdate:
			plan.Updates++
		case ActionSkip:
			plan.Skips++
		case ActionReview:
			plan.Reviews++
		case ActionReject:
			plan.Rejects++
		}
	}
	return plan, nil
}

// mapRow aplica o perfil a uma linha e valida o FORMATO dos campos (sem
// consultar o banco).
func (p *Planner) mapRow(header []string, colIdx map[string]int, row []string, rowNum int) RowResult {
	r := RowResult{
		RowNumber: rowNum,
		Raw:       map[string]string{},
		Mapped:    map[string]any{},
		Errors:    []RowError{},
		Warnings:  []RowError{},
	}

	// Staging cru: TUDO que veio, exatamente como veio. É o que permite
	// reprocessar sem pedir o arquivo de novo e auditar "de onde veio esse
	// preço" três meses depois.
	for i, h := range header {
		if i < len(row) && row[i] != "" {
			r.Raw[h] = row[i]
		}
	}

	specs := map[string]string{}

	for col, m := range p.Profile.Columns {
		idx, ok := colIdx[col]
		if !ok || idx >= len(row) {
			continue
		}
		raw := row[idx]

		// Detecção de fórmula ANTES do parse: queremos que apareça na revisão,
		// não só neutralizada em silêncio. Planilha com fórmula injetada não é
		// descuido de formatação.
		if IsFormula(raw) {
			r.addWarning(string(m.Field),
				fmt.Sprintf("coluna %q contém o que parece uma fórmula (%.40q) — neutralizada", col, raw))
		}

		if m.Field == FieldSpecs {
			if v := CleanText(raw); v != "" && !IsEmpty(v) {
				specs[m.SpecKey] = SanitizeCell(v)
			}
			continue
		}

		val, ok, warn := p.Profile.ParserFor(m).Apply(string(m.Field), raw)

		if !ok {
			// Distinção que importa: CÉLULA VAZIA é ausência ("não sei"), e a
			// obrigatoriedade é checada depois. Mas CÉLULA COM CONTEÚDO QUE NÃO
			// PARSEIA é ERRO — tratá-la como ausência faria um preço negativo
			// ou um "consultar" na coluna de preço virar silenciosamente
			// "produto sem preço", que é exatamente o dado ruim entrando sem
			// ninguém ver.
			if !IsEmpty(raw) {
				msg := "valor ilegível"
				if warn != nil {
					msg = warn.Message
				}
				r.addError(string(m.Field), fmt.Sprintf("coluna %q: %s (valor: %q)", col, msg, CleanText(raw)))
			}
			continue
		}

		if warn != nil {
			r.addWarning(string(m.Field), fmt.Sprintf("coluna %q: %s (valor bruto: %q)", col, warn.Message, warn.Raw))
		}
		r.Mapped[string(m.Field)] = val
	}

	// Defaults do perfil preenchem só o que a planilha não trouxe.
	for f, v := range p.Profile.Defaults {
		if _, ok := r.Mapped[string(f)]; !ok && v != "" {
			r.Mapped[string(f)] = SanitizeCell(CleanText(v))
		}
	}
	if len(specs) > 0 {
		r.Mapped[string(FieldSpecs)] = specs
	}

	p.validateRow(&r)
	return r
}

// validateRow checa as regras de negócio de UMA linha.
func (p *Planner) validateRow(r *RowResult) {
	// --- name: sem nome não há produto -------------------------------------
	name, _ := r.Mapped[string(FieldName)].(string)
	name = CleanText(name)
	if name == "" {
		r.addError("name", "nome é obrigatório")
	} else if len(name) > 500 {
		r.addError("name", "nome com mais de 500 caracteres")
	} else {
		r.Mapped[string(FieldName)] = name
	}

	// --- sku: chave de identidade ------------------------------------------
	// O doc é explícito: "Sem SKU, a linha é rejeitada — não adivinhamos".
	// Casar por nome criaria produto duplicado no dia em que o fornecedor
	// escrever "Cimento CP-II 50kg" em vez de "Cimento CP II 50 kg".
	sku, _ := r.Mapped[string(FieldSKU)].(string)
	sku = strings.TrimSpace(sku)
	switch {
	case sku == "":
		r.addError("sku", "SKU é obrigatório — é a chave de identidade do produto na importação")
	case len(sku) > 100:
		r.addError("sku", "SKU com mais de 100 caracteres")
	default:
		r.SKU = sku
		r.Mapped[string(FieldSKU)] = sku
	}

	// --- price / cost -------------------------------------------------------
	if v, ok := r.Mapped[string(FieldPrice)].(float64); ok {
		if err := checkMoney("price", v); err != nil {
			r.addError("price", err.Error())
		} else {
			r.Mapped[string(FieldPrice)] = v
		}
	}
	if v, ok := r.Mapped[string(FieldCost)].(float64); ok {
		if err := checkMoney("cost", v); err != nil {
			r.addError("cost", err.Error())
		}
	}
	if v, ok := r.Mapped[string(FieldOriginalPrice)].(float64); ok {
		if err := checkMoney("originalPrice", v); err != nil {
			r.addError("originalPrice", err.Error())
		}
	}

	// --- stock --------------------------------------------------------------
	if v, ok := r.Mapped[string(FieldStock)].(float64); ok {
		if v < 0 {
			r.addError("stock", "estoque negativo")
		} else if v > 1e9 {
			r.addError("stock", "estoque implausível — confira o separador decimal")
		}
	}

	// --- unidade de medida --------------------------------------------------
	if raw, ok := r.Mapped[string(FieldUnitOfMeasure)].(string); ok && raw != "" {
		if IsLaborUnit(raw) {
			// Mão de obra / locação / energia não é item de prateleira.
			r.addError("unitOfMeasure", fmt.Sprintf(
				"unidade %q indica mão de obra, locação ou serviço — não é um produto de ferragem", raw))
		} else if u, ok := NormalizeUnit(raw); ok {
			// Peso embutido na unidade do SINAPI ("SC25KG" → saco de 25 kg).
			if w := UnitWeightKg(raw); w > 0 {
				if _, has := r.Mapped[string(FieldWeightKg)]; !has {
					r.Mapped[string(FieldWeightKg)] = w
				}
			}
			// Conversão de múltiplo (tonelada → quilo) tem que ajustar TAMBÉM
			// as quantidades, senão o estoque fica mil vezes errado.
			if qty, conv, did := ConvertQuantity(raw, 1); did {
				if st, ok := r.Mapped[string(FieldStock)].(float64); ok {
					r.Mapped[string(FieldStock)] = st * qty
				}
				if pr, ok := r.Mapped[string(FieldPrice)].(float64); ok && qty != 0 {
					r.Mapped[string(FieldPrice)] = math.Round((pr/qty)*100) / 100
				}
				if cs, ok := r.Mapped[string(FieldCost)].(float64); ok && qty != 0 {
					r.Mapped[string(FieldCost)] = math.Round((cs/qty)*100) / 100
				}
				u = conv
				r.addWarning("unitOfMeasure", fmt.Sprintf(
					"unidade %q convertida para %q; preço e estoque foram ajustados na mesma proporção", raw, conv))
			}
			r.Mapped[string(FieldUnitOfMeasure)] = u
		} else {
			// Unidade desconhecida NÃO reprova a linha: vira "un" com aviso.
			// Rejeitar aqui reprovaria metade do catálogo pela grafia do
			// fornecedor, e "un" é o padrão certo em caso de dúvida.
			r.Mapped[string(FieldUnitOfMeasure)] = "un"
			r.addWarning("unitOfMeasure", fmt.Sprintf("unidade %q desconhecida — assumido 'un'", raw))
		}
	}

	// --- barcode ------------------------------------------------------------
	if raw, ok := r.Mapped[string(FieldBarcode)].(string); ok && raw != "" {
		b := strings.NewReplacer(" ", "", "-", "", ".", "").Replace(raw)
		if !isAllDigits(b) || len(b) < 8 || len(b) > 14 {
			// Código de barras ruim não invalida o produto — invalida o código.
			// O item ainda vende, só não é lido por scanner.
			delete(r.Mapped, string(FieldBarcode))
			r.addWarning("barcode", fmt.Sprintf(
				"código de barras %q não é um GTIN de 8 a 14 dígitos — ignorado (peça o arquivo em CSV UTF-8: o Excel destrói código longo)", raw))
		} else {
			r.Mapped[string(FieldBarcode)] = b
		}
	}

	// --- fiscais ------------------------------------------------------------
	for _, f := range []struct {
		field  Field
		digits int
	}{{FieldNCM, 8}, {FieldCFOP, 4}, {FieldCEST, 7}} {
		if raw, ok := r.Mapped[string(f.field)].(string); ok && raw != "" {
			v := strings.NewReplacer(".", "", "-", "", "/", "", " ", "").Replace(raw)
			if !isAllDigits(v) || len(v) != f.digits {
				delete(r.Mapped, string(f.field))
				r.addWarning(string(f.field), fmt.Sprintf(
					"%s %q não tem %d dígitos — ignorado", f.field, raw, f.digits))
			} else {
				r.Mapped[string(f.field)] = v
			}
		}
	}

	// --- status: o arquivo NÃO publica sozinho ------------------------------
	if raw, ok := r.Mapped[string(FieldStatus)].(string); ok && raw != "" {
		s := strings.ToLower(strings.TrimSpace(raw))
		switch s {
		case "draft", "rascunho", "0", "inativo":
			r.Mapped[string(FieldStatus)] = "draft"
		case "archived", "arquivado":
			r.Mapped[string(FieldStatus)] = "archived"
		case "published", "publicado", "ativo", "1":
			// Mesmo pedindo "published", a publicação depende de o perfil
			// permitir (`publishOnImport`) E de o preço estar revisado. Ver
			// `effectiveStatus`. Uma coluna de planilha não é autorização
			// pra colocar item na vitrine.
			r.Mapped[string(FieldStatus)] = "published"
		default:
			delete(r.Mapped, string(FieldStatus))
			r.addWarning("status", fmt.Sprintf("status %q não reconhecido — ignorado", raw))
		}
	}

	// --- imagem -------------------------------------------------------------
	if raw, ok := r.Mapped[string(FieldImageURL)].(string); ok && raw != "" {
		u := strings.TrimPrefix(raw, "'") // pode ter sido saneado
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			delete(r.Mapped, string(FieldImageURL))
			r.addWarning("imageUrl", "URL de imagem precisa ser http(s) absoluta — ignorada")
		} else if len(u) > 2000 {
			delete(r.Mapped, string(FieldImageURL))
			r.addWarning("imageUrl", "URL de imagem longa demais — ignorada")
		} else {
			r.Mapped[string(FieldImageURL)] = u
		}
	}
}

const maxMoney = 1_000_000.0

func checkMoney(field string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s não é um número válido", field)
	}
	if v < 0 {
		return fmt.Errorf("%s não pode ser negativo", field)
	}
	if v > maxMoney {
		return fmt.Errorf("%s de %.2f excede o máximo de %.2f — confira o separador decimal", field, v, maxMoney)
	}
	return nil
}

// decide compara a linha com o estado atual e escolhe a ação.
func (p *Planner) decide(r *RowResult, existing map[string]ExistingProduct, catCache map[string]bool, priceColumnInFile bool) {
	// Categoria: obrigatória e tem que existir. FK que estoura no commit
	// devolveria erro de driver no meio do lote em vez de um relatório.
	cat, _ := r.Mapped[string(FieldCategory)].(string)
	cat = strings.ToLower(strings.TrimSpace(cat))
	ex, found := existing[r.SKU]

	if cat == "" && !found {
		r.addError("category", "categoria é obrigatória para criar um produto")
		r.Action = ActionReject
		return
	}
	if cat != "" {
		ok, cached := catCache[cat]
		if !cached && p.Catalog != nil {
			var err error
			ok, err = p.Catalog.CategoryExists(cat)
			if err != nil {
				r.addError("category", "não foi possível validar a categoria: "+err.Error())
				r.Action = ActionReject
				return
			}
			catCache[cat] = ok
		}
		if !ok {
			r.addError("category", fmt.Sprintf("categoria %q não existe no catálogo", cat))
			r.Action = ActionReject
			return
		}
		r.Mapped[string(FieldCategory)] = cat
	}

	newPrice, hasNewPrice := r.Mapped[string(FieldPrice)].(float64)

	if !found {
		// --- CRIAÇÃO --------------------------------------------------------
		r.Action = ActionCreate
		if hasNewPrice {
			r.NewPrice = &newPrice
		}
		if !hasNewPrice || newPrice == 0 {
			if priceColumnInFile {
				// A planilha TEM coluna de preço e esta linha veio sem valor (ou
				// com zero). Isso não é o caso legítimo do SINAPI: é uma célula
				// que deveria ter preço e não tem. Aceitar cria produto sem preço
				// no catálogo — e produto a R$ 0,00 é o que aparece na vitrine
				// como brinde. Rejeitar também seria errado (o dado pode estar
				// certo e o operador perderia o produto sem entender), então
				// RETÉM: a decisão vai para um humano, que é o princípio do
				// dry-run.
				r.Action = ActionReview
				r.addWarning("price", fmt.Sprintf(
					"linha %d: produto novo %q com preço de venda %s na coluna de preço — retido para revisão humana. "+
						"Confira o valor na planilha: produto criado sem preço entra no catálogo valendo R$ 0,00",
					r.RowNumber, r.SKU, describeMissingPrice(hasNewPrice)))
				return
			}
			// Planilha que NÃO fala de preço (SINAPI: só custo de referência).
			// Legítimo — o produto nasce rascunho e sem revisão de preço.
			r.addWarning("price",
				"sem preço de venda — o produto entra como rascunho e precisa de precificação antes de publicar")
		}
		if p.holdIfPriceBelowCost(r, newPrice, hasNewPrice, nil) {
			return
		}
		return
	}

	// --- ATUALIZAÇÃO --------------------------------------------------------
	r.ProductID = ex.ID

	if hasNewPrice {
		r.OldPrice, r.NewPrice = &ex.Price, &newPrice

		// A TRAVA DE PREÇO. O erro de vírgula ("1.234,56" lido como "1,23") é o
		// modo de falha mais caro do catálogo: passa em toda validação de
		// "número positivo", não gera exceção, e vende cimento a R$ 1,23 até
		// alguém reparar. A defesa não é parsing melhor — é desconfiar da
		// VARIAÇÃO e exigir olho humano.
		if ex.Price > 0 {
			delta := (newPrice - ex.Price) / ex.Price * 100
			switch {
			case delta < -p.Profile.Options.dropLimit():
				r.DropPct = -delta
				r.Action = ActionReview
				r.addWarning("price", fmt.Sprintf(
					"queda de %.1f%% (de R$ %.2f para R$ %.2f) acima do limite de %.0f%% — retido para revisão humana",
					-delta, ex.Price, newPrice, p.Profile.Options.dropLimit()))
				return
			case delta > p.Profile.Options.riseLimit():
				r.Action = ActionReview
				r.addWarning("price", fmt.Sprintf(
					"alta de %.1f%% (de R$ %.2f para R$ %.2f) acima do limite de %.0f%% — retido para revisão humana",
					delta, ex.Price, newPrice, p.Profile.Options.riseLimit()))
				return
			}
		}
	}

	// Preço abaixo do custo também numa ATUALIZAÇÃO — e aqui o custo pode vir da
	// planilha OU do cadastro: a linha que só atualiza preço não traz custo, e é
	// exatamente essa a que derruba a margem sem ninguém ver.
	//
	// Note que a trava de queda percentual acima NÃO cobre este caso: um preço
	// que cai 20% (dentro do limite) e mesmo assim fica abaixo do custo passaria
	// batido, porque o limite fala de VARIAÇÃO e este fala de MARGEM.
	if p.holdIfPriceBelowCost(r, newPrice, hasNewPrice, ex.Cost) {
		return
	}

	// Idempotência: rodar o mesmo arquivo duas vezes tem que dar o mesmo
	// resultado. Se nada mudou, a ação é `skip` — e não um UPDATE que só mexe
	// no `updated_at` e polui a auditoria com milhares de "mudanças" vazias.
	if !p.hasChanges(r, ex) {
		r.Action = ActionSkip
		return
	}
	r.Action = ActionUpdate
}

// describeMissingPrice traduz o estado da célula para o vocabulário de quem lê o
// relatório — o comprador da loja, não um programador. "vazio" e "R$ 0,00" são
// problemas diferentes e pedem conferências diferentes na planilha.
func describeMissingPrice(hasPrice bool) string {
	if hasPrice {
		return "igual a R$ 0,00"
	}
	return "vazio"
}

// holdIfPriceBelowCost retém a linha quando o preço de venda fica abaixo do
// custo. Devolve true se reteve.
//
// POR QUE ISTO EXISTE, já havendo a trava de queda percentual: aquela desconfia
// da VARIAÇÃO (preço de ontem vs. de hoje) e não enxerga produto NOVO, que não
// tem "ontem". O erro de vírgula na primeira importação de um item — "123,40"
// digitado como "1,23" — entra sem nenhum alarme, e o único sinal disponível é
// o custo estar na mesma linha, dez vezes maior que o preço.
//
// RETÉM, não rejeita: vender abaixo do custo é uma decisão comercial real
// (item de isca, brinde, queima de estoque). Quem faz isso de propósito liga
// `allowPriceBelowCost` no perfil daquele fornecedor.
func (p *Planner) holdIfPriceBelowCost(r *RowResult, price float64, hasPrice bool, existingCost *float64) bool {
	if !hasPrice || p.Profile.Options.AllowPriceBelowCost {
		return false
	}

	cost, hasCost := r.Mapped[string(FieldCost)].(float64)
	origem := "custo da planilha"
	if !hasCost && existingCost != nil {
		cost, hasCost, origem = *existingCost, true, "custo cadastrado no produto"
	}
	// Custo zero/ausente não é sinal: significa "não sei o custo", e reter por
	// desconhecimento retém o catálogo inteiro de quem não manda coluna de custo.
	if !hasCost || cost <= 0 {
		return false
	}
	// Tolerância de meio centavo: preço IGUAL ao custo (margem zero) é decisão
	// comercial comum e não pode disparar a trava por ruído de float64.
	if cost-price <= 0.005 {
		return false
	}

	r.Action = ActionReview
	r.addWarning("price", fmt.Sprintf(
		"linha %d: preço de venda R$ %s abaixo do custo R$ %s (%s) — retido para revisão humana. "+
			"Confira o separador decimal na coluna de preço (o erro mais caro da importação é '123,40' digitado como '1,23'). "+
			"Se a venda abaixo do custo for intencional, ligue 'allowPriceBelowCost' no perfil deste fornecedor",
		r.RowNumber, moneyBR(price), moneyBR(cost), origem))
	return true
}

// moneyBR formata dinheiro na convenção brasileira. Existe porque a mensagem é
// lida pelo comprador da loja, e "R$ 31.40" ao lado de uma planilha que escreve
// "31,40" faz o operador duvidar se o sistema leu o número certo — exatamente a
// dúvida que a mensagem deveria eliminar.
func moneyBR(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	intPart, decPart := s[:len(s)-3], s[len(s)-2:]
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 && c != '-' {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String() + "," + decPart
}

// hasChanges compara o que a linha traz com o que já está gravado.
//
// Só considera campos PRESENTES na linha: ausência na planilha significa "não
// sei", nunca "apague" — a regra do "nunca apagar por ausência" aplicada campo
// a campo, não só a produto.
func (p *Planner) hasChanges(r *RowResult, ex ExistingProduct) bool {
	if v, ok := r.Mapped[string(FieldPrice)].(float64); ok && !floatEq(v, ex.Price) {
		return true
	}
	if v, ok := r.Mapped[string(FieldCost)].(float64); ok {
		if ex.Cost == nil || !floatEq(v, *ex.Cost) {
			return true
		}
	}
	if v, ok := r.Mapped[string(FieldStock)].(float64); ok && !floatEq(v, ex.Stock) {
		return true
	}
	if v, ok := r.Mapped[string(FieldName)].(string); ok && v != ex.Name {
		return true
	}
	if v, ok := r.Mapped[string(FieldBrand)].(string); ok && v != ex.Brand {
		return true
	}
	if v, ok := r.Mapped[string(FieldDescription)].(string); ok && v != ex.Description {
		return true
	}
	if v, ok := r.Mapped[string(FieldUnitOfMeasure)].(string); ok && v != ex.UnitOfMeasure {
		return true
	}
	if v, ok := r.Mapped[string(FieldBarcode)].(string); ok && v != ex.Barcode {
		return true
	}
	if v, ok := r.Mapped[string(FieldStatus)].(string); ok && v != ex.Status {
		return true
	}
	// Campos sem "antes" carregado no snapshot (specs, dimensões, fiscais):
	// tratados como mudança quando presentes. Preferimos um UPDATE a mais que
	// um dado silenciosamente não aplicado.
	for _, f := range []Field{FieldSpecs, FieldWeightKg, FieldNCM, FieldCFOP,
		FieldCEST, FieldOrigem, FieldQtyStep, FieldImageURL, FieldOriginalPrice,
		FieldLengthCm, FieldWidthCm, FieldHeightCm, FieldSupplierSKU} {
		if _, ok := r.Mapped[string(f)]; ok {
			return true
		}
	}
	return false
}

// floatEq compara dinheiro/quantidade com tolerância de meio centavo. Comparar
// float64 com `==` faria 42.90 lido do arquivo diferir de 42.90 vindo do
// NUMERIC do Postgres e o lote reportaria "atualizado" eternamente pro mesmo
// arquivo — quebrando a idempotência sem nenhum sintoma visível.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}

// MarshalRaw serializa a linha crua pro staging (import_rows.raw).
func (r *RowResult) MarshalRaw() []byte {
	b, err := json.Marshal(r.Raw)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

// MarshalMapped serializa a linha mapeada.
func (r *RowResult) MarshalMapped() []byte {
	b, err := json.Marshal(r.Mapped)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func marshalOrEmpty(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`[]`)
	}
	return b
}
