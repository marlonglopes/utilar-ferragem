package knowledge_test

// Testes NEGATIVOS da validação de boot.
//
// A validação do package knowledge só vale alguma coisa se alguém já a viu
// BARRAR dado quebrado. Estes testes alimentam LoadFS com bases propositalmente
// erradas — uma mutação por teste — e exigem erro. O caso de controle
// (TestValidacao_BaseMinimaValidaCarrega) existe para provar que os negativos
// falham pela regra testada, e não porque a base de apoio já era inválida.

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/utilar/assistant-service/internal/knowledge"
)

// ---------------------------------------------------------------------------
// Andaime: base mínima válida + uma mutação por teste
// ---------------------------------------------------------------------------

// baseKB é a base de apoio em memória. Cada teste parte de baseMinima() (sempre
// recém-construída, nunca compartilhada) e aplica UMA mutação.
type baseKB struct {
	Materiais   []knowledge.Material
	Ferramentas []knowledge.Ferramenta
	Servicos    []knowledge.Servico
	Conversoes  []knowledge.Conversao
}

func fonteOK() knowledge.Source {
	return knowledge.Source{
		Tipo: knowledge.KindMercado,
		Ref:  "consumo típico de mercado",
		Nota: "Prática corrente de obra; sem respaldo normativo.",
	}
}

func coefOK() knowledge.Coef {
	return knowledge.Coef{
		Min:   1.5,
		Max:   1.9,
		Unid:  "kg/m2",
		Perda: 0.1,
		Fonte: fonteOK(),
	}
}

func baseMinima() baseKB {
	return baseKB{
		Materiais: []knowledge.Material{{
			ID:            "cimento-cp2",
			Nome:          "Cimento Portland CP II (saco 50 kg)",
			UnidBase:      "kg",
			UnidVenda:     "saco 50 kg",
			ConteudoVend:  50,
			BuscaCatalogo: "cimento",
			Categoria:     "construcao",
			Fonte:         fonteOK(),
		}},
		Ferramentas: []knowledge.Ferramenta{{
			ID:            "colher-de-pedreiro",
			Nome:          "Colher de pedreiro",
			Para:          "Aplicar e espalhar argamassa",
			BuscaCatalogo: "colher de pedreiro",
		}},
		Servicos: []knowledge.Servico{{
			ID:          "alvenaria",
			Nome:        "Levantar parede",
			Oque:        "Erguer uma parede assentando blocos com argamassa.",
			QuandoUsar:  "Fechar um vão ou dividir um ambiente.",
			Base:        knowledge.BaseArea,
			Calculadora: knowledge.CalcLinear,
			Consumos: []knowledge.Consumo{{
				MaterialID: "cimento-cp2",
				Coef:       coefOK(),
			}},
			Ferramentas: []knowledge.FerramentaRef{{
				ID: "colher-de-pedreiro", Essencial: true,
			}},
			Sequencia: []string{"Marque a parede no piso."},
			Cuidados:  []string{"Molhe a base antes de assentar."},
			Fonte:     fonteOK(),
		}},
		Conversoes: []knowledge.Conversao{{
			De:    "saco de cimento",
			Para:  "kg",
			Fator: 50,
			Fonte: knowledge.Source{
				Tipo: knowledge.KindDefinicao,
				Ref:  "embalagem padrão de venda",
				Nota: "Saco de 50 kg é a embalagem padrão no varejo brasileiro.",
			},
		}},
	}
}

// arquivos serializa a base para os quatro JSONs que o LoadFS espera.
func (b baseKB) arquivos(t *testing.T) map[string]string {
	t.Helper()
	enc := func(chave string, v any) string {
		raw, err := json.Marshal(map[string]any{chave: v})
		if err != nil {
			t.Fatalf("serializando %s: %v", chave, err)
		}
		return string(raw)
	}
	return map[string]string{
		"data/materiais.json":   enc("materiais", b.Materiais),
		"data/ferramentas.json": enc("ferramentas", b.Ferramentas),
		"data/servicos.json":    enc("servicos", b.Servicos),
		"data/conversoes.json":  enc("conversoes", b.Conversoes),
	}
}

func carregar(t *testing.T, b baseKB) (*knowledge.KB, error) {
	t.Helper()
	return carregarArquivos(t, b.arquivos(t))
}

func carregarArquivos(t *testing.T, arquivos map[string]string) (*knowledge.KB, error) {
	t.Helper()
	fsys := fstest.MapFS{}
	for nome, conteudo := range arquivos {
		fsys[nome] = &fstest.MapFile{Data: []byte(conteudo)}
	}
	return knowledge.LoadFS(fsys)
}

// exigeErro falha o teste se a validação deixou passar, ou se a mensagem não
// aponta o culpado (erro que não diz qual campo/id quebrou não ajuda ninguém).
func exigeErro(t *testing.T, err error, pistas ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("a validação deveria ter barrado esta base, mas carregou sem erro")
	}
	for _, pista := range pistas {
		if !strings.Contains(err.Error(), pista) {
			t.Errorf("erro deveria citar %q; veio: %v", pista, err)
		}
	}
}

// servicoComVariantes devolve um serviço com duas variantes, cada uma com
// consumo próprio — base de apoio para os testes de variante.
func servicoComVariantes() knowledge.Servico {
	s := baseMinima().Servicos[0]
	s.Variantes = []knowledge.Variante{
		{ID: "bloco-14", Nome: "Bloco de concreto 14 cm", Padrao: true},
		{ID: "bloco-9", Nome: "Bloco de concreto 9 cm"},
	}
	s.Consumos = []knowledge.Consumo{
		{MaterialID: "cimento-cp2", Variante: "bloco-14", Coef: coefOK()},
		{MaterialID: "cimento-cp2", Variante: "bloco-9", Coef: coefOK()},
	}
	return s
}

// ---------------------------------------------------------------------------
// 1. Controle
// ---------------------------------------------------------------------------

// Sem este teste os negativos não provam nada: eles poderiam estar falhando por
// um defeito da base de apoio, não pela regra sob teste.
func TestValidacao_BaseMinimaValidaCarrega(t *testing.T) {
	kb, err := carregar(t, baseMinima())
	if err != nil {
		t.Fatalf("base mínima correta deveria carregar: %v", err)
	}
	if _, ok := kb.Servico("alvenaria"); !ok {
		t.Error("serviço da base mínima não foi indexado")
	}
	if _, ok := kb.Material("cimento-cp2"); !ok {
		t.Error("material da base mínima não foi indexado")
	}
}

// ---------------------------------------------------------------------------
// 2–10. Fonte e coeficiente
// ---------------------------------------------------------------------------

// Regra: número sem procedência é chute. A Alice tem que poder dizer de onde
// tirou o coeficiente quando o cliente perguntar.
func TestValidacao_CoeficienteSemFonte(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Fonte = knowledge.Source{}

	_, err := carregar(t, b)
	exigeErro(t, err, "fonte")
}

// Regra: a nota diz o que a referência de fato cobre. "ABNT NBR 6136" sem nota
// deixa o leitor achar que a norma publica consumo — ela não publica.
func TestValidacao_FonteSemNota(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Fonte.Nota = "   "

	_, err := carregar(t, b)
	exigeErro(t, err, "fonte.nota")
}

// Regra: coeficiente zero ou negativo não existe em obra — significa que o
// campo não foi preenchido, e passaria batido virando lista de material vazia.
func TestValidacao_CoeficienteMinZeroOuNegativo(t *testing.T) {
	for nome, min := range map[string]float64{"zero": 0, "negativo": -1.2} {
		t.Run(nome, func(t *testing.T) {
			b := baseMinima()
			b.Servicos[0].Consumos[0].Coef.Min = min

			_, err := carregar(t, b)
			exigeErro(t, err, "coef.min")
		})
	}
}

// Regra: faixa invertida (max < min) é dado digitado trocado. Mid() devolveria
// um número plausível e ninguém veria o erro.
func TestValidacao_FaixaInvertida(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Min = 9
	b.Servicos[0].Consumos[0].Coef.Max = 2

	_, err := carregar(t, b)
	exigeErro(t, err, "coef.max")
}

// Regra: erro de unidade é o erro mais caro de obra (kg vs. saco, m² vs. m³).
// Coeficiente sem unidade é número solto.
func TestValidacao_CoeficienteSemUnidade(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Unid = "  "

	_, err := carregar(t, b)
	exigeErro(t, err, "coef.unid")
}

// Regra: perda é fração (0,05 = 5%). Fora de 0..0,5 quase sempre é alguém
// escrevendo 10 querendo dizer 10% — o que decuplicaria a compra.
func TestValidacao_PerdaForaDaFaixa(t *testing.T) {
	for nome, perda := range map[string]float64{"negativa": -0.01, "percentual inteiro": 10} {
		t.Run(nome, func(t *testing.T) {
			b := baseMinima()
			b.Servicos[0].Consumos[0].Coef.Perda = perda

			_, err := carregar(t, b)
			exigeErro(t, err, "coef.perda")
		})
	}
}

// Regra: o tipo da fonte é um conjunto fechado. Rótulo inventado esconde o quão
// confiável é o número.
func TestValidacao_TipoDeFonteDesconhecido(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Fonte.Tipo = knowledge.SourceKind("achismo")

	_, err := carregar(t, b)
	exigeErro(t, err, "fonte.tipo")
}

// Regra anti-inventar-norma: se está marcado tipo=norma, a referência tem que
// parecer norma (NBR/NR-). Citar norma que não existe é o pior erro possível.
func TestValidacao_TipoNormaComRefQueNaoEhNorma(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Fonte = knowledge.Source{
		Tipo: knowledge.KindNorma,
		Ref:  "manual do fabricante de argamassa",
		Nota: "Faixa declarada na embalagem.",
	}

	_, err := carregar(t, b)
	exigeErro(t, err, "não parece norma")
}

// Regra inversa: NBR citada sob tipo=mercado empresta autoridade normativa a um
// número que é só prática de obra.
func TestValidacao_TipoMercadoCitandoNBR(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].Coef.Fonte = knowledge.Source{
		Tipo: knowledge.KindMercado,
		Ref:  "traço conforme ABNT NBR 8545",
		Nota: "Consumo típico de obra.",
	}

	_, err := carregar(t, b)
	exigeErro(t, err, "NBR")
}

// ---------------------------------------------------------------------------
// 11–17. Integridade referencial e forma do serviço
// ---------------------------------------------------------------------------

// Regra: material órfão vira item sem preço, sem embalagem e sem busca no
// catálogo — o orçamento sairia com uma linha fantasma.
func TestValidacao_ServicoComMaterialInexistente(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos[0].MaterialID = "cimento-que-nao-existe"

	_, err := carregar(t, b)
	exigeErro(t, err, "cimento-que-nao-existe", "materiais.json")
}

// Regra: ferramenta órfã sumiria em silêncio da lista de ferramentas.
func TestValidacao_ServicoComFerramentaInexistente(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Ferramentas = []knowledge.FerramentaRef{
		{ID: "britadeira-imaginaria", Essencial: true},
	}

	_, err := carregar(t, b)
	exigeErro(t, err, "britadeira-imaginaria", "ferramentas.json")
}

// Regra: serviço sem consumo não orça nada — devolveria lista de compras vazia
// para um pedido perfeitamente legítimo.
func TestValidacao_ServicoSemConsumos(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Consumos = nil

	_, err := carregar(t, b)
	exigeErro(t, err, "sem consumos")
}

// Regra: se nenhuma ferramenta é essencial, a Alice não sabe dizer o mínimo
// necessário para executar — vira lista de desejáveis.
func TestValidacao_ServicoSemFerramentaEssencial(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Ferramentas = []knowledge.FerramentaRef{
		{ID: "colher-de-pedreiro", Essencial: false},
	}

	_, err := carregar(t, b)
	exigeErro(t, err, "essencial")
}

// Regra: serviço de base m3 multiplica área × espessura. Sem espessura_padrao o
// volume daria zero e a lista de material sairia zerada em silêncio.
func TestValidacao_BaseM3SemEspessuraPadrao(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Base = knowledge.BaseVolume
	b.Servicos[0].EspessuraPadrao = 0

	_, err := carregar(t, b)
	exigeErro(t, err, "espessura")
}

// Regra: com variantes, exatamente uma é padrão. Zero deixa a escolha ao acaso;
// duas fazem a "padrão" depender da ordem do arquivo.
func TestValidacao_VariantesSemUmaUnicaPadrao(t *testing.T) {
	t.Run("nenhuma padrao", func(t *testing.T) {
		b := baseMinima()
		s := servicoComVariantes()
		s.Variantes[0].Padrao = false
		b.Servicos[0] = s

		_, err := carregar(t, b)
		exigeErro(t, err, "padrao")
	})

	t.Run("duas padrao", func(t *testing.T) {
		b := baseMinima()
		s := servicoComVariantes()
		s.Variantes[1].Padrao = true
		b.Servicos[0] = s

		_, err := carregar(t, b)
		exigeErro(t, err, "padrao")
	})
}

// Regra: variante sem consumo próprio devolveria, em silêncio, a lista da outra
// variante — o cliente pediria tijolo e receberia orçamento de bloco.
func TestValidacao_VarianteSemConsumoProprio(t *testing.T) {
	b := baseMinima()
	s := servicoComVariantes()
	s.Consumos = s.Consumos[:1] // sobra só o consumo de "bloco-14"
	b.Servicos[0] = s

	_, err := carregar(t, b)
	exigeErro(t, err, "bloco-9")
}

// ---------------------------------------------------------------------------
// 18–19. Material comprável
// ---------------------------------------------------------------------------

// Regra: sem conteudo_venda não dá para arredondar consumo para embalagem —
// 3,7 sacos de cimento não existem, são 4.
func TestValidacao_MaterialSemConteudoDeVenda(t *testing.T) {
	for nome, conteudo := range map[string]float64{"zero": 0, "negativo": -50} {
		t.Run(nome, func(t *testing.T) {
			b := baseMinima()
			b.Materiais[0].ConteudoVend = conteudo

			_, err := carregar(t, b)
			exigeErro(t, err, "conteudo_venda")
		})
	}
}

// Regra: sem busca_catalogo a Alice não acha o produto real no catalog-service
// e o item fica sem preço.
func TestValidacao_MaterialSemBuscaCatalogo(t *testing.T) {
	b := baseMinima()
	b.Materiais[0].BuscaCatalogo = ""

	_, err := carregar(t, b)
	exigeErro(t, err, "busca_catalogo")
}

// ---------------------------------------------------------------------------
// 20–23. Integridade do arquivo
// ---------------------------------------------------------------------------

// Regra: id duplicado faz o último vencer em silêncio — o registro anterior
// (possivelmente o correto) simplesmente desaparece do mapa.
func TestValidacao_IDsDuplicados(t *testing.T) {
	t.Run("material duplicado", func(t *testing.T) {
		b := baseMinima()
		b.Materiais = append(b.Materiais, b.Materiais[0])

		_, err := carregar(t, b)
		exigeErro(t, err, "duplicado", "cimento-cp2")
	})

	t.Run("servico duplicado", func(t *testing.T) {
		b := baseMinima()
		b.Servicos = append(b.Servicos, b.Servicos[0])

		_, err := carregar(t, b)
		exigeErro(t, err, "duplicado", "alvenaria")
	})
}

// Regra (DisallowUnknownFields): um typo como "perdaa" seria ignorado pelo
// decoder padrão e o coeficiente entraria com perda = 0 — compra a menos, obra
// parada. O campo desconhecido tem que derrubar o boot.
func TestValidacao_CampoDesconhecidoNoJSON(t *testing.T) {
	arquivos := baseMinima().arquivos(t)
	arquivos["data/servicos.json"] = strings.Replace(
		arquivos["data/servicos.json"], `"perda"`, `"perdaa"`, 1)

	_, err := carregarArquivos(t, arquivos)
	exigeErro(t, err, "perdaa")
}

// Regra: dependência apontando para serviço inexistente quebraria a ordenação
// de montar_lista_de_obra — etapa citada que ninguém sabe orçar.
func TestValidacao_DependeDeServicoInexistente(t *testing.T) {
	b := baseMinima()
	b.Servicos[0].Depende = []string{"fundacao-que-nao-existe"}

	_, err := carregar(t, b)
	exigeErro(t, err, "fundacao-que-nao-existe")
}

// Regra: JSON quebrado tem que falhar no boot com a mensagem apontando o
// arquivo, não virar base vazia servida ao cliente.
func TestValidacao_JSONMalformado(t *testing.T) {
	arquivos := baseMinima().arquivos(t)
	arquivos["data/servicos.json"] = `{"servicos": [ {"id": "alvenaria",`

	_, err := carregarArquivos(t, arquivos)
	exigeErro(t, err, "servicos.json")
}
