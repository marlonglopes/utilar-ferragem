// Package knowledge é a base de conhecimento de obra da Alice.
//
// PRINCÍPIO: tool use é a única fonte de fatos. Assim como preço e estoque vêm
// do catalog-service (e nunca da memória do modelo), coeficiente de consumo de
// material vem DAQUI — dados versionados no repositório, com fonte declarada
// item a item — e nunca do prompt nem do que o modelo "lembra".
//
// Motivo prático: coeficiente errado tem consequência física. Cimento a menos
// para a obra e ela para; a mais e o cliente perde dinheiro (e cimento vence).
// Por isso todo coeficiente aqui é uma FAIXA (min–max) com fonte. Quando não há
// confiança num número, a regra é publicar a faixa honesta ou não publicar.
//
// Os dados vivem em data/*.json e são embutidos no binário. A validação roda no
// BOOT (Load): coeficiente faltando, faixa invertida ou fonte ausente derruba o
// serviço na subida. Falhar alto é melhor que servir número errado em silêncio.
package knowledge

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed data/*.json
var dataFS embed.FS

// ---------------------------------------------------------------------------
// Fonte
// ---------------------------------------------------------------------------

// SourceKind classifica de onde veio um número. A distinção é deliberada: uma
// NBR quase nunca publica "consumo por m²" — ela fixa dimensões, requisitos de
// produto ou procedimento de execução. Dizer "NBR" para um coeficiente de
// consumo seria inventar norma, que é exatamente o que não se pode fazer.
type SourceKind string

const (
	// KindNorma — a norma citada cobre diretamente o que está sendo afirmado.
	KindNorma SourceKind = "norma"
	// KindGeometrico — número DERIVADO por cálculo a partir de dimensões
	// nominais (normalmente fixadas por uma norma) mais junta/sobreposição.
	// Ex.: 12,5 blocos/m² não está em norma nenhuma; sai de 39x19 cm + junta 1 cm.
	KindGeometrico SourceKind = "geometrico"
	// KindFabricante — faixa declarada em embalagem/ficha técnica de fabricantes.
	// Costuma ser otimista; por isso guardamos faixa, não o número do folheto.
	KindFabricante SourceKind = "fabricante"
	// KindMercado — consumo típico de mercado / prática corrente de obra.
	// Sem respaldo normativo. É o rótulo mais honesto para a maioria dos traços.
	KindMercado SourceKind = "mercado"
	// KindDefinicao — conversão exata por definição de unidade ou de embalagem
	// (1 m³ = 1000 L; saco de cimento = 50 kg). Não é estimativa nem norma.
	KindDefinicao SourceKind = "definicao"
)

var validKinds = map[SourceKind]bool{
	KindNorma: true, KindGeometrico: true, KindFabricante: true,
	KindMercado: true, KindDefinicao: true,
}

// Source é a procedência de um dado. Obrigatória em todo coeficiente — a Alice
// tem que poder dizer de onde tirou o número quando o cliente perguntar.
type Source struct {
	Tipo SourceKind `json:"tipo"`
	Ref  string     `json:"ref"`  // "ABNT NBR 6136" | "consumo típico de mercado"
	Nota string     `json:"nota"` // o que essa referência de fato cobre
}

func (s Source) validate(path string) error {
	if !validKinds[s.Tipo] {
		return fmt.Errorf("%s: fonte.tipo inválido %q", path, s.Tipo)
	}
	if strings.TrimSpace(s.Ref) == "" {
		return fmt.Errorf("%s: fonte.ref vazia — todo dado precisa de procedência", path)
	}
	if strings.TrimSpace(s.Nota) == "" {
		return fmt.Errorf("%s: fonte.nota vazia — precisa dizer o que a referência cobre", path)
	}
	// Guarda anti-"inventar norma": se o tipo é norma, a ref tem que parecer uma.
	if s.Tipo == KindNorma && !looksLikeStandard(s.Ref) {
		return fmt.Errorf("%s: fonte.tipo=norma mas ref %q não parece norma (NBR/NR)", path, s.Ref)
	}
	// E o inverso: não deixar uma NBR ser citada como se fosse achismo de mercado.
	if s.Tipo == KindMercado && strings.Contains(strings.ToUpper(s.Ref), "NBR") {
		return fmt.Errorf("%s: ref cita NBR mas tipo=mercado — use tipo=norma ou tire a NBR", path)
	}
	return nil
}

func looksLikeStandard(ref string) bool {
	u := strings.ToUpper(ref)
	return strings.Contains(u, "NBR") || strings.HasPrefix(u, "NR-") || strings.Contains(u, " NR-")
}

// Human formata a fonte para a Alice citar em texto.
func (s Source) Human() string {
	switch s.Tipo {
	case KindGeometrico:
		return "cálculo geométrico (" + s.Ref + ")"
	case KindFabricante:
		return "faixa declarada por fabricantes (" + s.Ref + ")"
	case KindMercado:
		return s.Ref
	case KindDefinicao:
		return "conversão exata (" + s.Ref + ")"
	default:
		return s.Ref
	}
}

// ---------------------------------------------------------------------------
// Coeficiente
// ---------------------------------------------------------------------------

// Coef é sempre uma FAIXA. Não existe coeficiente de consumo exato em obra:
// junta, prumo, perda e mão de obra variam. Publicar "12,5" como verdade única
// é falsa precisão; a faixa é o dado honesto. Mid() é o que vai para o
// orçamento, Min/Max é o que a Alice mostra na memória de cálculo.
type Coef struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Unid   string  `json:"unid"` // unidade do coeficiente, ex "bloco/m2", "kg/m2"
	Fonte  Source  `json:"fonte"`
	Perda  float64 `json:"perda"`  // fração de perda já recomendada (0.05 = 5%)
	Motivo string  `json:"motivo"` // por que a faixa é larga, quando for
}

// Mid é o valor central da faixa — o que usamos para orçar.
func (c Coef) Mid() float64 { return (c.Min + c.Max) / 2 }

func (c Coef) validate(path string) error {
	if c.Min <= 0 {
		return fmt.Errorf("%s: coef.min deve ser > 0 (veio %v)", path, c.Min)
	}
	if c.Max < c.Min {
		return fmt.Errorf("%s: coef.max (%v) < coef.min (%v)", path, c.Max, c.Min)
	}
	if strings.TrimSpace(c.Unid) == "" {
		return fmt.Errorf("%s: coef.unid vazia — erro de unidade é o erro mais caro", path)
	}
	if c.Perda < 0 || c.Perda > 0.5 {
		return fmt.Errorf("%s: coef.perda fora de 0..0,5 (veio %v)", path, c.Perda)
	}
	return c.Fonte.validate(path + ".fonte")
}

// ---------------------------------------------------------------------------
// Material
// ---------------------------------------------------------------------------

// Material descreve como o item é VENDIDO — não como é consumido. A ponte entre
// consumo e compra é o arredondamento: 3,7 sacos de cimento não existem, são 4.
type Material struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`

	// Unidade em que a obra CONSOME o material (kg, m3, un, L, m).
	UnidBase string `json:"unid_base"`
	// Como a loja VENDE (ex "saco 50 kg"), e quanto de UnidBase vem em cada
	// embalagem (ex 50). Embalagem 1 + unid_base "un" = item avulso.
	UnidVenda    string  `json:"unid_venda"`
	ConteudoVend float64 `json:"conteudo_venda"`

	// Termo usado para casar com produto real no catálogo (search_products).
	BuscaCatalogo string `json:"busca_catalogo"`
	Categoria     string `json:"categoria"`

	Cura        string `json:"cura,omitempty"`
	Validade    string `json:"validade,omitempty"`
	Armazenagem string `json:"armazenagem,omitempty"`

	Fonte Source `json:"fonte"`
}

func (m Material) validate() error {
	p := "material[" + m.ID + "]"
	if m.ID == "" || m.Nome == "" {
		return fmt.Errorf("%s: id e nome são obrigatórios", p)
	}
	if m.UnidBase == "" || m.UnidVenda == "" {
		return fmt.Errorf("%s: unid_base e unid_venda são obrigatórias", p)
	}
	if m.ConteudoVend <= 0 {
		return fmt.Errorf("%s: conteudo_venda deve ser > 0 (veio %v)", p, m.ConteudoVend)
	}
	if m.BuscaCatalogo == "" {
		return fmt.Errorf("%s: busca_catalogo vazia — sem isso a Alice não acha o produto real", p)
	}
	return m.Fonte.validate(p + ".fonte")
}

// ---------------------------------------------------------------------------
// Ferramenta
// ---------------------------------------------------------------------------

type Ferramenta struct {
	ID            string `json:"id"`
	Nome          string `json:"nome"`
	Para          string `json:"para"`
	BuscaCatalogo string `json:"busca_catalogo"`
	EPI           bool   `json:"epi"`
}

func (f Ferramenta) validate() error {
	if f.ID == "" || f.Nome == "" || f.Para == "" {
		return fmt.Errorf("ferramenta[%s]: id, nome e para são obrigatórios", f.ID)
	}
	if f.BuscaCatalogo == "" {
		return fmt.Errorf("ferramenta[%s]: busca_catalogo vazia", f.ID)
	}
	return nil
}

// FerramentaRef liga um serviço a uma ferramenta, marcando o que é essencial
// (não dá pra executar sem) vs. desejável (acelera / melhora o acabamento).
type FerramentaRef struct {
	ID        string `json:"id"`
	Essencial bool   `json:"essencial"`
}

// ---------------------------------------------------------------------------
// Serviço
// ---------------------------------------------------------------------------

// Consumo liga um material a um serviço através de um coeficiente.
// Variante vazia = vale para todas as variantes do serviço.
type Consumo struct {
	MaterialID string `json:"material_id"`
	Variante   string `json:"variante,omitempty"`
	Coef       Coef   `json:"coef"`
	// Base sobrescreve a base do serviço para ESTE consumo. Existe porque um
	// serviço volumétrico consome itens por área: o contrapiso é orçado em m³
	// (cimento, areia), mas a lona plástica que vai embaixo dele é por m².
	// Vazio = usa a base do serviço.
	Base Base `json:"base,omitempty"`
}

// BaseEfetiva devolve a base que multiplica este consumo.
func (c Consumo) BaseEfetiva(s Servico) Base {
	if c.Base != "" {
		return c.Base
	}
	return s.Base
}

// Variante é uma forma alternativa de executar o serviço (bloco vs. tijolo),
// que muda os coeficientes mas não o passo a passo.
type Variante struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	// Dimensões nominais em metros, quando o item é modular (bloco/telha).
	// Servem para a memória de cálculo mostrar de onde saiu o coeficiente.
	Comprimento float64 `json:"comprimento,omitempty"`
	Altura      float64 `json:"altura,omitempty"`
	Junta       float64 `json:"junta,omitempty"`
	Padrao      bool    `json:"padrao,omitempty"`
}

// Base define qual grandeza multiplica os coeficientes do serviço.
type Base string

const (
	BaseArea   Base = "m2" // área: comprimento × altura (parede) ou área direta
	BaseVolume Base = "m3" // volume: área × espessura
	BaseLinear Base = "m"  // metro linear
)

// Calculadora escolhe a função pura que monta a lista de materiais. "linear" é
// o caso comum (base × coeficiente). As demais existem porque a conta NÃO é
// linear e fingir que é daria número errado.
type Calculadora string

const (
	CalcLinear      Calculadora = "linear"      // base × coef
	CalcCeramico    Calculadora = "ceramico"    // + peças e rejunte por geometria da placa
	CalcTelhado     Calculadora = "telhado"     // + fator de inclinação sobre a projeção
	CalcDrywall     Calculadora = "drywall"     // + montantes/guias por geometria real
	CalcPintura     Calculadora = "pintura"     // + nº de demãos
	CalcConcretagem Calculadora = "concretagem" // + traço de concreto sobre volume
)

var validCalc = map[Calculadora]bool{
	CalcLinear: true, CalcCeramico: true, CalcTelhado: true,
	CalcDrywall: true, CalcPintura: true, CalcConcretagem: true,
}

// Risco marca serviços que exigem tratamento especial na resposta.
// NÃO é decoração: RiscoEstrutural, RiscoEletrico e RiscoGas disparam
// encaminhamento obrigatório a profissional (ver package safety).
type Risco string

const (
	RiscoEstrutural Risco = "estrutural" // dimensionamento exige engenheiro/arquiteto
	RiscoEletrico   Risco = "eletrico"   // execução exige profissional habilitado
	RiscoGas        Risco = "gas"        // nunca instruir instalação
	RiscoAltura     Risco = "altura"     // EPI + NR-35
	RiscoQuimico    Risco = "quimico"    // solvente, cal, ventilação
)

// Servico é uma unidade de trabalho de obra que a Alice sabe explicar e orçar.
type Servico struct {
	ID         string   `json:"id"`
	Nome       string   `json:"nome"`
	Aliases    []string `json:"aliases,omitempty"`
	Oque       string   `json:"oque"`
	QuandoUsar string   `json:"quando_usar"`

	Base        Base        `json:"base"`
	Calculadora Calculadora `json:"calculadora"`
	// EspessuraPadrao em metros, para serviços de base m3 (contrapiso, reboco).
	EspessuraPadrao float64 `json:"espessura_padrao,omitempty"`
	EspessuraMin    float64 `json:"espessura_min,omitempty"`
	EspessuraMax    float64 `json:"espessura_max,omitempty"`

	Variantes   []Variante      `json:"variantes,omitempty"`
	Consumos    []Consumo       `json:"consumos"`
	Ferramentas []FerramentaRef `json:"ferramentas"`

	Sequencia   []string `json:"sequencia"`
	Cuidados    []string `json:"cuidados"`
	ErrosComuns []string `json:"erros_comuns"`
	Riscos      []Risco  `json:"riscos,omitempty"`
	// Depende lista serviços que precisam vir antes (usado em montar_lista_de_obra
	// para ordenar o orçamento na sequência real de execução).
	Depende []string `json:"depende,omitempty"`

	Fonte Source `json:"fonte"`
}

// VariantePadrao devolve a variante marcada como padrão (ou a primeira).
func (s Servico) VariantePadrao() string {
	for _, v := range s.Variantes {
		if v.Padrao {
			return v.ID
		}
	}
	if len(s.Variantes) > 0 {
		return s.Variantes[0].ID
	}
	return ""
}

// Variante busca uma variante pelo id.
func (s Servico) Variante(id string) (Variante, bool) {
	for _, v := range s.Variantes {
		if v.ID == id {
			return v, true
		}
	}
	return Variante{}, false
}

// ---------------------------------------------------------------------------
// Conversões
// ---------------------------------------------------------------------------

// Conversao é um fator entre duas unidades. Erro de unidade (saco vs. kg, m³
// vs. lata) é o erro mais comum e mais caro de obra — por isso ele é dado
// versionado com fonte, não conta de cabeça do modelo.
type Conversao struct {
	De     string  `json:"de"`
	Para   string  `json:"para"`
	Fator  float64 `json:"fator"` // 1 "de" = Fator "para"
	Nota   string  `json:"nota,omitempty"`
	Fonte  Source  `json:"fonte"`
	Aprox  bool    `json:"aprox,omitempty"` // depende de densidade/umidade
	Escopo string  `json:"escopo,omitempty"`
}

func (c Conversao) validate() error {
	p := fmt.Sprintf("conversao[%s→%s]", c.De, c.Para)
	if c.De == "" || c.Para == "" {
		return fmt.Errorf("%s: de/para obrigatórios", p)
	}
	if c.De == c.Para {
		return fmt.Errorf("%s: conversão de uma unidade para ela mesma", p)
	}
	if c.Fator <= 0 {
		return fmt.Errorf("%s: fator deve ser > 0 (veio %v)", p, c.Fator)
	}
	return c.Fonte.validate(p + ".fonte")
}

// ---------------------------------------------------------------------------
// Base carregada
// ---------------------------------------------------------------------------

// Base é a base de conhecimento já validada. Imutável depois do Load.
type KB struct {
	servicos    map[string]Servico
	materiais   map[string]Material
	ferramentas map[string]Ferramenta
	conversoes  []Conversao

	ordemServicos []string // ids ordenados (saída determinística)
	aliasIdx      map[string]string
}

// LeiaMe existe só para o DisallowUnknownFields aceitar o cabeçalho de
// documentação que cada arquivo de dados carrega. É doc para quem edita o JSON.
type fileServicos struct {
	LeiaMe   []string  `json:"_leia_me"`
	Servicos []Servico `json:"servicos"`
}
type fileMateriais struct {
	LeiaMe    []string   `json:"_leia_me"`
	Materiais []Material `json:"materiais"`
}
type fileFerramentas struct {
	LeiaMe      []string     `json:"_leia_me"`
	Ferramentas []Ferramenta `json:"ferramentas"`
}
type fileConversoes struct {
	LeiaMe     []string    `json:"_leia_me"`
	Conversoes []Conversao `json:"conversoes"`
}

// Load lê e VALIDA a base embutida. Chamada no boot: qualquer inconsistência
// (fonte ausente, coeficiente fora de faixa, material órfão) retorna erro e o
// serviço não sobe. É de propósito — passar batido é o modo de falha caro.
func Load() (*KB, error) { return LoadFS(dataFS) }

// LoadFS é o Load parametrizado pelo sistema de arquivos. Existe para os testes
// poderem alimentar dados PROPOSITALMENTE quebrados e provar que a validação
// barra — uma validação que nunca foi vista falhando não é uma validação.
func LoadFS(fsys fs.FS) (*KB, error) {
	var fsv fileServicos
	var fm fileMateriais
	var ff fileFerramentas
	var fc fileConversoes

	for _, spec := range []struct {
		name string
		out  any
	}{
		{"data/materiais.json", &fm},
		{"data/ferramentas.json", &ff},
		{"data/servicos.json", &fsv},
		{"data/conversoes.json", &fc},
	} {
		raw, err := fs.ReadFile(fsys, spec.name)
		if err != nil {
			return nil, fmt.Errorf("knowledge: lendo %s: %w", spec.name, err)
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		// Campo desconhecido é erro: protege contra typo silencioso ("perdaa":
		// 0.1) que viraria coeficiente sem perda sem ninguém perceber.
		dec.DisallowUnknownFields()
		if err := dec.Decode(spec.out); err != nil {
			return nil, fmt.Errorf("knowledge: parse %s: %w", spec.name, err)
		}
	}

	kb := &KB{
		servicos:    map[string]Servico{},
		materiais:   map[string]Material{},
		ferramentas: map[string]Ferramenta{},
		conversoes:  fc.Conversoes,
		aliasIdx:    map[string]string{},
	}

	for _, m := range fm.Materiais {
		if err := m.validate(); err != nil {
			return nil, fmt.Errorf("knowledge: %w", err)
		}
		if _, dup := kb.materiais[m.ID]; dup {
			return nil, fmt.Errorf("knowledge: material duplicado %q", m.ID)
		}
		kb.materiais[m.ID] = m
	}

	for _, f := range ff.Ferramentas {
		if err := f.validate(); err != nil {
			return nil, fmt.Errorf("knowledge: %w", err)
		}
		if _, dup := kb.ferramentas[f.ID]; dup {
			return nil, fmt.Errorf("knowledge: ferramenta duplicada %q", f.ID)
		}
		kb.ferramentas[f.ID] = f
	}

	for _, c := range fc.Conversoes {
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("knowledge: %w", err)
		}
	}

	for _, s := range fsv.Servicos {
		if err := kb.validateServico(s); err != nil {
			return nil, fmt.Errorf("knowledge: %w", err)
		}
		if _, dup := kb.servicos[s.ID]; dup {
			return nil, fmt.Errorf("knowledge: serviço duplicado %q", s.ID)
		}
		kb.servicos[s.ID] = s
		kb.ordemServicos = append(kb.ordemServicos, s.ID)
		kb.aliasIdx[norm(s.ID)] = s.ID
		kb.aliasIdx[norm(s.Nome)] = s.ID
		for _, a := range s.Aliases {
			kb.aliasIdx[norm(a)] = s.ID
		}
	}

	// Dependências só podem apontar para serviços que existem.
	for _, s := range kb.servicos {
		for _, d := range s.Depende {
			if _, ok := kb.servicos[d]; !ok {
				return nil, fmt.Errorf("knowledge: serviço %q depende de %q, que não existe", s.ID, d)
			}
		}
	}

	sort.Strings(kb.ordemServicos)
	return kb, nil
}

func (kb *KB) validateServico(s Servico) error {
	p := "servico[" + s.ID + "]"
	if s.ID == "" || s.Nome == "" {
		return fmt.Errorf("%s: id e nome são obrigatórios", p)
	}
	if s.Oque == "" || s.QuandoUsar == "" {
		return fmt.Errorf("%s: oque e quando_usar são obrigatórios", p)
	}
	switch s.Base {
	case BaseArea, BaseVolume, BaseLinear:
	default:
		return fmt.Errorf("%s: base inválida %q", p, s.Base)
	}
	if !validCalc[s.Calculadora] {
		return fmt.Errorf("%s: calculadora inválida %q", p, s.Calculadora)
	}
	if s.Base == BaseVolume {
		if s.EspessuraPadrao <= 0 {
			return fmt.Errorf("%s: base m3 exige espessura_padrao > 0", p)
		}
		if s.EspessuraMin <= 0 || s.EspessuraMax < s.EspessuraMin {
			return fmt.Errorf("%s: espessura_min/max inválidas", p)
		}
		if s.EspessuraPadrao < s.EspessuraMin || s.EspessuraPadrao > s.EspessuraMax {
			return fmt.Errorf("%s: espessura_padrao fora de [min,max]", p)
		}
	}
	if len(s.Consumos) == 0 {
		return fmt.Errorf("%s: sem consumos — serviço que não lista material não orça nada", p)
	}
	if len(s.Sequencia) == 0 {
		return fmt.Errorf("%s: sem sequencia de execução", p)
	}
	if len(s.Cuidados) == 0 {
		return fmt.Errorf("%s: sem cuidados", p)
	}
	if len(s.Ferramentas) == 0 {
		return fmt.Errorf("%s: sem ferramentas", p)
	}

	variantes := map[string]bool{}
	padroes := 0
	for _, v := range s.Variantes {
		if v.ID == "" || v.Nome == "" {
			return fmt.Errorf("%s: variante sem id/nome", p)
		}
		if variantes[v.ID] {
			return fmt.Errorf("%s: variante duplicada %q", p, v.ID)
		}
		variantes[v.ID] = true
		if v.Padrao {
			padroes++
		}
	}
	if len(s.Variantes) > 0 && padroes != 1 {
		return fmt.Errorf("%s: exatamente uma variante deve ser padrao (achei %d)", p, padroes)
	}

	for i, c := range s.Consumos {
		cp := fmt.Sprintf("%s.consumos[%d]", p, i)
		if _, ok := kb.materiais[c.MaterialID]; !ok {
			return fmt.Errorf("%s: material %q não existe em materiais.json", cp, c.MaterialID)
		}
		if c.Variante != "" && !variantes[c.Variante] {
			return fmt.Errorf("%s: variante %q não declarada no serviço", cp, c.Variante)
		}
		switch c.Base {
		case "", BaseArea, BaseVolume, BaseLinear:
		default:
			return fmt.Errorf("%s: base inválida %q", cp, c.Base)
		}
		// Consumo por m³ dentro de serviço que não tem espessura não tem como
		// ser calculado — pega no boot em vez de virar zero silencioso.
		if c.BaseEfetiva(s) == BaseVolume && s.EspessuraPadrao <= 0 {
			return fmt.Errorf("%s: consumo por m3 mas o serviço não define espessura_padrao", cp)
		}
		if err := c.Coef.validate(cp + ".coef"); err != nil {
			return err
		}
	}

	// Toda variante precisa render pelo menos um consumo próprio, senão orçar
	// aquela variante devolve a mesma lista da outra — bug silencioso.
	for v := range variantes {
		found := false
		for _, c := range s.Consumos {
			if c.Variante == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: variante %q não tem nenhum consumo específico", p, v)
		}
	}

	essenciais := 0
	for _, fr := range s.Ferramentas {
		if _, ok := kb.ferramentas[fr.ID]; !ok {
			return fmt.Errorf("%s: ferramenta %q não existe em ferramentas.json", p, fr.ID)
		}
		if fr.Essencial {
			essenciais++
		}
	}
	if essenciais == 0 {
		return fmt.Errorf("%s: nenhuma ferramenta marcada como essencial", p)
	}

	return s.Fonte.validate(p + ".fonte")
}

// ---------------------------------------------------------------------------
// Consultas
// ---------------------------------------------------------------------------

func (kb *KB) Servico(id string) (Servico, bool) {
	s, ok := kb.servicos[id]
	return s, ok
}

// ResolveServico aceita id, nome ou alias ("levantar parede", "muro", "alvenaria").
// Casa por normalização (minúsculas, sem acento) e, em último caso, por
// substring — o modelo não escreve o id exato sempre.
func (kb *KB) ResolveServico(termo string) (Servico, bool) {
	n := norm(termo)
	if n == "" {
		return Servico{}, false
	}
	if id, ok := kb.aliasIdx[n]; ok {
		return kb.servicos[id], true
	}
	// Substring: escolhe o alias mais longo que casa, para "assentar piso" não
	// perder para "piso" quando ambos existirem.
	best, bestLen := "", 0
	for alias, id := range kb.aliasIdx {
		if len(alias) > bestLen && (strings.Contains(n, alias) || strings.Contains(alias, n)) {
			best, bestLen = id, len(alias)
		}
	}
	if best != "" {
		return kb.servicos[best], true
	}
	return Servico{}, false
}

func (kb *KB) Material(id string) (Material, bool) {
	m, ok := kb.materiais[id]
	return m, ok
}

func (kb *KB) Ferramenta(id string) (Ferramenta, bool) {
	f, ok := kb.ferramentas[id]
	return f, ok
}

// Servicos devolve todos os serviços em ordem determinística.
func (kb *KB) Servicos() []Servico {
	out := make([]Servico, 0, len(kb.ordemServicos))
	for _, id := range kb.ordemServicos {
		out = append(out, kb.servicos[id])
	}
	return out
}

func (kb *KB) Conversoes() []Conversao { return kb.conversoes }

// Conversao acha o fator De→Para, direto ou pelo inverso.
func (kb *KB) Conversao(de, para string) (Conversao, bool) {
	nd, np := norm(de), norm(para)
	for _, c := range kb.conversoes {
		if norm(c.De) == nd && norm(c.Para) == np {
			return c, true
		}
	}
	for _, c := range kb.conversoes {
		if norm(c.De) == np && norm(c.Para) == nd {
			inv := c
			inv.De, inv.Para = c.Para, c.De
			inv.Fator = 1 / c.Fator
			return inv, true
		}
	}
	return Conversao{}, false
}

// UnidadesConhecidas lista as unidades aceitas por converter_unidade — usada
// para devolver erro útil ("unidade desconhecida; conheço: ...") em vez de zero.
func (kb *KB) UnidadesConhecidas() []string {
	set := map[string]bool{}
	for _, c := range kb.conversoes {
		set[c.De] = true
		set[c.Para] = true
	}
	out := make([]string, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// norm normaliza texto para busca: minúsculas, sem acento, espaços colapsados.
func norm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'á', 'à', 'â', 'ã', 'ä':
			b.WriteRune('a')
		case 'é', 'è', 'ê', 'ë':
			b.WriteRune('e')
		case 'í', 'ì', 'î', 'ï':
			b.WriteRune('i')
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			b.WriteRune('o')
		case 'ú', 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
