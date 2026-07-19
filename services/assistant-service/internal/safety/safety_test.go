package safety_test

import (
	"strings"
	"testing"

	"github.com/utilar/assistant-service/internal/safety"
)

func temCategoria(achados []safety.Achado, c safety.Categoria) bool {
	for _, a := range achados {
		if a.Categoria == c {
			return true
		}
	}
	return false
}

// O teste central do package: pergunta sobre DIMENSIONAMENTO ESTRUTURAL tem que
// produzir encaminhamento a profissional. Se este teste passar a falhar, a Alice
// está livre para dar bitola de ferro de viga — e isso derruba construção.
func TestDimensionamentoEstrutural_EncaminhaAProfissional(t *testing.T) {
	perguntas := []string{
		"qual a bitola de ferro para uma viga de 4 metros?",
		"quantos ferros vão numa laje de 5x4?",
		"que espessura deve ter a laje da minha garagem?",
		"como dimensionar o pilar da minha varanda?",
		"qual a profundidade da sapata para uma casa de 2 andares?",
		"preciso calcular a armadura de um muro de arrimo",
		"quero fazer uma viga de concreto armado, qual o cálculo estrutural?",
		"minha laje aguenta o peso de uma caixa d'água?",
		"qual vergalhão usar na cinta de amarração?",
		"que aço CA-50 eu uso na fundação?",
	}
	for _, p := range perguntas {
		t.Run(p, func(t *testing.T) {
			achados := safety.Analisar(p)
			if !temCategoria(achados, safety.Estrutural) {
				t.Fatalf("pergunta estrutural NÃO detectada: %q", p)
			}
			if !safety.Bloqueia(achados) {
				t.Errorf("deveria bloquear dimensionamento: %q", p)
			}
			texto := safety.TextoAvisos(achados)
			if !strings.Contains(texto, "engenheiro") && !strings.Contains(texto, "arquiteto") {
				t.Errorf("aviso deveria encaminhar a engenheiro/arquiteto; veio %q", texto)
			}
			instr := safety.InstrucaoParaModelo(achados)
			if !strings.Contains(instr, "NÃO") {
				t.Errorf("instrução ao modelo deveria conter proibição explícita; veio %q", instr)
			}
		})
	}
}

// Derrubar parede é o caso clássico de risco escondido: quem pergunta acha que
// é reforma, e pode ser estrutura.
func TestDemolicao_NaoDaVeredito(t *testing.T) {
	perguntas := []string{
		"posso derrubar a parede entre a sala e a cozinha?",
		"quero remover parede pra juntar os cômodos",
		"como abrir um vão nessa parede?",
		"quero demolir uma parede da sala",
	}
	for _, p := range perguntas {
		achados := safety.Analisar(p)
		if !temCategoria(achados, safety.Demolicao) && !temCategoria(achados, safety.Estrutural) {
			t.Errorf("pergunta de demolição não detectada: %q", p)
		}
		if !safety.Bloqueia(achados) {
			t.Errorf("demolição deveria bloquear veredito: %q", p)
		}
	}
}

// Gás é o único caso de RECUSA, não de aviso: a Alice não instrui instalação.
func TestGas_RecusaInstruirInstalacao(t *testing.T) {
	perguntas := []string{
		"como instalar o gás encanado da cozinha?",
		"quero ligar o fogão a gás, como faço?",
		"como trocar a mangueira de gás?",
		"instalação de central de GLP",
		"como instalar aquecedor a gás?",
	}
	for _, p := range perguntas {
		if !safety.RecusaGas(p) {
			t.Errorf("pergunta de gás não detectada: %q", p)
		}
		texto := safety.TextoAvisos(safety.Analisar(p))
		if !strings.Contains(texto, "credenciado") {
			t.Errorf("deveria encaminhar a instalador credenciado; veio %q", texto)
		}
	}
}

// "gas" é curto e vive dentro de outras palavras. Sem limite de palavra, a Alice
// recusaria falar de "gasto de material" ou "desgaste da broca".
func TestGas_NaoDisparaEmPalavraQueContemGas(t *testing.T) {
	for _, p := range []string{
		"qual o gasto de material nessa obra?",
		"o desgaste da broca é normal?",
		"quanto custa a gasolina do gerador?",
	} {
		if safety.RecusaGas(p) {
			t.Errorf("falso positivo de gás em %q", p)
		}
	}
}

func TestEletrico_ExigeProfissionalHabilitado(t *testing.T) {
	for _, p := range []string{
		"como fazer a instalação elétrica do banheiro?",
		"qual bitola de cabo pro chuveiro elétrico?",
		"como ligar o disjuntor no quadro de distribuição?",
		"preciso passar fiação nova",
	} {
		achados := safety.Analisar(p)
		if !temCategoria(achados, safety.Eletrico) {
			t.Errorf("pergunta elétrica não detectada: %q", p)
		}
		texto := safety.TextoAvisos(achados)
		if !strings.Contains(texto, "profissional habilitado") {
			t.Errorf("deveria exigir profissional habilitado; veio %q", texto)
		}
	}
}

func TestAltura_MencionaEPIeRisco(t *testing.T) {
	for _, p := range []string{
		"como colocar telha no telhado?",
		"preciso montar andaime pra pintar a fachada",
		"vou limpar a calha",
	} {
		achados := safety.Analisar(p)
		if !temCategoria(achados, safety.Altura) {
			t.Errorf("risco de altura não detectado: %q", p)
		}
		texto := safety.TextoAvisos(achados)
		if !strings.Contains(texto, "NR-35") {
			t.Errorf("aviso de altura deveria citar a NR-35; veio %q", texto)
		}
	}
}

// A Alice não pode virar um disclaimer ambulante: aviso em toda resposta treina
// o cliente a ignorar TODOS os avisos, inclusive os que salvam vida.
func TestPerguntaComum_NaoDisparaAviso(t *testing.T) {
	for _, p := range []string{
		"quanto custa um saco de cimento?",
		"quantos blocos preciso pra uma parede de 10 m²?",
		"qual a melhor furadeira que vocês têm?",
		"vocês vendem tinta branca?",
		"quanto de argamassa colante pra 20 m² de piso?",
		"qual a altura do rodapé padrão?",
	} {
		if achados := safety.Analisar(p); len(achados) > 0 {
			t.Errorf("falso positivo em pergunta comum %q → %v", p, achados[0].Categoria)
		}
	}
}

func TestAnalisar_EntradaVazia(t *testing.T) {
	if len(safety.Analisar("")) != 0 {
		t.Error("texto vazio não deveria gerar achado")
	}
	if len(safety.Analisar("   ")) != 0 {
		t.Error("texto em branco não deveria gerar achado")
	}
}

// Acento e caixa não podem escapar do detector.
func TestAnalisar_NormalizaAcentoECaixa(t *testing.T) {
	for _, p := range []string{
		"QUAL A BITOLA DE FERRO DA VIGA?",
		"Instalação Elétrica",
		"instalacao eletrica",
		"FUNDAÇÃO",
	} {
		if len(safety.Analisar(p)) == 0 {
			t.Errorf("detector escapou por acento/caixa: %q", p)
		}
	}
}

// Riscos declarados na base de conhecimento também disparam o aviso, mesmo que
// a pergunta não use nenhuma palavra de risco. Pedir "lista de material do
// telhado" traz o aviso de altura sem o cliente ter falado em altura.
func TestAnalisarRiscos_APartirDaBase(t *testing.T) {
	achados := safety.AnalisarRiscos([]string{"altura", "estrutural"})
	if !temCategoria(achados, safety.Altura) || !temCategoria(achados, safety.Estrutural) {
		t.Fatal("riscos declarados deveriam virar achados")
	}
	if !safety.Bloqueia(achados) {
		t.Error("risco estrutural declarado deveria bloquear dimensionamento")
	}
	if len(safety.AnalisarRiscos([]string{"inexistente"})) != 0 {
		t.Error("risco desconhecido não deveria gerar achado")
	}
}

// A ordem importa na apresentação: o mais grave primeiro.
func TestAnalisar_OrdenaPorGravidade(t *testing.T) {
	achados := safety.Analisar("quero instalar o gás e também subir no telhado pra ver a viga")
	if len(achados) < 2 {
		t.Fatalf("esperava múltiplos achados, veio %d", len(achados))
	}
	if achados[0].Categoria != safety.Gas {
		t.Errorf("gás deveria vir primeiro, veio %q", achados[0].Categoria)
	}
}

// O texto do aviso é fixo e vem do servidor. Nem o modelo nem uma injeção de
// prompt podem reescrevê-lo ou suprimi-lo.
func TestTextoAvisos_Determinista(t *testing.T) {
	a := safety.TextoAvisos(safety.Analisar("bitola de ferro da viga"))
	b := safety.TextoAvisos(safety.Analisar("bitola de ferro da viga"))
	if a != b {
		t.Error("o texto do aviso tem que ser determinístico")
	}
	if a == "" {
		t.Fatal("aviso não pode ser vazio")
	}
	if safety.TextoAvisos(nil) != "" {
		t.Error("sem achados, sem aviso")
	}
}
