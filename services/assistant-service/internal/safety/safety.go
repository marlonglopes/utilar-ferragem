// Package safety é a barreira de segurança de CONTEÚDO da Alice.
//
// Conselho de construção tem consequência física. A distinção que este package
// implementa é a que separa uma balconista competente de um risco:
//
//	A Alice CALCULA QUANTIDADE DE MATERIAL. Ela NÃO DIMENSIONA ESTRUTURA.
//
// Quantos blocos cabem em 10 m² é aritmética e ela responde. Que bitola de ferro
// vai numa viga de 4 m, se aquela parede pode cair, que espessura tem a laje —
// isso depende de carga, vão, solo e uso, e é atribuição legal de engenheiro ou
// arquiteto. Errar ali não gera compra errada: gera desabamento.
//
// Isto é implementado como REGRA DE SISTEMA e VERIFICAÇÃO, não como pedido
// gentil no prompt. Um prompt pode ser contornado por uma pergunta bem torta ou
// por injeção; um gate em Go, não. O detector roda sobre a pergunta do usuário
// ANTES do modelo e sobre o resultado das ferramentas, e o aviso é anexado de
// forma determinística, fora do controle do modelo.
package safety

import (
	"sort"
	"strings"
)

// Categoria de risco detectado.
type Categoria string

const (
	// Estrutural — dimensionamento de elemento que sustenta carga.
	// A Alice explica e lista material, mas NUNCA dá dimensão, bitola ou o
	// veredito de "pode derrubar".
	Estrutural Categoria = "estrutural"
	// Eletrico — execução exige profissional habilitado (NR-10).
	Eletrico Categoria = "eletrico"
	// Gas — a mais restritiva: nunca instruir instalação.
	Gas Categoria = "gas"
	// Altura — trabalho em altura, EPI e NR-35.
	Altura Categoria = "altura"
	// Demolicao — remover parede/abrir vão: pode ser estrutural sem parecer.
	Demolicao Categoria = "demolicao"
)

// Achado é um risco detectado, com o encaminhamento correspondente.
type Achado struct {
	Categoria Categoria `json:"categoria"`
	// Aviso é o texto que vai para o cliente, textual e não negociável.
	Aviso string `json:"aviso"`
	// BloqueiaDimensionamento marca os casos em que a Alice não pode dar
	// número de dimensionamento de jeito nenhum.
	BloqueiaDimensionamento bool `json:"bloqueia_dimensionamento"`
	// Termo que disparou — para depuração e para os testes serem legíveis.
	Termo string `json:"termo,omitempty"`
}

// Avisos por categoria. Texto fixo: é obrigação legal e de segurança, não
// varia com o humor do modelo nem com o quanto o cliente insiste.
var avisos = map[Categoria]string{
	Estrutural: "⚠️ Atenção: isso envolve elemento ESTRUTURAL (o que sustenta a construção). " +
		"Eu consigo explicar como funciona e listar o material, mas NÃO posso dimensionar: " +
		"bitola de ferro, seção de viga ou pilar, espessura de laje e profundidade de fundação " +
		"dependem de carga, vão, tipo de solo e uso da edificação. Esse cálculo é atribuição legal " +
		"de um engenheiro civil ou arquiteto, que assume a responsabilidade técnica pelo projeto. " +
		"Errar aqui não custa dinheiro, custa vidas — procure um profissional antes de executar.",

	Demolicao: "⚠️ Atenção: antes de remover qualquer parede ou abrir um vão, é preciso um " +
		"profissional verificar NO LOCAL se ela é estrutural. Eu não tenho como saber isso à distância, " +
		"e parede que parece de vedação às vezes está sustentando carga. " +
		"Derrubar a parede errada pode comprometer a construção inteira. " +
		"Consulte um engenheiro civil ou arquiteto antes.",

	Eletrico: "⚠️ Atenção: instalação elétrica tem risco de choque e de incêndio. " +
		"Eu explico como funciona e monto a lista de material, mas a EXECUÇÃO e a ligação no quadro " +
		"exigem profissional habilitado, seguindo a ABNT NBR 5410 e as medidas de segurança da NR-10. " +
		"O dimensionamento do circuito (bitola do cabo, disjuntor, DR) depende da carga e do percurso — " +
		"não é coisa de tabela genérica.",

	Gas: "🚫 Instalação de gás eu NÃO oriento, em nenhuma hipótese. " +
		"Vazamento de gás causa explosão e intoxicação, e a execução é privativa de profissional " +
		"qualificado e credenciado, seguindo a ABNT NBR 15526. " +
		"Procure a distribuidora de gás da sua região ou um instalador credenciado. " +
		"Se você suspeita de vazamento agora: não acione interruptor nem chama, abra as janelas, " +
		"feche o registro e saia do local antes de ligar para a emergência.",

	Altura: "⚠️ Atenção: trabalho em altura (telhado, andaime, escada acima de 2 m) é uma das " +
		"maiores causas de acidente grave na construção. Exige cinturão tipo paraquedista ancorado " +
		"em ponto firme, calçado antiderrapante, área isolada embaixo e nunca trabalhar sozinho — " +
		"são as exigências da NR-35. Em telhado, nunca pise direto na telha: use tábua apoiada sobre as terças.",
}

// Termos que disparam cada categoria. Casados por substring sobre texto
// normalizado (minúsculo, sem acento), o que cobre plural e conjugação.
//
// Deliberadamente ABRANGENTE: um falso positivo custa um aviso a mais numa
// resposta; um falso negativo custa a integridade de uma construção. A
// assimetria é óbvia, e a lista reflete isso.
var termos = map[Categoria][]string{
	Estrutural: {
		"viga", "pilar", "laje", "fundacao", "sapata", "baldrame", "bloco de fundacao",
		"arrimo", "muro de arrimo", "estrutural", "estrutura", "armadura", "ferragem estrutural",
		"bitola do ferro", "bitola de ferro", "ferro da viga", "vergalhao", "aco ca50", "ca-50", "ca 50",
		"treliça", "trelica", "escada de concreto", "balanco", "marquise", "mezanino",
		"radier", "estaca", "tubulao", "cinta de amarracao", "verga", "contraverga",
		"dimensionar", "dimensionamento", "calculo estrutural", "carga da laje",
		"quanto de ferro", "quantos ferros", "espessura da laje", "vao livre", "vencer o vao",
		"telhado suporta", "aguenta o peso", "suporta o peso",
	},
	Demolicao: {
		"derrubar parede", "derrubar a parede", "remover parede", "remover a parede",
		"tirar parede", "tirar a parede", "quebrar parede", "quebrar a parede",
		"abrir vao", "abrir um vao", "juntar os comodos", "unir os comodos",
		"parede e estrutural", "posso derrubar", "pode derrubar", "demolir",
	},
	Eletrico: {
		"eletrica", "eletrico", "fiacao", "disjuntor", "quadro de distribuicao", "quadro de luz",
		"dr ", "idr", "aterramento", "curto circuito", "choque", "tomada", "interruptor",
		"chuveiro eletrico", "bitola do cabo", "bitola do fio", "circuito", "voltagem",
		"110v", "220v", "trifasico", "monofasico", "padrao de entrada", "medidor de energia",
	},
	Gas: {
		"gas", "glp", "botijao", "botijão", "gas encanado", "gas natural", "mangueira de gas",
		"registro de gas", "aquecedor a gas", "fogao a gas", "cilindro de gas", "central de gas",
	},
	Altura: {
		"telhado", "andaime", "escada", "altura", "cobertura", "laje superior",
		"fachada", "beiral", "calha", "platibanda", "caixa d'agua elevada", "subir no telhado",
	},
}

// Termos que NEGAM uma detecção. Existem porque "tomada" e "escada" são
// palavras comuns demais: sem isto, "quantas tomadas tem no meu pedido" e
// "escada de pintor" disparariam avisos irrelevantes e a Alice viraria um
// disclaimer ambulante — o que treina o cliente a ignorar todos os avisos,
// inclusive os que importam.
var excecoes = map[Categoria][]string{
	Altura: {"escada de pintor", "escada de mao para pintura", "altura do rodape", "altura da bancada", "altura do azulejo"},
}

// Analisar roda o detector sobre um texto (pergunta do cliente ou resultado de
// ferramenta) e devolve os achados, ordenados por gravidade.
func Analisar(texto string) []Achado {
	n := normalizar(texto)
	if n == "" {
		return nil
	}

	vistos := map[Categoria]Achado{}
	for cat, lista := range termos {
		if negado(n, excecoes[cat]) {
			continue
		}
		for _, t := range lista {
			if !strings.Contains(n, t) {
				continue
			}
			// "gas" é curto e aparece dentro de outras palavras ("gasto",
			// "gasolina", "desgaste"). Exige limite de palavra.
			if t == "gas" && !palavraInteira(n, "gas") {
				continue
			}
			vistos[cat] = Achado{
				Categoria:               cat,
				Aviso:                   avisos[cat],
				BloqueiaDimensionamento: cat == Estrutural || cat == Demolicao || cat == Gas,
				Termo:                   t,
			}
			break
		}
	}

	out := make([]Achado, 0, len(vistos))
	for _, a := range vistos {
		out = append(out, a)
	}
	// Ordem de gravidade: gás primeiro (é o único que se recusa a instruir),
	// depois estrutural, demolição, elétrico e altura.
	rank := map[Categoria]int{Gas: 0, Estrutural: 1, Demolicao: 2, Eletrico: 3, Altura: 4}
	sort.Slice(out, func(i, j int) bool { return rank[out[i].Categoria] < rank[out[j].Categoria] })
	return out
}

// AnalisarRiscos converte os riscos declarados num serviço da base de
// conhecimento em achados. Assim o aviso não depende de o cliente ter usado a
// palavra certa: pedir "lista de material pro telhado" já traz o aviso de
// altura, mesmo sem a pergunta citar risco nenhum.
func AnalisarRiscos(riscos []string) []Achado {
	var out []Achado
	for _, r := range riscos {
		var cat Categoria
		switch r {
		case "estrutural":
			cat = Estrutural
		case "eletrico":
			cat = Eletrico
		case "gas":
			cat = Gas
		case "altura":
			cat = Altura
		default:
			continue
		}
		out = append(out, Achado{
			Categoria:               cat,
			Aviso:                   avisos[cat],
			BloqueiaDimensionamento: cat == Estrutural || cat == Gas,
			Termo:                   "risco declarado no serviço",
		})
	}
	return out
}

// RecusaGas informa se o texto pede instrução de instalação de gás — o único
// caso em que a Alice recusa orientar a execução, e não apenas avisa.
func RecusaGas(texto string) bool {
	for _, a := range Analisar(texto) {
		if a.Categoria == Gas {
			return true
		}
	}
	return false
}

// Bloqueia informa se algum achado impede dar número de dimensionamento.
func Bloqueia(achados []Achado) bool {
	for _, a := range achados {
		if a.BloqueiaDimensionamento {
			return true
		}
	}
	return false
}

// TextoAvisos junta os avisos para anexar à resposta. A Alice não escreve estes
// textos — eles são anexados pelo servidor, então nem o modelo nem uma injeção
// de prompt conseguem suprimi-los ou reescrevê-los.
func TextoAvisos(achados []Achado) string {
	if len(achados) == 0 {
		return ""
	}
	var b strings.Builder
	for i, a := range achados {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(a.Aviso)
	}
	return b.String()
}

// InstrucaoParaModelo devolve a orientação que acompanha o resultado da
// ferramenta, dizendo ao modelo o que ele NÃO pode fazer neste turno.
func InstrucaoParaModelo(achados []Achado) string {
	if len(achados) == 0 {
		return ""
	}
	var regras []string
	for _, a := range achados {
		switch a.Categoria {
		case Estrutural:
			regras = append(regras, "NÃO forneça dimensionamento estrutural (bitola de ferro, seção de viga ou pilar, "+
				"espessura de laje, profundidade de fundação). Explique o serviço e liste material, "+
				"e encaminhe o dimensionamento a engenheiro ou arquiteto.")
		case Demolicao:
			regras = append(regras, "NÃO afirme se uma parede pode ou não ser derrubada. "+
				"Encaminhe a verificação a um profissional no local.")
		case Eletrico:
			regras = append(regras, "Explique e liste material, mas deixe claro que a execução exige profissional habilitado. "+
				"NÃO dimensione circuito, cabo ou disjuntor para uma carga específica.")
		case Gas:
			regras = append(regras, "NÃO instrua instalação de gás em hipótese alguma. Encaminhe a instalador credenciado.")
		case Altura:
			regras = append(regras, "Mencione EPI e o risco de trabalho em altura (NR-35).")
		}
	}
	return "REGRAS DE SEGURANÇA OBRIGATÓRIAS PARA ESTA RESPOSTA:\n- " + strings.Join(regras, "\n- ")
}

// ---------------------------------------------------------------------------

func palavraInteira(texto, palavra string) bool {
	for i := 0; i+len(palavra) <= len(texto); i++ {
		if texto[i:i+len(palavra)] != palavra {
			continue
		}
		antesOK := i == 0 || !ehLetra(texto[i-1])
		j := i + len(palavra)
		depoisOK := j >= len(texto) || !ehLetra(texto[j])
		if antesOK && depoisOK {
			return true
		}
	}
	return false
}

func ehLetra(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func negado(texto string, excs []string) bool {
	for _, e := range excs {
		if strings.Contains(texto, e) {
			return true
		}
	}
	return false
}

// normalizar deixa o texto em minúsculas e sem acento, para o casamento por
// substring funcionar com "elétrica", "eletrica" e "ELÉTRICA".
func normalizar(s string) string {
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
