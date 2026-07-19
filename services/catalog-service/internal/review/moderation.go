package review

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ============================================================================
// MODERAÇÃO — a decisão e o porquê
// ============================================================================
//
// A PERGUNTA: avaliação entra publicada ou pendente?
//
// As duas respostas puras são ruins, e cada uma falha de um jeito:
//
//   • TUDO PENDENTE (pré-moderação). Correto no papel e inviável aqui: a Utilar
//     não tem equipe de moderação. O que acontece de verdade numa loja pequena
//     que exige aprovação manual é a fila crescer, ninguém revisar, e o produto
//     ficar semanas com "seja o primeiro a avaliar" enquanto há dez avaliações
//     paradas. Pior: o cliente que se deu ao trabalho de escrever não vê o
//     texto no ar e conclui que a loja apagou. Moderação que não é executada é
//     igual a censura arbitrária, com o custo de ter sido implementada.
//
//   • TUDO PUBLICADO. Texto livre público sem barreira é alvo de spam de SEO,
//     de contato ("chama no zap 9xxxx, faço mais barato") e de ataque a
//     concorrente.
//
// A DECISÃO: publicação imediata, com DUAS barreiras antes dela.
//
//   1. COMPRA VERIFICADA (grant.go). É a barreira que faz o trabalho pesado, e
//      é qualitativamente diferente de um captcha: para postar spam é preciso
//      comprar o produto de verdade, com dinheiro, e cada compra rende UMA
//      avaliação (índice único por pessoa/produto). O spam em massa deixa de
//      fechar a conta. Nenhuma loja com moderação manual e review aberta a
//      qualquer visitante tem uma barreira tão forte quanto essa.
//
//   2. TRIAGEM AUTOMÁTICA (este arquivo). O que sobra depois da barreira 1 é o
//      comprador real que abusa do espaço — e isso tem forma reconhecível:
//      link, telefone, e-mail, grito em caixa alta, texto repetitivo. Só ESSE
//      subconjunto vai para 'pending'. É uma fila que cabe no tempo que o dono
//      da loja tem, porque é pequena por construção.
//
// CONSEQUÊNCIA ACEITA: uma ofensa escrita em português normal, sem link nem
// telefone, entra publicada e só sai quando alguém denunciar. É o preço de não
// ter equipe de moderação, e é preferível ao modo de falha da pré-moderação
// (nada no ar, nunca). O caminho de denúncia é o que ficou de fora deste corte
// — está anotado em docs/reviews-e-recomendacao.md.
//
// NOTA/ESTRELA SEM TEXTO nunca vai para a fila: não há o que moderar num
// número, e ele já está limitado a um por pessoa por produto.
// ============================================================================

// Status possíveis de uma avaliação. Espelham o CHECK da migration 015.
const (
	StatusPublished = "published"
	StatusPending   = "pending"
	StatusRejected  = "rejected"
)

// MaxBodyLen / MaxTitleLen espelham os CHECK do banco. Validados também aqui
// para o cliente receber 400 com mensagem útil em vez de 500 de constraint.
const (
	MaxTitleLen = 120
	MaxBodyLen  = 2000
)

var (
	// URL/domínio. Pega "http://x", "www.x.com" e "loja.com.br" sem protocolo —
	// o formato mais comum de spam de SEO em avaliação.
	reURL = regexp.MustCompile(`(?i)(https?://|www\.|\b[a-z0-9-]+\.(com|net|org|br|shop|store|io|co)\b)`)

	// E-mail.
	reEmail = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

	// Telefone brasileiro em qualquer formatação usual: (11) 91234-5678,
	// 11912345678, +55 11 91234 5678. Exige 10+ dígitos para não pegar
	// "furadeira 750W 220V" nem medida ("2,5mm 750V").
	rePhone = regexp.MustCompile(`(?:\+?55[\s.\-]?)?(?:\(?\d{2}\)?[\s.\-]?)?9?\d{4}[\s.\-]?\d{4}`)

	// Convite explícito a contato fora da plataforma. Cobre o spam que escreve
	// o telefone por extenso justamente para escapar do regex de número.
	reContato = regexp.MustCompile(`(?i)\b(whats?app|whatzap|zap|telegram|chama\s+no|me\s+chama|fora\s+do\s+site|direto\s+comigo)\b`)

)

// maxRunLength — a partir de quantas repetições do MESMO caractere o texto é
// considerado ruído. 6 deixa passar "boaaaa" e "!!!" (ênfase normal) e pega
// "boooooooom" e "!!!!!!!!!!".
const maxRunLength = 6

// temRepeticao detecta sequência longa do mesmo caractere.
//
// PORQUÊ um laço e não regex: a expressão natural (`(.)\1{5,}`) usa
// BACKREFERENCE, e o motor de regex de Go é RE2, que não tem backreference por
// desenho (é o que lhe garante tempo linear). `regexp.MustCompile` com ela
// entra em PÂNICO no init do pacote — ou seja, derruba o serviço no boot, não
// na primeira avaliação. Pego por teste.
func temRepeticao(s string) bool {
	var anterior rune
	run := 0
	for _, r := range s {
		if r == anterior {
			run++
			if run >= maxRunLength {
				return true
			}
			continue
		}
		anterior, run = r, 1
	}
	return false
}

// Verdict é o resultado da triagem.
type Verdict struct {
	Status string
	// Note é a razão, gravada em product_reviews.moderation_note e exibida ao
	// admin na fila. É o que permite decidir sem reler o texto inteiro
	// procurando o que o robô viu.
	Note string
}

// Classify decide o status inicial de uma avaliação a partir do texto.
//
// É deliberadamente CONSERVADORA no sentido de mandar pouco para a fila: cada
// falso positivo é um cliente honesto que não vê a própria avaliação no ar, e
// numa loja onde a fila é revisada devagar isso custa mais caro que deixar
// passar um texto ruim.
func Classify(title, body string) Verdict {
	texto := strings.TrimSpace(title + " " + body)
	if texto == "" {
		// Avaliação só com estrela. Nada a moderar.
		return Verdict{Status: StatusPublished}
	}

	switch {
	case reURL.MatchString(texto):
		return Verdict{StatusPending, "contém link ou domínio"}
	case reEmail.MatchString(texto):
		return Verdict{StatusPending, "contém e-mail"}
	case reContato.MatchString(texto):
		return Verdict{StatusPending, "convite a contato fora da plataforma"}
	case temTelefone(texto):
		return Verdict{StatusPending, "contém número de telefone"}
	case gritando(texto):
		return Verdict{StatusPending, "texto majoritariamente em caixa alta"}
	case temRepeticao(texto):
		return Verdict{StatusPending, "repetição excessiva de caracteres"}
	}

	return Verdict{Status: StatusPublished}
}

// temTelefone confere o regex de telefone e depois CONTA OS DÍGITOS do trecho
// casado.
//
// PORQUÊ a contagem: o regex sozinho casa com códigos e medidas que aparecem o
// tempo todo em avaliação de ferragem ("cabo 2,5mm² 750V", "GSB 13 RE 650W").
// Um falso positivo aqui manda para a fila justamente a avaliação técnica, que
// é a mais útil da loja. Telefone brasileiro tem 10 ou 11 dígitos; abaixo
// disso é medida, não contato.
func temTelefone(s string) bool {
	for _, m := range rePhone.FindAllString(s, -1) {
		n := 0
		for _, r := range m {
			if unicode.IsDigit(r) {
				n++
			}
		}
		if n >= 10 {
			return true
		}
	}
	return false
}

// gritando: mais de 70% das letras em caixa alta, em texto com pelo menos 20
// letras.
//
// O piso de 20 letras existe porque "PÉSSIMO" e "TOP" são reação legítima e
// curta, não spam. O que a regra mira é o parágrafo inteiro em maiúscula.
func gritando(s string) bool {
	var letras, maiusculas int
	for _, r := range s {
		if unicode.IsLetter(r) {
			letras++
			if unicode.IsUpper(r) {
				maiusculas++
			}
		}
	}
	if letras < 20 {
		return false
	}
	return float64(maiusculas)/float64(letras) > 0.7
}

// ValidateText aplica os limites de tamanho antes do banco.
// Devolve mensagem pronta para o 400, ou "" se está tudo certo.
func ValidateText(title, body string) string {
	if utf8.RuneCountInString(title) > MaxTitleLen {
		return "título excede 120 caracteres"
	}
	if utf8.RuneCountInString(body) > MaxBodyLen {
		return "texto excede 2000 caracteres"
	}
	return ""
}

// DisplayName reduz o nome do cliente ao que a vitrine exibe: primeiro nome +
// inicial do sobrenome ("Marlon Gomes Lopes" → "Marlon G.").
//
// ⚠️ É MINIMIZAÇÃO DE DADO PESSOAL, não formatação. O nome completo de quem
// comprou fica no auth-service; publicar "Marlon Gomes Lopes" junto de um
// produto comprado é expor consumo associado a pessoa identificável, numa
// página aberta e indexável. O que a avaliação precisa é de um humano
// reconhecível, não de identificação.
func DisplayName(nome string) string {
	partes := strings.Fields(strings.TrimSpace(nome))
	switch len(partes) {
	case 0:
		return "Cliente"
	case 1:
		return partes[0]
	}
	ultimo := []rune(partes[len(partes)-1])
	return partes[0] + " " + strings.ToUpper(string(ultimo[0])) + "."
}
