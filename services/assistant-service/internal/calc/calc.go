// Package calc são as calculadoras de material da Alice.
//
// REGRA: todo cálculo aqui é FUNÇÃO PURA em Go, testada com casos conferíveis à
// mão. Nada de conta dentro do prompt — modelo de linguagem erra aritmética em
// silêncio, e aqui um erro vira cimento a menos (obra parada) ou a mais
// (dinheiro perdido, e cimento vence em 3 meses).
//
// Os coeficientes NÃO moram aqui: vêm do package knowledge, que os carrega de
// dados versionados com fonte. Este package só aplica geometria e aritmética.
//
// Toda quantidade sai acompanhada da MEMÓRIA DE CÁLCULO. O cliente precisa
// poder conferir de onde veio o número — número sem origem é indistinguível de
// chute, e a Alice não dá chute.
package calc

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/utilar/assistant-service/internal/knowledge"
)

// ---------------------------------------------------------------------------
// Entrada e validação
// ---------------------------------------------------------------------------

// Limites de sanidade. O modelo pode alucinar argumento (dimensão negativa,
// "10000000 m²", string onde se espera número), então nada entra sem passar
// por aqui. Os tetos são generosos para obra real e apertados o bastante para
// que um argumento absurdo vire erro claro em vez de um orçamento delirante.
const (
	MaxDimensaoLinear = 500.0   // m — parede de 500 m já é obra industrial
	MaxArea           = 20000.0 // m²
	MaxEspessura      = 2.0     // m
	MaxDemaos         = 5
	MaxInclinacaoPct  = 100.0 // 100% = 45°
	MinPlacaM         = 0.02  // 2 cm — pastilha
	MaxPlacaM         = 3.0   // 3 m — placa grande de porcelanato
	MaxJuntaMM        = 30.0
)

// Dims são as dimensões informadas pelo cliente. Campos zerados significam
// "não informado" e o cálculo cai no padrão do serviço (ou avisa).
type Dims struct {
	Comprimento float64 // m
	Altura      float64 // m
	Area        float64 // m² — alternativa a comprimento×altura
	Espessura   float64 // m — serviços volumétricos
	Perimetro   float64 // m — fôrma de calçada

	Demaos        int     // pintura
	InclinacaoPct float64 // telhado, em % (30% ≈ 16,7°)

	PlacaComprimento float64 // m — placa cerâmica
	PlacaLargura     float64 // m
	JuntaMM          float64 // mm — largura da junta de rejunte
	ProfundidadeMM   float64 // mm — profundidade da junta

	EspacamentoMontante float64 // m — drywall (0,40 ou 0,60)
}

// ErrValidacao é erro de argumento — vira mensagem clara para o modelo (e para
// o cliente), nunca um número silenciosamente errado.
type ErrValidacao struct{ Msg string }

func (e ErrValidacao) Error() string { return e.Msg }

func errf(format string, a ...any) error { return ErrValidacao{Msg: fmt.Sprintf(format, a...)} }

// validarNumero rejeita NaN, infinito, negativo e absurdo de uma vez só.
// NaN merece atenção especial: ele passa por qualquer comparação `>` sem
// disparar, e contaminaria o orçamento inteiro em silêncio.
func validarNumero(nome string, v, max float64, obrigatorio bool) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return errf("%s: valor inválido (não é um número)", nome)
	}
	if v == 0 {
		if obrigatorio {
			return errf("%s: preciso desse valor para calcular", nome)
		}
		return nil
	}
	if v < 0 {
		return errf("%s: não existe dimensão negativa (veio %g)", nome, v)
	}
	if v > max {
		return errf("%s: %g está fora da faixa que eu calculo (máximo %g). "+
			"Se a obra é desse porte mesmo, vale um profissional dimensionar", nome, v, max)
	}
	return nil
}

// Validar checa as dimensões contra o serviço pedido. Roda ANTES de qualquer
// conta.
func (d Dims) Validar(s knowledge.Servico) error {
	for _, c := range []struct {
		nome string
		v    float64
		max  float64
	}{
		{"comprimento", d.Comprimento, MaxDimensaoLinear},
		{"altura", d.Altura, MaxDimensaoLinear},
		{"área", d.Area, MaxArea},
		{"perímetro", d.Perimetro, MaxDimensaoLinear * 4},
		{"espessura", d.Espessura, MaxEspessura},
	} {
		if err := validarNumero(c.nome, c.v, c.max, false); err != nil {
			return err
		}
	}

	if d.Demaos < 0 || d.Demaos > MaxDemaos {
		return errf("demãos: %d está fora do razoável (1 a %d)", d.Demaos, MaxDemaos)
	}
	if err := validarNumero("inclinação", d.InclinacaoPct, MaxInclinacaoPct, false); err != nil {
		return err
	}
	for _, c := range []struct {
		nome string
		v    float64
	}{
		{"comprimento da placa", d.PlacaComprimento},
		{"largura da placa", d.PlacaLargura},
	} {
		if c.v == 0 {
			continue
		}
		if err := validarNumero(c.nome, c.v, MaxPlacaM, false); err != nil {
			return err
		}
		if c.v < MinPlacaM {
			return errf("%s: %g m é pequeno demais para ser uma placa", c.nome, c.v)
		}
	}
	if err := validarNumero("junta", d.JuntaMM, MaxJuntaMM, false); err != nil {
		return err
	}
	if err := validarNumero("profundidade da junta", d.ProfundidadeMM, MaxJuntaMM, false); err != nil {
		return err
	}
	if d.EspacamentoMontante != 0 && (d.EspacamentoMontante < 0.2 || d.EspacamentoMontante > 1.0) {
		return errf("espaçamento dos montantes: %g m está fora do usual (0,40 a 0,60 m)", d.EspacamentoMontante)
	}

	// A grandeza principal do serviço tem que ser determinável.
	if _, _, err := d.grandezaBase(s); err != nil {
		return err
	}
	return nil
}

// grandezaBase resolve a área (ou o comprimento) sobre a qual o serviço incide,
// devolvendo também a memória de como chegou lá.
func (d Dims) grandezaBase(s knowledge.Servico) (float64, string, error) {
	switch s.Base {
	case knowledge.BaseLinear:
		v := d.Comprimento
		if v == 0 {
			v = d.Perimetro
		}
		if v <= 0 {
			return 0, "", errf("preciso do comprimento (em metros) para calcular %s", s.Nome)
		}
		return v, fmt.Sprintf("comprimento informado: %s m", num(v)), nil

	default: // m2 e m3 partem sempre de uma área
		if d.Area > 0 {
			return d.Area, fmt.Sprintf("área informada: %s m²", num(d.Area)), nil
		}
		if d.Comprimento > 0 && d.Altura > 0 {
			a := d.Comprimento * d.Altura
			return a, fmt.Sprintf("área = %s m × %s m = %s m²",
				num(d.Comprimento), num(d.Altura), num(a)), nil
		}
		return 0, "", errf("preciso da área em m², ou do comprimento e da altura, para calcular %s", s.Nome)
	}
}

// ---------------------------------------------------------------------------
// Saída
// ---------------------------------------------------------------------------

// Item é uma linha da lista de materiais.
type Item struct {
	MaterialID string `json:"material_id"`
	Nome       string `json:"nome"`

	// Consumo calculado, na unidade em que a obra consome (kg, m³, un…).
	Quantidade float64 `json:"quantidade"`
	UnidBase   string  `json:"unid_base"`

	// Quanto COMPRAR: embalagens inteiras. 3,7 sacos não existem — são 4.
	Embalagens int    `json:"embalagens"`
	UnidVenda  string `json:"unid_venda"`

	// Memória de cálculo desta linha, em português, conferível à mão.
	Memoria string `json:"memoria"`
	// Fonte do coeficiente, para a Alice citar.
	Fonte string `json:"fonte"`
	// Faixa do coeficiente (min–max) — a honestidade da estimativa.
	CoefMin  float64 `json:"coef_min"`
	CoefMax  float64 `json:"coef_max"`
	CoefUnid string  `json:"coef_unid"`
	// Motivo de a faixa ser larga, quando houver.
	Observacao string `json:"observacao,omitempty"`
}

// Resultado é a saída de uma calculadora.
type Resultado struct {
	ServicoID   string `json:"servico_id"`
	ServicoNome string `json:"servico_nome"`
	Variante    string `json:"variante,omitempty"`

	Base        float64 `json:"base"`
	BaseUnid    string  `json:"base_unid"`
	MemoriaBase string  `json:"memoria_base"`

	Itens  []Item   `json:"itens"`
	Avisos []string `json:"avisos,omitempty"`

	FerramentasEssenciais []string `json:"ferramentas_essenciais,omitempty"`
	FerramentasDesejaveis []string `json:"ferramentas_desejaveis,omitempty"`
	EPI                   []string `json:"epi,omitempty"`
}

// ---------------------------------------------------------------------------
// Calculadora principal
// ---------------------------------------------------------------------------

// Calcular monta a lista de materiais de um serviço. Função pura: mesmas
// entradas, mesma saída, sem I/O.
func Calcular(kb *knowledge.KB, s knowledge.Servico, variante string, d Dims) (*Resultado, error) {
	if err := d.Validar(s); err != nil {
		return nil, err
	}

	// Variante: vazia usa a padrão; inválida é erro (não silenciosamente ignorada,
	// senão o cliente orça bloco de concreto achando que pediu tijolo).
	if variante == "" {
		variante = s.VariantePadrao()
	} else if len(s.Variantes) > 0 {
		if _, ok := s.Variante(variante); !ok {
			nomes := make([]string, 0, len(s.Variantes))
			for _, v := range s.Variantes {
				nomes = append(nomes, v.ID)
			}
			return nil, errf("não conheço a variante %q de %s. As que eu tenho: %s",
				variante, s.Nome, strings.Join(nomes, ", "))
		}
	}

	area, memBase, err := d.grandezaBase(s)
	if err != nil {
		return nil, err
	}

	res := &Resultado{
		ServicoID:   s.ID,
		ServicoNome: s.Nome,
		Variante:    variante,
	}

	// Ajustes por calculadora que mudam a GRANDEZA base (não os coeficientes).
	switch s.Calculadora {
	case knowledge.CalcTelhado:
		incl := d.InclinacaoPct
		if incl == 0 {
			incl = 30 // inclinação corrente de telhado cerâmico
			res.Avisos = append(res.Avisos,
				"Considerei 30% de inclinação (valor corrente). Me diga a inclinação real do seu telhado — "+
					"ela muda a área e, mais importante, cada tipo de telha tem uma inclinação MÍNIMA que precisa ser respeitada, "+
					"senão dá infiltração.")
		}
		fator := FatorInclinacao(incl)
		areaReal := area * fator
		memBase += fmt.Sprintf("; área real do telhado = %s m² × %s (fator de inclinação de %s%%) = %s m²",
			num(area), num(fator), num(incl), num(areaReal))
		area = areaReal
	}

	res.Base = area
	res.MemoriaBase = memBase
	res.BaseUnid = "m²"
	if s.Base == knowledge.BaseLinear {
		res.BaseUnid = "m"
	}

	// Volume, para serviços volumétricos.
	espessura := d.Espessura
	var volume float64
	if s.Base == knowledge.BaseVolume {
		if espessura == 0 {
			espessura = s.EspessuraPadrao
			res.Avisos = append(res.Avisos, fmt.Sprintf(
				"Usei a espessura padrão de %s cm. Se a sua for diferente, me diga — a espessura multiplica "+
					"direto o consumo de material.", num(espessura*100)))
		}
		if espessura < s.EspessuraMin || espessura > s.EspessuraMax {
			return nil, errf("espessura de %s cm está fora da faixa usual de %s (%s a %s cm). "+
				"Confirme a medida, ou consulte um profissional se o caso for atípico",
				num(espessura*100), s.Nome, num(s.EspessuraMin*100), num(s.EspessuraMax*100))
		}
		volume = area * espessura
		res.Base = volume
		res.BaseUnid = "m³"
		res.MemoriaBase = memBase + fmt.Sprintf("; volume = %s m² × %s m de espessura = %s m³",
			num(area), num(espessura), num(volume))
	}

	demaos := d.Demaos
	if s.Calculadora == knowledge.CalcPintura && demaos == 0 {
		demaos = 2
		res.Avisos = append(res.Avisos,
			"Considerei 2 demãos, que é o usual. Cor forte sobre parede clara costuma pedir 3.")
	}

	// Percorre os consumos aplicáveis à variante escolhida.
	for _, c := range s.Consumos {
		if c.Variante != "" && c.Variante != variante {
			continue
		}
		m, ok := kb.Material(c.MaterialID)
		if !ok {
			continue // impossível: o Load valida. Defesa em profundidade.
		}

		base := area
		baseDesc := fmt.Sprintf("%s m²", num(area))
		switch c.BaseEfetiva(s) {
		case knowledge.BaseVolume:
			base = volume
			baseDesc = fmt.Sprintf("%s m³", num(volume))
		case knowledge.BaseLinear:
			base = area // em serviço linear, `area` já é o comprimento
			baseDesc = fmt.Sprintf("%s m", num(area))
		}

		coef := c.Coef.Mid()
		qtd := base * coef
		memoria := fmt.Sprintf("%s × %s %s", baseDesc, num(coef), c.Coef.Unid)

		// Tinta é por demão: multiplica. Selador e massa, não.
		if strings.Contains(c.Coef.Unid, "demão") && demaos > 0 {
			qtd *= float64(demaos)
			memoria += fmt.Sprintf(" × %d demãos", demaos)
		}

		// Perda declarada no coeficiente.
		if c.Coef.Perda > 0 {
			qtd *= 1 + c.Coef.Perda
			memoria += fmt.Sprintf(" + %s%% de perda", num(c.Coef.Perda*100))
		}

		// Sobrescritas geométricas: onde a conta NÃO é linear, uma função
		// dedicada substitui o coeficiente de tabela — e diz que substituiu.
		if q, mem, ok := sobrescreverGeometria(s, c, m, d, area, demaos); ok {
			qtd, memoria = q, mem
		}

		memoria += fmt.Sprintf(" = %s %s", num(qtd), m.UnidBase)

		it := Item{
			MaterialID: m.ID,
			Nome:       m.Nome,
			Quantidade: arred(qtd, 3),
			UnidBase:   m.UnidBase,
			Embalagens: Embalagens(qtd, m.ConteudoVend),
			UnidVenda:  m.UnidVenda,
			Memoria:    memoria,
			Fonte:      c.Coef.Fonte.Human() + " — " + c.Coef.Fonte.Nota,
			CoefMin:    c.Coef.Min,
			CoefMax:    c.Coef.Max,
			CoefUnid:   c.Coef.Unid,
			Observacao: c.Coef.Motivo,
		}
		if it.Embalagens > 0 && m.ConteudoVend != 1 {
			it.Memoria += fmt.Sprintf(" → %d %s (arredondado para cima: não dá para comprar fração de %s)",
				it.Embalagens, plural(m.UnidVenda, it.Embalagens), m.UnidVenda)
		}
		res.Itens = append(res.Itens, it)
	}

	if len(res.Itens) == 0 {
		return nil, errf("não achei consumo cadastrado para %s na variante %q", s.Nome, variante)
	}

	preencherFerramentas(kb, s, res)
	return res, nil
}

// sobrescreverGeometria aplica as contas que NÃO são lineares. Só age quando o
// cliente informou as dimensões necessárias; sem elas, o coeficiente de tabela
// (declaradamente grosseiro) continua valendo.
func sobrescreverGeometria(
	s knowledge.Servico, c knowledge.Consumo, m knowledge.Material,
	d Dims, area float64, demaos int,
) (float64, string, bool) {
	switch {
	// Rejunte: o consumo depende da geometria da placa e da junta, não de um
	// coeficiente médio. Com as medidas em mãos, a fórmula é bem mais precisa.
	case s.Calculadora == knowledge.CalcCeramico && m.ID == "rejunte-cimenticio":
		if d.PlacaComprimento <= 0 || d.PlacaLargura <= 0 {
			return 0, "", false
		}
		junta := d.JuntaMM
		if junta <= 0 {
			junta = 3
		}
		prof := d.ProfundidadeMM
		if prof <= 0 {
			prof = 8
		}
		kgM2 := RejunteKgPorM2(d.PlacaComprimento, d.PlacaLargura, junta, prof)
		qtd := area * kgM2 * (1 + c.Coef.Perda)
		mem := fmt.Sprintf(
			"placa de %s × %s m, junta de %s mm e %s mm de profundidade → "+
				"(%s + %s) ÷ (%s × %s) × %s m × %s m × 1600 kg/m³ = %s kg/m²; "+
				"%s m² × %s kg/m² + %s%% de perda",
			num(d.PlacaComprimento), num(d.PlacaLargura), num(junta), num(prof),
			num(d.PlacaComprimento), num(d.PlacaLargura), num(d.PlacaComprimento), num(d.PlacaLargura),
			num(junta/1000), num(prof/1000), num(kgM2),
			num(area), num(kgM2), num(c.Coef.Perda*100))
		return qtd, mem, true

	// Guias de drywall: são exatamente 2 por parede (piso e teto). Sabendo a
	// altura, o consumo por m² é 2 ÷ altura — exato, sem faixa.
	case s.Calculadora == knowledge.CalcDrywall && m.ID == "guia-48":
		if d.Altura <= 0 || d.Comprimento <= 0 {
			return 0, "", false
		}
		metros := 2 * d.Comprimento * (1 + c.Coef.Perda)
		mem := fmt.Sprintf(
			"2 guias (piso e teto) × %s m de parede = %s m + %s%% de perda",
			num(d.Comprimento), num(2*d.Comprimento), num(c.Coef.Perda*100))
		return metros, mem, true

	// Montantes: o consumo é o número de prumadas × a altura. O espaçamento é
	// escolha de projeto, então usamos o informado (ou 0,60 m).
	case s.Calculadora == knowledge.CalcDrywall && m.ID == "montante-48":
		if d.Altura <= 0 || d.Comprimento <= 0 {
			return 0, "", false
		}
		esp := d.EspacamentoMontante
		if esp <= 0 {
			esp = 0.60
		}
		n := math.Floor(d.Comprimento/esp) + 1
		metros := n * d.Altura * (1 + c.Coef.Perda)
		mem := fmt.Sprintf(
			"montantes a cada %s m em %s m de parede = %s prumadas × %s m de altura = %s m + %s%% de perda",
			num(esp), num(d.Comprimento), num(n), num(d.Altura), num(n*d.Altura), num(c.Coef.Perda*100))
		return metros, mem, true

	// Fôrma de calçada: acompanha o PERÍMETRO, não a área. Com o perímetro
	// informado o número é exato, e a faixa larga do coeficiente some.
	case s.Calculadora == knowledge.CalcConcretagem && m.ID == "tabua-pinus":
		if d.Perimetro <= 0 {
			return 0, "", false
		}
		metros := d.Perimetro * (1 + c.Coef.Perda)
		mem := fmt.Sprintf("perímetro informado: %s m + %s%% de perda",
			num(d.Perimetro), num(c.Coef.Perda*100))
		return metros, mem, true
	}
	return 0, "", false
}

func preencherFerramentas(kb *knowledge.KB, s knowledge.Servico, res *Resultado) {
	for _, fr := range s.Ferramentas {
		f, ok := kb.Ferramenta(fr.ID)
		if !ok {
			continue
		}
		switch {
		case f.EPI:
			res.EPI = append(res.EPI, f.Nome)
		case fr.Essencial:
			res.FerramentasEssenciais = append(res.FerramentasEssenciais, f.Nome)
		default:
			res.FerramentasDesejaveis = append(res.FerramentasDesejaveis, f.Nome)
		}
	}
}

// ---------------------------------------------------------------------------
// Funções puras auxiliares (testadas isoladamente)
// ---------------------------------------------------------------------------

// Embalagens converte consumo em quantidade de embalagens a COMPRAR,
// arredondando sempre para cima. É o elo entre "preciso de 3,7 sacos" e
// "compre 4" — arredondar para baixo faria a obra parar.
func Embalagens(qtd, conteudo float64) int {
	if qtd <= 0 || conteudo <= 0 {
		return 0
	}
	n := math.Ceil(qtd/conteudo - 1e-9) // tolerância: 4,0000000001 não vira 5
	if n < 1 {
		n = 1
	}
	if math.IsInf(n, 0) || n > 1e9 {
		return 0
	}
	return int(n)
}

// FatorInclinacao converte área projetada em área real de telhado.
// Uma inclinação de i% significa i metros de subida a cada 100 de projeção,
// logo o comprimento real da água é √(1 + (i/100)²) vezes a projeção.
func FatorInclinacao(pct float64) float64 {
	if pct <= 0 {
		return 1
	}
	t := pct / 100
	return math.Sqrt(1 + t*t)
}

// densidadeRejunte é a massa específica aparente do rejunte cimentício
// endurecido. Valor típico declarado por fabricantes.
const densidadeRejunte = 1600.0 // kg/m³

// RejunteKgPorM2 calcula o consumo de rejunte pela geometria real da junta.
//
// Dedução: cada placa C×L contribui com meia junta em cada um dos seus quatro
// lados, o que equivale a uma junta inteira ao longo de C e outra ao longo de L.
// Volume de junta por placa = (C + L) × largura × profundidade.
// Dividindo pela área da placa (C × L) chega-se ao volume por m², e a massa sai
// multiplicando pela massa específica.
//
// C e L em metros; junta e profundidade em MILÍMETROS (é como o cliente fala).
func RejunteKgPorM2(placaC, placaL, juntaMM, profundidadeMM float64) float64 {
	if placaC <= 0 || placaL <= 0 || juntaMM <= 0 || profundidadeMM <= 0 {
		return 0
	}
	volPorM2 := (placaC + placaL) / (placaC * placaL) * (juntaMM / 1000) * (profundidadeMM / 1000)
	return volPorM2 * densidadeRejunte
}

// AreaParede é a área de uma parede retangular.
func AreaParede(comprimento, altura float64) float64 { return comprimento * altura }

// Volume é área × espessura.
func Volume(area, espessura float64) float64 { return area * espessura }

// arred arredonda para n casas — só para apresentação, nunca no meio da conta.
func arred(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// num formata número em pt-BR (vírgula decimal), sem zeros à toa.
func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		s = fmt.Sprintf("%.0f", v)
	} else if math.Abs(v) < 0.1 {
		s = fmt.Sprintf("%.4f", v)
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	} else {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return strings.ReplaceAll(s, ".", ",")
}

// plural pluraliza a unidade de venda de forma simples ("saco 50 kg" → "sacos
// 50 kg"). Cobre os casos da base; não pretende ser um pluralizador geral.
func plural(unid string, n int) string {
	if n <= 1 {
		return unid
	}
	campos := strings.SplitN(unid, " ", 2)
	cab := campos[0]
	switch {
	case strings.HasSuffix(cab, "ão"):
		cab = strings.TrimSuffix(cab, "ão") + "ões"
	case strings.HasSuffix(cab, "l"):
		cab = strings.TrimSuffix(cab, "l") + "is"
	case strings.HasSuffix(cab, "m"):
		cab = strings.TrimSuffix(cab, "m") + "ns"
	case strings.HasSuffix(cab, "s"):
		// já plural ou invariável
	default:
		cab += "s"
	}
	if len(campos) == 2 {
		return cab + " " + campos[1]
	}
	return cab
}

// ---------------------------------------------------------------------------
// Consolidação de vários serviços (lista de obra)
// ---------------------------------------------------------------------------

// Consolidado é o orçamento de vários serviços com os materiais somados.
type Consolidado struct {
	Servicos []*Resultado `json:"servicos"`
	// Materiais somados: não pedir cimento duas vezes.
	Itens  []Item   `json:"itens"`
	Avisos []string `json:"avisos,omitempty"`

	FerramentasEssenciais []string `json:"ferramentas_essenciais,omitempty"`
	EPI                   []string `json:"epi,omitempty"`
}

// Consolidar soma os materiais repetidos entre serviços. A soma é feita no
// CONSUMO (unidade base) e só depois arredondada para embalagem — somar
// embalagens já arredondadas compraria sobra a cada serviço.
func Consolidar(kb *knowledge.KB, resultados []*Resultado) *Consolidado {
	out := &Consolidado{Servicos: resultados}

	type acc struct {
		item     Item
		total    float64
		memorias []string
	}
	porMaterial := map[string]*acc{}
	ordem := []string{}

	for _, r := range resultados {
		out.Avisos = append(out.Avisos, r.Avisos...)
		for _, it := range r.Itens {
			a, ok := porMaterial[it.MaterialID]
			if !ok {
				a = &acc{item: it}
				porMaterial[it.MaterialID] = a
				ordem = append(ordem, it.MaterialID)
			}
			a.total += it.Quantidade
			a.memorias = append(a.memorias, fmt.Sprintf("%s: %s", r.ServicoNome, it.Memoria))
		}
	}

	for _, id := range ordem {
		a := porMaterial[id]
		m, _ := kb.Material(id)
		it := a.item
		it.Quantidade = arred(a.total, 3)
		it.Embalagens = Embalagens(a.total, m.ConteudoVend)
		if len(a.memorias) > 1 {
			it.Memoria = fmt.Sprintf("somado de %d serviços — %s. Total: %s %s → %d %s",
				len(a.memorias), strings.Join(a.memorias, " | "),
				num(a.total), m.UnidBase, it.Embalagens, plural(m.UnidVenda, it.Embalagens))
		}
		out.Itens = append(out.Itens, it)
	}

	// Ferramentas e EPI da obra inteira, sem repetir.
	essSet, epiSet := map[string]bool{}, map[string]bool{}
	for _, r := range resultados {
		for _, f := range r.FerramentasEssenciais {
			essSet[f] = true
		}
		for _, e := range r.EPI {
			epiSet[e] = true
		}
	}
	out.FerramentasEssenciais = ordenado(essSet)
	out.EPI = ordenado(epiSet)
	return out
}

func ordenado(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
