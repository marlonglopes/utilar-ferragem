package ingest

// O PERFIL DE MAPEAMENTO — "como as colunas deste fornecedor viram nossos
// campos". É DADO (JSONB em import_profiles), não código: fornecedor novo é
// uma linha de tabela, não um deploy.
//
// Este arquivo carrega também a fronteira de segurança mais importante do
// pipeline: a WHITELIST DE CAMPOS. Sem ela, um perfil que mapeasse
// `"COLUNA X" → "id"` ou `→ "created_at"` faria o importador escrever em
// coluna arbitrária de `products`. Nada do que vem do arquivo, nem do perfil,
// chega ao SQL sem passar por `fieldSpecs` abaixo.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Field é um campo do domínio que a ingestão pode preencher.
type Field string

const (
	FieldSKU           Field = "sku"
	FieldName          Field = "name"
	FieldCategory      Field = "category"
	FieldPrice         Field = "price"
	FieldOriginalPrice Field = "originalPrice"
	FieldCost          Field = "cost"
	FieldStock         Field = "stock"
	FieldBrand         Field = "brand"
	FieldDescription   Field = "description"
	FieldUnitOfMeasure Field = "unitOfMeasure"
	FieldQtyStep       Field = "qtyStep"
	FieldBarcode       Field = "barcode"
	FieldWeightKg      Field = "weightKg"
	FieldLengthCm      Field = "lengthCm"
	FieldWidthCm       Field = "widthCm"
	FieldHeightCm      Field = "heightCm"
	FieldSupplierID    Field = "supplierId"
	FieldSupplierSKU   Field = "supplierSku"
	FieldNCM           Field = "ncm"
	FieldCFOP          Field = "cfop"
	FieldCEST          Field = "cest"
	FieldOrigem        Field = "origem"
	FieldImageURL      Field = "imageUrl"
	FieldStatus        Field = "status"
	FieldSpecs         Field = "specs"
)

// FieldSpec descreve um campo mapeável: a coluna FÍSICA que ele alimenta e o
// parser padrão.
//
// `Column` é o único lugar de onde sai nome de coluna pro SQL do upsert. É
// literal, escrito à mão aqui — nunca vem do arquivo nem do perfil. É o que
// torna impossível transformar a importação em escrita arbitrária.
type FieldSpec struct {
	Column        string // coluna real em `products`
	DefaultParser Parser
	// Aliases usados na sugestão automática de mapeamento. A tela de upload
	// propõe o de/para e o humano confirma — sugerir não é decidir.
	Aliases []string
}

// fieldSpecs — A WHITELIST. Campo fora daqui é rejeitado na validação do
// perfil, com nome, e não chega ao banco.
//
// Ausências deliberadas: `id`, `slug`, `seller_id`, `created_at`,
// `updated_at`, `status` como coluna livre, `price_reviewed`, `source`.
//   - `id`/`created_at`: identidade e trilha; planilha não os define.
//   - `slug`: derivado do nome, e URL do produto mudar por importação quebra
//     link já indexado e compartilhado.
//   - `price_reviewed`/`source`: são as travas de segurança do próprio
//     pipeline. Se a planilha pudesse escrevê-las, a trava seria contornável
//     por quem envia o arquivo.
var fieldSpecs = map[Field]FieldSpec{
	FieldSKU:           {"sku", ParserCode, []string{"sku", "codigo", "código", "cod", "cod produto", "codigo produto", "referencia", "referência", "ref"}},
	FieldName:          {"name", ParserText, []string{"name", "nome", "descricao", "descrição", "descricao do produto", "produto", "item", "descricao do insumo"}},
	FieldCategory:      {"category_id", ParserText, []string{"category", "categoria", "grupo", "familia", "família", "departamento", "secao", "seção"}},
	FieldPrice:         {"price", ParserMoneyBR, []string{"price", "preco", "preço", "valor", "vlr venda", "preco venda", "preço de venda", "valor unitario", "pvenda", "pv"}},
	FieldOriginalPrice: {"original_price", ParserMoneyBR, []string{"originalprice", "preco original", "preço de", "valor de", "preco cheio"}},
	FieldCost:          {"cost", ParserMoneyBR, []string{"cost", "custo", "preco custo", "preço de custo", "vlr custo", "custo unitario", "preco mediano"}},
	FieldStock:         {"stock", ParserNumber, []string{"stock", "estoque", "qtd", "quantidade", "saldo", "qtde", "disponivel", "disponível"}},
	FieldBrand:         {"brand", ParserText, []string{"brand", "marca", "fabricante"}},
	FieldDescription:   {"description", ParserText, []string{"description", "descricao completa", "detalhes", "observacao", "observação", "obs"}},
	FieldUnitOfMeasure: {"unit_of_measure", ParserText, []string{"unit", "unidade", "un", "und", "unid", "unidade de medida", "um", "medida"}},
	FieldQtyStep:       {"qty_step", ParserNumber, []string{"qtystep", "passo", "fracionamento", "multiplo", "múltiplo", "embalagem"}},
	FieldBarcode:       {"barcode", ParserCode, []string{"barcode", "ean", "gtin", "codigo de barras", "código de barras", "ean13", "cod barras"}},
	FieldWeightKg:      {"weight_kg", ParserNumber, []string{"weightkg", "peso", "peso kg", "peso liquido", "peso bruto"}},
	FieldLengthCm:      {"length_cm", ParserNumber, []string{"lengthcm", "comprimento", "comp", "comprimento cm"}},
	FieldWidthCm:       {"width_cm", ParserNumber, []string{"widthcm", "largura", "larg", "largura cm"}},
	FieldHeightCm:      {"height_cm", ParserNumber, []string{"heightcm", "altura", "alt", "altura cm"}},
	FieldSupplierID:    {"supplier_id", ParserText, []string{"supplierid", "fornecedor", "cod fornecedor", "id fornecedor"}},
	FieldSupplierSKU:   {"supplier_sku", ParserCode, []string{"suppliersku", "codigo fornecedor", "código do fornecedor", "ref fornecedor"}},
	FieldNCM:           {"ncm", ParserCode, []string{"ncm", "cod ncm", "classificacao fiscal"}},
	FieldCFOP:          {"cfop", ParserCode, []string{"cfop"}},
	FieldCEST:          {"cest", ParserCode, []string{"cest"}},
	FieldOrigem:        {"origem", ParserNumber, []string{"origem", "origem mercadoria"}},
	FieldImageURL:      {"", ParserText, []string{"imageurl", "image_url", "imagem", "foto", "url imagem", "link foto"}},
	FieldStatus:        {"status", ParserText, []string{"status", "situacao", "situação", "ativo"}},
	FieldSpecs:         {"specs", ParserText, []string{"specs", "ficha tecnica", "ficha técnica", "atributos"}},
}

// KnownFields devolve a lista ordenada de campos mapeáveis — alimenta a tela
// de mapeamento e a mensagem de erro de perfil inválido.
func KnownFields() []string {
	out := make([]string, 0, len(fieldSpecs))
	for f := range fieldSpecs {
		out = append(out, string(f))
	}
	sort.Strings(out)
	return out
}

// ColumnFor devolve a coluna física de um campo, com um segundo retorno
// dizendo se o campo é escrevível em `products`. Único caminho pro SQL.
func ColumnFor(f Field) (string, bool) {
	spec, ok := fieldSpecs[f]
	if !ok || spec.Column == "" {
		return "", false
	}
	return spec.Column, true
}

// ColumnMapping é o de/para de UMA coluna da planilha.
type ColumnMapping struct {
	Field  Field  `json:"field"`
	Parser Parser `json:"parser,omitempty"`
	// SpecKey: quando preenchido, a coluna vira uma chave dentro de `specs` em
	// vez de um campo próprio. É como uma planilha com 40 colunas técnicas
	// ("Tensão", "RPM", "Bitola") entra sem exigir 40 colunas no banco.
	SpecKey string `json:"specKey,omitempty"`
}

// Options são as travas de segurança do lote — configuráveis por perfil porque
// o limite razoável difere entre fornecedores, mas sempre COM valor padrão
// seguro (perfil omisso não desliga a trava).
type Options struct {
	// MaxPriceDropPct: queda percentual acima disto manda a linha pra revisão
	// humana em vez de aplicar. É a defesa contra o erro de vírgula
	// ("1.234,56" lido como "1,23" = queda de 99,9%), o modo de falha mais caro
	// do catálogo. 0 = usa o padrão.
	MaxPriceDropPct float64 `json:"maxPriceDropPct,omitempty"`
	// MaxPriceRisePct: subida absurda também é erro (o mesmo bug ao contrário,
	// ou custo mapeado na coluna de preço).
	MaxPriceRisePct float64 `json:"maxPriceRisePct,omitempty"`
	// ArchiveMissing: arquivar produtos do fornecedor que não vieram nesta
	// planilha. Padrão FALSE porque fornecedor manda planilha parcial o tempo
	// todo, e o comportamento "some da planilha = some da loja" evaporaria o
	// catálogo. Quando ligado, arquiva — NUNCA apaga.
	ArchiveMissing bool `json:"archiveMissing,omitempty"`
	// PublishOnImport: publicar direto. Padrão FALSE — produto importado entra
	// como rascunho e a vitrine é decisão humana.
	PublishOnImport bool `json:"publishOnImport,omitempty"`
	// DefaultCategory / DefaultUnit: usados quando a planilha não traz a coluna.
	DefaultCategory string `json:"defaultCategory,omitempty"`
	DefaultUnit     string `json:"defaultUnit,omitempty"`
	// HeaderRow: linha 1-based do cabeçalho. 0 = detectar.
	HeaderRow int `json:"headerRow,omitempty"`
	// Sheet: aba do XLSX. Vazio = a primeira.
	Sheet string `json:"sheet,omitempty"`
}

const (
	DefaultMaxPriceDropPct = 30.0
	DefaultMaxPriceRisePct = 300.0
)

func (o Options) dropLimit() float64 {
	if o.MaxPriceDropPct <= 0 {
		return DefaultMaxPriceDropPct
	}
	return o.MaxPriceDropPct
}

func (o Options) riseLimit() float64 {
	if o.MaxPriceRisePct <= 0 {
		return DefaultMaxPriceRisePct
	}
	return o.MaxPriceRisePct
}

// Profile é o perfil completo.
type Profile struct {
	ID      string                   `json:"id,omitempty"`
	Name    string                   `json:"name"`
	Version int                      `json:"version"`
	Kind    string                   `json:"kind,omitempty"`
	Columns map[string]ColumnMapping `json:"columns"`
	// Defaults aplica valor fixo a um campo quando a planilha não o traz.
	// Valor de admin, não do arquivo — e passa pela mesma whitelist.
	Defaults map[Field]string `json:"defaults,omitempty"`
	Options  Options          `json:"options,omitempty"`
}

// Validate roda ANTES de qualquer uso do perfil. Um perfil inválido tem que
// falhar na criação, com mensagem, e não na 4.000ª linha da importação.
func (p *Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("perfil precisa de nome")
	}
	if len(p.Columns) == 0 && len(p.Defaults) == 0 {
		return fmt.Errorf("perfil precisa mapear ao menos uma coluna")
	}
	if len(p.Columns) > MaxColumns {
		return fmt.Errorf("perfil com %d colunas excede o limite de %d", len(p.Columns), MaxColumns)
	}

	seen := map[Field]string{}
	for col, m := range p.Columns {
		if _, ok := fieldSpecs[m.Field]; !ok {
			return fmt.Errorf("coluna %q mapeia o campo desconhecido %q; campos válidos: %s",
				col, m.Field, strings.Join(KnownFields(), ", "))
		}
		if m.Parser != "" && !KnownParsers[m.Parser] {
			return fmt.Errorf("coluna %q usa o parser desconhecido %q", col, m.Parser)
		}
		// Duas colunas no mesmo campo tornariam o resultado dependente da ordem
		// de iteração do mapa — ou seja, não-determinístico entre execuções.
		// Exceção: `specs`, que é justamente a agregação de várias colunas.
		if m.Field != FieldSpecs {
			if prev, dup := seen[m.Field]; dup {
				return fmt.Errorf("campo %q mapeado por duas colunas (%q e %q); escolha uma",
					m.Field, prev, col)
			}
			seen[m.Field] = col
		}
		if m.Field == FieldSpecs && strings.TrimSpace(m.SpecKey) == "" {
			return fmt.Errorf("coluna %q mapeada para specs precisa de specKey", col)
		}
	}

	for f := range p.Defaults {
		if _, ok := fieldSpecs[f]; !ok {
			return fmt.Errorf("default para o campo desconhecido %q", f)
		}
	}

	// Sem `name` não há produto, e sem SKU não há upsert idempotente. Exigimos
	// `name` sempre; `sku` é exigido no commit (linha sem SKU é rejeitada lá,
	// com o número da linha), não aqui — planilha pode ter SKU em algumas
	// linhas e não em outras, e isso é problema de linha, não de perfil.
	if _, ok := seen[FieldName]; !ok {
		if _, hasDefault := p.Defaults[FieldName]; !hasDefault {
			return fmt.Errorf("perfil precisa mapear o campo 'name'")
		}
	}
	if p.Kind != "" && p.Kind != "generic" && p.Kind != "sinapi" {
		return fmt.Errorf("kind %q inválido (generic|sinapi)", p.Kind)
	}
	return nil
}

// ParserFor devolve o parser efetivo da coluna (o do perfil, ou o padrão do
// campo). Deixar o perfil omitir o parser é o que torna a tela de mapeamento
// utilizável: o operador escolhe o CAMPO e o sistema já sabe como ler.
func (p *Profile) ParserFor(m ColumnMapping) Parser {
	if m.Parser != "" {
		return m.Parser
	}
	if spec, ok := fieldSpecs[m.Field]; ok && spec.DefaultParser != "" {
		return spec.DefaultParser
	}
	return ParserText
}

// normalizeHeader deixa o cabeçalho comparável: sem acento, sem caixa, sem
// pontuação, com espaço único. "VLR. VENDA " e "vlr venda" têm que colidir —
// planilha de fornecedor escreve o mesmo conceito de cinco maneiras, e comparar
// a string crua é o que faz o mapeamento automático não reconhecer nada.
func normalizeHeader(s string) string {
	s = strings.ToLower(stripAccents(CleanText(s)))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}), " ")
}

// Confidence é o grau de certeza de uma sugestão de mapeamento.
//
// Existe para que a tela DESTAQUE o que precisa de conferência humana. Sem
// graduação, o operador vê 40 linhas de palpite com o mesmo peso visual e
// confirma tudo no automático — que é exatamente como um "VLR CUSTO" mapeado em
// `price` chega ao catálogo.
type Confidence string

const (
	// ConfidenceExact: o cabeçalho normalizado É um alias conhecido
	// ("PREÇO" → "preco"). Praticamente não erra.
	ConfidenceExact Confidence = "exact"
	// ConfidenceHigh: o alias aparece como palavra inteira dentro do cabeçalho
	// ("VLR VENDA UNITARIO" contém "vlr venda"). Confiável, mas o operador ainda
	// deve bater o olho.
	ConfidenceHigh Confidence = "high"
	// ConfidenceLow: casou por prefixo/abreviação. É palpite — a tela deve pedir
	// confirmação explícita antes de aceitar.
	ConfidenceLow Confidence = "low"
)

// Suggestion é o palpite para UMA coluna da planilha.
type Suggestion struct {
	// Column é o cabeçalho como veio no arquivo (não o normalizado): é o que o
	// operador vê na planilha dele.
	Column     string     `json:"column"`
	Field      Field      `json:"field,omitempty"`
	Parser     Parser     `json:"parser,omitempty"`
	Confidence Confidence `json:"confidence,omitempty"`
	// MatchedAlias explica o PORQUÊ do palpite. "Por que ele achou que essa
	// coluna é preço?" tem que ser respondível na própria tela, senão o humano
	// não tem como confirmar de forma informada.
	MatchedAlias string `json:"matchedAlias,omitempty"`
	// Recognized=false é o caso normal de coluna que não conhecemos. NÃO é erro:
	// a coluna fica sem mapear e é ignorada na importação. Fornecedor manda
	// coluna interna ("COD_ERP_ANTIGO", "OBS VENDEDOR") o tempo todo, e reprovar
	// o arquivo por causa dela tornaria o importador inutilizável.
	Recognized bool `json:"recognized"`
}

// SuggestColumns detecta as colunas e propõe o de/para COM grau de confiança.
//
// É SUGESTÃO, não decisão: o fluxo exige confirmação humana (a tela mostra as
// colunas detectadas e o palpite ao lado). Mapear preço na coluna errada é o
// desastre nº 1 da ingestão, e adivinhar sozinho não é um risco que valha a
// economia de um clique.
//
// Devolve UMA entrada por coluna do arquivo, inclusive as não reconhecidas —
// a tela precisa listar o que será ignorado, senão o operador só descobre que
// a coluna de custo não entrou depois de importar.
func SuggestColumns(header []string) []Suggestion {
	// Índice alias→campo. Ordenado pra que empate resolva sempre igual: iteração
	// de mapa em Go é aleatória, e sugestão que muda a cada F5 destrói a
	// confiança do operador na tela.
	type cand struct {
		field Field
		alias string
	}
	var cands []cand
	for f, spec := range fieldSpecs {
		// `specs` fica FORA da sugestão automática: mapear uma coluna para specs
		// exige um `specKey`, que só o humano sabe escolher. Sugeri-la produziria
		// um perfil que falha em Validate() — palpite que não pode ser aceito não
		// é palpite útil.
		if f == FieldSpecs {
			continue
		}
		for _, a := range spec.Aliases {
			cands = append(cands, cand{f, normalizeHeader(a)})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if len(cands[i].alias) != len(cands[j].alias) {
			return len(cands[i].alias) > len(cands[j].alias) // alias mais específico primeiro
		}
		if cands[i].alias != cands[j].alias {
			return cands[i].alias < cands[j].alias
		}
		return cands[i].field < cands[j].field
	})

	out := make([]Suggestion, 0, len(header))
	used := map[Field]bool{}

	// DUAS PASSADAS por nível de confiança, e não uma só na ordem das colunas.
	// Um campo só pode ser reivindicado por uma coluna; se a primeira coluna da
	// planilha casasse por prefixo fraco, ela tomaria o campo e a coluna
	// seguinte — que casava EXATAMENTE — ficaria sem mapeamento. A ordem das
	// colunas no arquivo não deve decidir a qualidade do mapeamento.
	type match struct {
		conf  Confidence
		field Field
		alias string
	}
	best := make([]*match, len(header))

	for pass := 0; pass < 3; pass++ {
		for i, h := range header {
			if best[i] != nil {
				continue
			}
			n := normalizeHeader(h)
			if n == "" {
				continue
			}
			for _, c := range cands {
				if used[c.field] {
					continue
				}
				var conf Confidence
				switch {
				case pass == 0 && n == c.alias:
					conf = ConfidenceExact
				case pass == 1 && containsWord(n, c.alias):
					conf = ConfidenceHigh
				case pass == 2 && weakMatch(n, c.alias):
					conf = ConfidenceLow
				default:
					continue
				}
				best[i] = &match{conf, c.field, c.alias}
				used[c.field] = true
				break
			}
		}
	}

	for i, h := range header {
		s := Suggestion{Column: h}
		if m := best[i]; m != nil {
			s.Field, s.Confidence, s.MatchedAlias, s.Recognized = m.field, m.conf, m.alias, true
			if spec, ok := fieldSpecs[m.field]; ok {
				s.Parser = spec.DefaultParser
			}
		}
		out = append(out, s)
	}
	return out
}

// weakMatch é o último recurso: o cabeçalho é uma grafia MAIS ESPECÍFICA de um
// alias conhecido, colada sem separador ("PRECOVENDA" → "preco", "QTDESTOQUE" →
// "qtd"). Só a partir de 4 caracteres — abaixo disso o prefixo casa com
// qualquer coisa.
//
// ⚠️ A DIREÇÃO IMPORTA, e só uma delas é segura. Aceitar também o inverso
// (cabeçalho como prefixo de um alias mais longo) parece simétrico e é um
// desastre: "PRECO" é prefixo de "preco de custo", então um arquivo em que
// outra coluna já tivesse reivindicado `price` faria "PRECO" cair em `cost` —
// o preço de venda gravado como custo, em silêncio, com o rótulo mais óbvio
// possível na planilha. Um cabeçalho genérico tem que ficar SEM MAPEAR (o
// humano resolve na tela) e nunca escorregar para um campo vizinho.
func weakMatch(header, alias string) bool {
	if len(header) < 4 || len(alias) < 4 {
		return false
	}
	return strings.HasPrefix(header, alias)
}

// SuggestMapping é a forma pronta para virar `Profile.Columns`, descartando as
// colunas não reconhecidas.
//
// Mantida separada de SuggestColumns porque as duas respondem a perguntas
// diferentes: esta responde "qual perfil eu proponho", aquela responde "o que
// eu vi na planilha e com que certeza" — e a tela de confirmação precisa da
// segunda, inclusive do que ficou de fora.
func SuggestMapping(header []string) map[string]ColumnMapping {
	out := map[string]ColumnMapping{}
	for _, s := range SuggestColumns(header) {
		if s.Recognized {
			out[s.Column] = ColumnMapping{Field: s.Field}
		}
	}
	return out
}

func containsWord(hay, needle string) bool {
	if needle == "" || len(needle) < 3 {
		return false // alias curto ("um", "un") casaria com qualquer coisa
	}
	return hay == needle ||
		strings.HasPrefix(hay, needle+" ") ||
		strings.HasSuffix(hay, " "+needle) ||
		strings.Contains(hay, " "+needle+" ")
}

// MarshalMapping serializa o perfil pra coluna JSONB.
func (p *Profile) MarshalMapping() ([]byte, error) {
	return json.Marshal(struct {
		Columns  map[string]ColumnMapping `json:"columns"`
		Defaults map[Field]string         `json:"defaults,omitempty"`
		Options  Options                  `json:"options"`
	}{p.Columns, p.Defaults, p.Options})
}

// UnmarshalMapping lê o perfil da coluna JSONB e REVALIDA.
//
// Revalidar na leitura e não só na escrita: o JSONB pode ter sido editado
// direto no banco por um DBA, ou ter vindo de uma versão anterior do código com
// campos que não existem mais. Confiar no que está gravado é como um perfil
// antigo passa a escrever em coluna que a whitelist de hoje não permite.
func (p *Profile) UnmarshalMapping(raw []byte) error {
	var m struct {
		Columns  map[string]ColumnMapping `json:"columns"`
		Defaults map[Field]string         `json:"defaults"`
		Options  Options                  `json:"options"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("mapeamento do perfil ilegível: %w", err)
	}
	p.Columns, p.Defaults, p.Options = m.Columns, m.Defaults, m.Options
	return p.Validate()
}
