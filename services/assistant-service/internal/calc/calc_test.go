package calc_test

import (
	"math"
	"strings"
	"testing"

	"github.com/utilar/assistant-service/internal/calc"
	"github.com/utilar/assistant-service/internal/knowledge"
)

func kb(t *testing.T) *knowledge.KB {
	t.Helper()
	k, err := knowledge.Load()
	if err != nil {
		t.Fatalf("base não carregou: %v", err)
	}
	return k
}

func servico(t *testing.T, k *knowledge.KB, id string) knowledge.Servico {
	t.Helper()
	s, ok := k.Servico(id)
	if !ok {
		t.Fatalf("serviço %q não existe", id)
	}
	return s
}

func item(t *testing.T, r *calc.Resultado, materialID string) calc.Item {
	t.Helper()
	for _, it := range r.Itens {
		if it.MaterialID == materialID {
			return it
		}
	}
	t.Fatalf("material %q ausente do resultado (tem: %v)", materialID, ids(r))
	return calc.Item{}
}

func ids(r *calc.Resultado) []string {
	out := []string{}
	for _, it := range r.Itens {
		out = append(out, it.MaterialID)
	}
	return out
}

func perto(t *testing.T, nome string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, esperava %v (±%v)", nome, got, want, tol)
	}
}

// ---------------------------------------------------------------------------
// Caso conferível à mão: parede de 10 m² em bloco de concreto 14x19x39
// ---------------------------------------------------------------------------
//
// Conferência manual:
//   bloco: coeficiente médio (12,3+12,7)/2 = 12,5 blocos/m²
//          10 m² × 12,5 = 125 blocos, +5% de perda = 131,25 → compra 132
//   cimento: média (1,5+1,9)/2 = 1,7 kg/m²
//          10 × 1,7 = 17 kg, +10% = 18,7 kg → 1 saco de 50 kg
//   areia: média (0,010+0,012)/2 = 0,011 m³/m²
//          10 × 0,011 = 0,11 m³, +10% = 0,121 m³ → 1 m³

func TestCalcular_ParedeBloco10m2(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "alvenaria")

	r, err := calc.Calcular(k, s, "bloco-concreto-14", calc.Dims{Comprimento: 5, Altura: 2})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	perto(t, "área base", r.Base, 10, 0.001)

	bloco := item(t, r, "bloco-concreto-14")
	perto(t, "blocos", bloco.Quantidade, 131.25, 0.01)
	if bloco.Embalagens != 132 {
		t.Errorf("blocos a comprar = %d, esperava 132 (arredonda para cima)", bloco.Embalagens)
	}

	cim := item(t, r, "cimento-cp2")
	perto(t, "cimento kg", cim.Quantidade, 18.7, 0.01)
	if cim.Embalagens != 1 {
		t.Errorf("cimento = %d sacos, esperava 1", cim.Embalagens)
	}

	areia := item(t, r, "areia-media")
	perto(t, "areia m3", areia.Quantidade, 0.121, 0.001)

	// A memória de cálculo é requisito de produto: o cliente tem que conseguir
	// conferir de onde saiu o número.
	if !strings.Contains(bloco.Memoria, "12,5") {
		t.Errorf("memória de cálculo deveria mostrar o coeficiente; veio %q", bloco.Memoria)
	}
	if !strings.Contains(r.MemoriaBase, "5 m × 2 m") {
		t.Errorf("memória da base deveria mostrar a multiplicação; veio %q", r.MemoriaBase)
	}
	if bloco.Fonte == "" {
		t.Error("todo item precisa citar fonte")
	}
}

// A variante muda o material E o consumo. Tijolo maciço consome muito mais
// argamassa que bloco — se isso não aparecer, o orçamento está errado.
func TestCalcular_VarianteMudaMaterialEConsumo(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "alvenaria")

	bloco, err := calc.Calcular(k, s, "bloco-concreto-14", calc.Dims{Area: 10})
	if err != nil {
		t.Fatal(err)
	}
	macico, err := calc.Calcular(k, s, "tijolo-macico", calc.Dims{Area: 10})
	if err != nil {
		t.Fatal(err)
	}

	if item(t, macico, "tijolo-macico").Quantidade <= item(t, bloco, "bloco-concreto-14").Quantidade {
		t.Error("tijolo maciço deveria render muito mais peças por m² que o bloco")
	}
	if item(t, macico, "cimento-cp2").Quantidade <= item(t, bloco, "cimento-cp2").Quantidade {
		t.Error("tijolo maciço tem mais junta e deveria consumir mais cimento")
	}
}

func TestCalcular_VarianteInvalidaNaoPassaSilenciosamente(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "alvenaria")
	_, err := calc.Calcular(k, s, "bloco-de-ouro", calc.Dims{Area: 10})
	if err == nil {
		t.Fatal("variante inexistente deveria dar erro, não cair na padrão em silêncio")
	}
	if !strings.Contains(err.Error(), "bloco-de-ouro") {
		t.Errorf("erro deveria citar a variante pedida; veio %q", err)
	}
}

// ---------------------------------------------------------------------------
// Casos de borda das dimensões — o modelo pode alucinar qualquer coisa
// ---------------------------------------------------------------------------

func TestCalcular_DimensoesInvalidas(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "alvenaria")

	casos := []struct {
		nome string
		d    calc.Dims
	}{
		{"área zero (nada informado)", calc.Dims{}},
		{"área negativa", calc.Dims{Area: -10}},
		{"comprimento negativo", calc.Dims{Comprimento: -5, Altura: 2}},
		{"altura negativa", calc.Dims{Comprimento: 5, Altura: -2}},
		{"área absurda", calc.Dims{Area: 1e9}},
		{"comprimento absurdo", calc.Dims{Comprimento: 1e6, Altura: 2}},
		{"NaN", calc.Dims{Area: math.NaN()}},
		{"infinito", calc.Dims{Area: math.Inf(1)}},
		{"só o comprimento, sem altura", calc.Dims{Comprimento: 5}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := calc.Calcular(k, s, "", c.d); err == nil {
				t.Errorf("%s deveria ser rejeitado, mas passou", c.nome)
			}
		})
	}
}

// NaN merece teste próprio: ele atravessa qualquer comparação `>` sem disparar
// e contaminaria o orçamento inteiro em silêncio.
func TestCalcular_NaNNaoContamina(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "reboco")
	if _, err := calc.Calcular(k, s, "", calc.Dims{Area: 10, Espessura: math.NaN()}); err == nil {
		t.Fatal("espessura NaN deveria ser rejeitada")
	}
}

func TestCalcular_EspessuraForaDaFaixa(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "contrapiso")

	if _, err := calc.Calcular(k, s, "", calc.Dims{Area: 20, Espessura: 0.5}); err == nil {
		t.Error("contrapiso de 50 cm deveria ser recusado (fora da faixa usual)")
	}
	if _, err := calc.Calcular(k, s, "", calc.Dims{Area: 20, Espessura: 0.001}); err == nil {
		t.Error("contrapiso de 1 mm deveria ser recusado")
	}
}

// Sem espessura informada, o serviço volumétrico usa o padrão — mas AVISA.
// Silenciar isso esconderia do cliente uma premissa que dobra o orçamento.
func TestCalcular_EspessuraPadraoAvisa(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "contrapiso")

	r, err := calc.Calcular(k, s, "", calc.Dims{Area: 25})
	if err != nil {
		t.Fatal(err)
	}
	// 25 m² × 0,04 m = 1 m³
	perto(t, "volume", r.Base, 1.0, 0.001)
	if len(r.Avisos) == 0 {
		t.Error("deveria avisar que assumiu a espessura padrão")
	}
	// cimento: média (340+380)/2 = 360 kg/m³ × 1 m³ × 1,08 = 388,8 kg → 8 sacos
	cim := item(t, r, "cimento-cp2")
	perto(t, "cimento kg", cim.Quantidade, 388.8, 0.1)
	if cim.Embalagens != 8 {
		t.Errorf("cimento = %d sacos, esperava 8 (388,8 ÷ 50 = 7,78 → 8)", cim.Embalagens)
	}
}

// Consumo por m² dentro de serviço volumétrico (lona sob o contrapiso).
func TestCalcular_ConsumoPorAreaEmServicoVolumetrico(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "contrapiso")
	r, err := calc.Calcular(k, s, "", calc.Dims{Area: 100, Espessura: 0.04})
	if err != nil {
		t.Fatal(err)
	}
	lona := item(t, r, "lona-plastica")
	// média (1,05+1,15)/2 = 1,10 × 100 m² × 1,05 = 115,5 m² → 1 rolo de 200 m²
	perto(t, "lona m2", lona.Quantidade, 115.5, 0.1)
	if lona.Embalagens != 1 {
		t.Errorf("lona = %d rolos, esperava 1", lona.Embalagens)
	}
}

// ---------------------------------------------------------------------------
// Arredondamento para unidade de venda
// ---------------------------------------------------------------------------

func TestEmbalagens_ArredondaParaCima(t *testing.T) {
	casos := []struct {
		qtd, conteudo float64
		want          int
		porque        string
	}{
		{185, 50, 4, "3,7 sacos não existem — são 4"},
		{200, 50, 4, "exatamente 4 sacos continua 4"},
		{200.5, 50, 5, "meio saco a mais já obriga o quinto"},
		{1, 50, 1, "qualquer necessidade pede pelo menos 1 embalagem"},
		{0, 50, 0, "consumo zero não compra nada"},
		{-5, 50, 0, "consumo negativo não compra nada"},
		{10, 0, 0, "embalagem sem conteúdo é dado inválido"},
		{131.25, 1, 132, "item avulso arredonda a própria contagem"},
	}
	for _, c := range casos {
		if got := calc.Embalagens(c.qtd, c.conteudo); got != c.want {
			t.Errorf("Embalagens(%v, %v) = %d, esperava %d — %s", c.qtd, c.conteudo, got, c.want, c.porque)
		}
	}
}

// Ponto flutuante: 4,000000001 sacos não pode virar 5.
func TestEmbalagens_ToleranciaDePontoFlutuante(t *testing.T) {
	if got := calc.Embalagens(0.1+0.2, 0.3); got != 1 {
		t.Errorf("0,1+0,2 sobre 0,3 deveria dar 1 embalagem, veio %d", got)
	}
}

// ---------------------------------------------------------------------------
// Geometria: rejunte e telhado
// ---------------------------------------------------------------------------
//
// Conferência manual do rejunte, placa 30×30 cm, junta 3 mm, profundidade 8 mm:
//   (0,30 + 0,30) ÷ (0,30 × 0,30) = 0,6 ÷ 0,09 = 6,667 1/m
//   6,667 × 0,003 m × 0,008 m = 1,60e-4 m³/m²
//   1,60e-4 × 1600 kg/m³ = 0,256 kg/m²

func TestRejunteKgPorM2_ConferivelAMao(t *testing.T) {
	got := calc.RejunteKgPorM2(0.30, 0.30, 3, 8)
	perto(t, "rejunte 30x30", got, 0.256, 0.001)

	// Peça maior tem menos junta por m² — o consumo tem que CAIR.
	grande := calc.RejunteKgPorM2(0.60, 0.60, 3, 8)
	if grande >= got {
		t.Errorf("peça 60x60 (%v) deveria consumir menos rejunte que 30x30 (%v)", grande, got)
	}
	perto(t, "rejunte 60x60", grande, 0.128, 0.001)

	// Junta mais larga consome mais, proporcionalmente.
	larga := calc.RejunteKgPorM2(0.30, 0.30, 6, 8)
	perto(t, "junta dobrada dobra o consumo", larga, 2*got, 0.001)
}

func TestRejunteKgPorM2_EntradaInvalida(t *testing.T) {
	for _, c := range [][4]float64{
		{0, 0.3, 3, 8}, {0.3, 0, 3, 8}, {0.3, 0.3, 0, 8}, {0.3, 0.3, 3, 0},
		{-0.3, 0.3, 3, 8},
	} {
		if got := calc.RejunteKgPorM2(c[0], c[1], c[2], c[3]); got != 0 {
			t.Errorf("RejunteKgPorM2%v deveria dar 0, veio %v", c, got)
		}
	}
}

// Informando a geometria da placa, o cálculo do rejunte deixa de usar a faixa
// grosseira de tabela e passa a usar a fórmula — e a memória tem que dizer isso.
func TestCalcular_RejuntePorGeometriaSobrescreveTabela(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "assentar-piso")

	semPlaca, err := calc.Calcular(k, s, "ceramica", calc.Dims{Area: 20})
	if err != nil {
		t.Fatal(err)
	}
	comPlaca, err := calc.Calcular(k, s, "ceramica", calc.Dims{
		Area: 20, PlacaComprimento: 0.30, PlacaLargura: 0.30, JuntaMM: 3, ProfundidadeMM: 8,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 20 m² × 0,256 kg/m² × 1,10 de perda = 5,632 kg
	perto(t, "rejunte com geometria", item(t, comPlaca, "rejunte-cimenticio").Quantidade, 5.632, 0.01)

	memGeo := item(t, comPlaca, "rejunte-cimenticio").Memoria
	if !strings.Contains(memGeo, "placa de") {
		t.Errorf("memória deveria explicitar a geometria da placa; veio %q", memGeo)
	}
	if item(t, semPlaca, "rejunte-cimenticio").Memoria == memGeo {
		t.Error("sem as medidas da placa o cálculo deveria usar o coeficiente de tabela")
	}
}

// Telhado: a área real é maior que a projetada. Ignorar isso compra telha a menos.
func TestFatorInclinacao(t *testing.T) {
	perto(t, "telhado plano", calc.FatorInclinacao(0), 1.0, 1e-9)
	perto(t, "inclinação negativa é tratada como plana", calc.FatorInclinacao(-10), 1.0, 1e-9)
	// 30% → √(1 + 0,3²) = √1,09 = 1,04403
	perto(t, "30%", calc.FatorInclinacao(30), 1.04403, 1e-5)
	// 100% (45°) → √2
	perto(t, "100%", calc.FatorInclinacao(100), math.Sqrt2, 1e-9)
}

func TestCalcular_TelhadoUsaAreaReal(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "telhado")

	r, err := calc.Calcular(k, s, "portuguesa", calc.Dims{Area: 100, InclinacaoPct: 30})
	if err != nil {
		t.Fatal(err)
	}
	perto(t, "área real do telhado", r.Base, 104.403, 0.01)

	// telha: média (16+17)/2 = 16,5 /m² × 104,403 × 1,05 = 1808,8
	perto(t, "telhas", item(t, r, "telha-ceramica-portuguesa").Quantidade, 1808.79, 1)

	// Sem inclinação informada, assume 30% mas avisa — inclinação mínima é
	// questão de infiltração, não de estética.
	semIncl, err := calc.Calcular(k, s, "portuguesa", calc.Dims{Area: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(semIncl.Avisos) == 0 {
		t.Error("deveria avisar que assumiu a inclinação")
	}

	// Colonial usa capa e canal: tem que dar muito mais peças.
	col, err := calc.Calcular(k, s, "colonial", calc.Dims{Area: 100, InclinacaoPct: 30})
	if err != nil {
		t.Fatal(err)
	}
	if item(t, col, "telha-ceramica-colonial").Quantidade <= item(t, r, "telha-ceramica-portuguesa").Quantidade {
		t.Error("telha colonial (capa + canal) deveria dar mais peças que a portuguesa")
	}
}

// ---------------------------------------------------------------------------
// Drywall: geometria real substitui a faixa
// ---------------------------------------------------------------------------
//
// Parede de 5 m × 2,7 m = 13,5 m²
//   guias: 2 × 5 m = 10 m, +10% = 11 m → 4 barras de 3 m
//   montantes a cada 0,60 m: floor(5/0,6)+1 = 9 prumadas × 2,7 m = 24,3 m,
//     +10% = 26,73 m → 9 barras de 3 m

func TestCalcular_DrywallGeometriaExata(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "drywall")

	r, err := calc.Calcular(k, s, "parede", calc.Dims{Comprimento: 5, Altura: 2.7})
	if err != nil {
		t.Fatal(err)
	}

	guia := item(t, r, "guia-48")
	perto(t, "guias m", guia.Quantidade, 11.0, 0.01)
	if guia.Embalagens != 4 {
		t.Errorf("guias = %d barras, esperava 4 (11 m ÷ 3 m = 3,67 → 4)", guia.Embalagens)
	}

	mont := item(t, r, "montante-48")
	perto(t, "montantes m", mont.Quantidade, 26.73, 0.01)
	if mont.Embalagens != 9 {
		t.Errorf("montantes = %d barras, esperava 9 (26,73 ÷ 3 = 8,91 → 9)", mont.Embalagens)
	}

	// Chapa: 13,5 m² × 2 faces × 1,10 = 29,7 m² → 14 chapas de 2,16 m²
	chapa := item(t, r, "chapa-drywall-st")
	perto(t, "chapa m2", chapa.Quantidade, 29.7, 0.01)
	if chapa.Embalagens != 14 {
		t.Errorf("chapas = %d, esperava 14 (29,7 ÷ 2,16 = 13,75 → 14)", chapa.Embalagens)
	}

	// Espaçamento de 40 cm (revestimento cerâmico sobre drywall) usa mais montante.
	apertado, err := calc.Calcular(k, s, "parede", calc.Dims{Comprimento: 5, Altura: 2.7, EspacamentoMontante: 0.40})
	if err != nil {
		t.Fatal(err)
	}
	if item(t, apertado, "montante-48").Quantidade <= mont.Quantidade {
		t.Error("espaçamento de 40 cm deveria consumir mais montante que 60 cm")
	}
}

func TestCalcular_DrywallForroFechaUmaFace(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "drywall")

	parede, _ := calc.Calcular(k, s, "parede", calc.Dims{Area: 20})
	forro, err := calc.Calcular(k, s, "forro", calc.Dims{Area: 20})
	if err != nil {
		t.Fatal(err)
	}
	pq := item(t, parede, "chapa-drywall-st").Quantidade
	fq := item(t, forro, "chapa-drywall-st").Quantidade
	perto(t, "parede consome o dobro do forro", pq, 2*fq, 0.01)
}

// ---------------------------------------------------------------------------
// Pintura: demãos multiplicam a tinta, mas não o selador
// ---------------------------------------------------------------------------

func TestCalcular_PinturaDemaos(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "pintura-interna")

	duas, err := calc.Calcular(k, s, "acrilica", calc.Dims{Area: 50, Demaos: 2})
	if err != nil {
		t.Fatal(err)
	}
	tres, err := calc.Calcular(k, s, "acrilica", calc.Dims{Area: 50, Demaos: 3})
	if err != nil {
		t.Fatal(err)
	}

	// tinta: média (0,07+0,11)/2 = 0,09 L/m² por demão
	// 50 m² × 0,09 × 2 demãos × 1,05 = 9,45 L → 1 lata de 18 L
	tinta2 := item(t, duas, "tinta-latex-acrilica")
	perto(t, "tinta 2 demãos", tinta2.Quantidade, 9.45, 0.01)
	if tinta2.Embalagens != 1 {
		t.Errorf("tinta = %d latas, esperava 1", tinta2.Embalagens)
	}

	tinta3 := item(t, tres, "tinta-latex-acrilica")
	perto(t, "3 demãos = 1,5× a tinta de 2", tinta3.Quantidade, tinta2.Quantidade*1.5, 0.01)

	// O selador é uma demão só: não pode escalar com as demãos de tinta.
	if item(t, duas, "selador-acrilico").Quantidade != item(t, tres, "selador-acrilico").Quantidade {
		t.Error("selador é demão única — não deveria mudar com o número de demãos de tinta")
	}

	if _, err := calc.Calcular(k, s, "acrilica", calc.Dims{Area: 50, Demaos: 99}); err == nil {
		t.Error("99 demãos deveria ser recusado")
	}
	if _, err := calc.Calcular(k, s, "acrilica", calc.Dims{Area: 50, Demaos: -1}); err == nil {
		t.Error("demãos negativas deveriam ser recusadas")
	}
}

func TestCalcular_PinturaSemDemaosAssumeDoisEAvisa(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "pintura-interna")
	r, err := calc.Calcular(k, s, "acrilica", calc.Dims{Area: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Avisos) == 0 {
		t.Error("deveria avisar que assumiu 2 demãos")
	}
	perto(t, "tinta com 2 demãos assumidas", item(t, r, "tinta-latex-acrilica").Quantidade, 9.45, 0.01)
}

// ---------------------------------------------------------------------------
// Serviço linear
// ---------------------------------------------------------------------------

func TestCalcular_ServicoLinear(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "hidraulica-basica")

	r, err := calc.Calcular(k, s, "", calc.Dims{Comprimento: 30})
	if err != nil {
		t.Fatal(err)
	}
	// tubo: média (1,0+1,15)/2 = 1,075 m/m × 30 × 1,10 = 35,475 m → 6 barras de 6 m
	tubo := item(t, r, "tubo-pvc-soldavel-25")
	perto(t, "tubo m", tubo.Quantidade, 35.475, 0.01)
	if tubo.Embalagens != 6 {
		t.Errorf("tubo = %d barras, esperava 6 (35,475 ÷ 6 = 5,91 → 6)", tubo.Embalagens)
	}
	if r.BaseUnid != "m" {
		t.Errorf("base de serviço linear deveria ser m, veio %q", r.BaseUnid)
	}
	if _, err := calc.Calcular(k, s, "", calc.Dims{}); err == nil {
		t.Error("serviço linear sem comprimento deveria dar erro")
	}
}

// ---------------------------------------------------------------------------
// Ferramentas e EPI
// ---------------------------------------------------------------------------

func TestCalcular_SeparaFerramentasEEPI(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "telhado")
	r, err := calc.Calcular(k, s, "portuguesa", calc.Dims{Area: 50, InclinacaoPct: 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.FerramentasEssenciais) == 0 {
		t.Error("deveria listar ferramentas essenciais")
	}
	if len(r.EPI) == 0 {
		t.Error("telhado sem EPI listado é falha de segurança")
	}
	// EPI não pode aparecer misturado como "ferramenta desejável".
	for _, f := range r.FerramentasDesejaveis {
		if strings.Contains(strings.ToLower(f), "cinturão") {
			t.Error("cinturão de segurança não pode ser listado como desejável")
		}
	}
}

// ---------------------------------------------------------------------------
// Consolidação de vários serviços
// ---------------------------------------------------------------------------

func TestConsolidar_SomaMaterialRepetido(t *testing.T) {
	k := kb(t)

	alv, err := calc.Calcular(k, servico(t, k, "alvenaria"), "bloco-concreto-14", calc.Dims{Area: 10})
	if err != nil {
		t.Fatal(err)
	}
	cont, err := calc.Calcular(k, servico(t, k, "contrapiso"), "", calc.Dims{Area: 25, Espessura: 0.04})
	if err != nil {
		t.Fatal(err)
	}

	c := calc.Consolidar(k, []*calc.Resultado{alv, cont})

	// Cimento aparece nos dois serviços e tem que virar UMA linha.
	n := 0
	var cimento calc.Item
	for _, it := range c.Itens {
		if it.MaterialID == "cimento-cp2" {
			n++
			cimento = it
		}
	}
	if n != 1 {
		t.Fatalf("cimento apareceu %d vezes; deveria ser consolidado em 1 linha", n)
	}

	// 18,7 kg (alvenaria) + 388,8 kg (contrapiso) = 407,5 kg → 9 sacos
	perto(t, "cimento total", cimento.Quantidade, 407.5, 0.1)
	if cimento.Embalagens != 9 {
		t.Errorf("cimento consolidado = %d sacos, esperava 9 (407,5 ÷ 50 = 8,15 → 9)", cimento.Embalagens)
	}
	if !strings.Contains(cimento.Memoria, "somado de 2 serviços") {
		t.Errorf("memória deveria explicar a soma; veio %q", cimento.Memoria)
	}
}

// Arredondar por serviço e só depois somar compraria sobra a cada linha.
// A soma tem que acontecer no CONSUMO, e o arredondamento uma vez só, no fim.
func TestConsolidar_ArredondaDepoisDeSomar(t *testing.T) {
	k := kb(t)
	s := servico(t, k, "alvenaria")

	// Duas paredes pequenas: cada uma sozinha já obriga 1 saco de cimento,
	// mas juntas ainda cabem em 1 saco.
	a, _ := calc.Calcular(k, s, "bloco-concreto-14", calc.Dims{Area: 5})
	b, _ := calc.Calcular(k, s, "bloco-concreto-14", calc.Dims{Area: 5})

	if a.Itens[1].Embalagens+b.Itens[1].Embalagens != 2 {
		t.Fatal("pré-condição do teste: cada parede deveria pedir 1 saco isoladamente")
	}

	c := calc.Consolidar(k, []*calc.Resultado{a, b})
	for _, it := range c.Itens {
		if it.MaterialID == "cimento-cp2" {
			if it.Embalagens != 1 {
				t.Errorf("consolidado = %d sacos; somando o consumo (18,7 kg) cabe em 1", it.Embalagens)
			}
		}
	}
}

func TestConsolidar_JuntaFerramentasEEPISemRepetir(t *testing.T) {
	k := kb(t)
	a, _ := calc.Calcular(k, servico(t, k, "alvenaria"), "bloco-concreto-14", calc.Dims{Area: 10})
	b, _ := calc.Calcular(k, servico(t, k, "reboco"), "", calc.Dims{Area: 10, Espessura: 0.02})

	c := calc.Consolidar(k, []*calc.Resultado{a, b})
	visto := map[string]bool{}
	for _, f := range c.FerramentasEssenciais {
		if visto[f] {
			t.Errorf("ferramenta repetida na consolidação: %s", f)
		}
		visto[f] = true
	}
	if len(c.EPI) == 0 {
		t.Error("consolidação deveria juntar o EPI de todos os serviços")
	}
}
