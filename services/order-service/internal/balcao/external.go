package balcao

import (
	"errors"
	"fmt"
	"strings"
)

// ============================================================================
// Liquidação externa — venda de balcão paga na MAQUININHA DA LOJA
// ----------------------------------------------------------------------------
// A maquininha é de um adquirente próprio, FORA da Appmax. O dinheiro entra por
// fora do nosso PSP: não existe cobrança, não existe webhook, não existe nada
// que prove de fora que a venda foi paga. A única prova é o comprovante de
// papel na mão do operador, e o que amarra esse papel ao nosso pedido é o NSU.
//
// PORQUÊ isto é o ponto mais perigoso do PDV: é literalmente um endpoint que
// diz "pagou" sem dinheiro nenhum ter entrado no nosso sistema. Não existe
// verificação criptográfica possível — a única defesa é QUEM pode chamar e o
// rastro que fica. Por isso as regras estão aqui, como funções puras com testes
// que rodam sempre, e não como `if`s dentro do handler.
// ============================================================================

var (
	// ErrNotSettler — papel sem poder de liquidar por fora. Vale sobretudo para
	// `customer`: um cliente na loja online marcando o próprio pedido como pago
	// é o pior caso desta feature, e é o caso que ela precisa tornar impossível.
	ErrNotSettler = errors.New("balcao: role cannot settle payments externally")
	// ErrNotBalcaoOrder — pedido do site. Venda web se paga pelo PSP; liquidar
	// por fora um pedido web seria entregar mercadoria com base em nada.
	ErrNotBalcaoOrder = errors.New("balcao: only balcao orders can be settled externally")
	// ErrApprovalPending — desconto ainda na fila do gerente. Liquidar antes
	// esvaziaria a fila de aprovação na prática: bastaria dar 40% e cobrar.
	ErrApprovalPending = errors.New("balcao: discount is pending approval")
	// ErrApprovalRejected — desconto recusado. O pedido precisa ser refeito com
	// o valor certo, não liquidado pelo valor que o gerente vetou.
	ErrApprovalRejected = errors.New("balcao: discount was rejected")
	// ErrInvalidNSU — comprovante sem NSU utilizável.
	ErrInvalidNSU = errors.New("balcao: NSU inválido")
	// ErrNSUMismatch — pedido já liquidado com OUTRO NSU. Não é retry: são dois
	// comprovantes diferentes para a mesma venda, e alguém precisa olhar.
	ErrNSUMismatch = errors.New("balcao: order already settled with a different NSU")
)

// CanSettleExternal decide se `a` pode marcar `o` como pago na maquininha.
//
// A ordem das checagens é deliberada: o canal vem ANTES do papel. Um pedido do
// site nunca é liquidável por este caminho, nem por admin — assim, mesmo que
// um dia alguém afrouxe a regra de papel, um pedido web continua exigindo
// dinheiro de verdade passando pelo PSP.
func CanSettleExternal(a Actor, o OrderRef) error {
	if o.Channel != ChannelBalcao {
		return ErrNotBalcaoOrder
	}

	switch a.Role {
	case RoleAdmin:
		// Admin passa em qualquer loja — mas passa AUDITADO, como todo mundo.
		// A trilha é a mesma; não existe liquidação sem rastro até a pessoa.

	case RoleStoreOperator:
		if a.StoreID == "" {
			// Vínculo revogado ou token sem loja. Fail-closed: sem loja, sem
			// liquidação. Ver actorFromContext — o vínculo do auth-service é a
			// verdade, o token é só hint.
			return ErrNoStoreBinding
		}
		if o.StoreID != a.StoreID {
			return fmt.Errorf("%w: operator store %s, order store %s", ErrForeignStore, a.StoreID, o.StoreID)
		}

	default:
		// customer, seller, service e qualquer papel futuro. O default é
		// RECUSAR: um papel novo que ninguém pensou não deve nascer podendo
		// declarar vendas como pagas.
		return ErrNotSettler
	}

	// Desconto acima do teto NÃO bloqueia a venda (ver ResolveDiscount), mas
	// bloqueia a LIQUIDAÇÃO. É a diferença entre "registrar o que aconteceu" e
	// "dar baixa no dinheiro": o primeiro é sempre melhor que empurrar o
	// vendedor para o desconto por fora; o segundo é o gerente quem homologa.
	switch o.ApprovalStatus {
	case ApprovalPending:
		return ErrApprovalPending
	case ApprovalRejected:
		return ErrApprovalRejected
	}

	return nil
}

// maxNSULen — os adquirentes brasileiros usam NSU de 6 a 12 caracteres; 32 é
// folga para qualquer formato e trava payload absurdo.
const maxNSULen = 32
const minNSULen = 4

// NormalizeNSU valida e normaliza o NSU do comprovante.
//
// O NSU é o ÚNICO campo que amarra nossa venda à linha do extrato do
// adquirente. Sem ele, a liquidação externa vira "o operador disse que pagou" —
// não conciliável, não auditável, não contestável.
//
// Normaliza para maiúsculas e sem separadores porque o operador digita do jeito
// que está no papel (com espaço, com hífen, minúsculo) e o financeiro busca por
// igualdade exata contra o arquivo do adquirente. Divergência de formatação
// aqui é divergência de conciliação depois.
func NormalizeNSU(raw string) (string, error) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '.':
			// separador do comprovante: descartado
		default:
			return "", fmt.Errorf("%w: caractere inesperado %q", ErrInvalidNSU, r)
		}
	}
	nsu := b.String()
	if len(nsu) < minNSULen {
		return "", fmt.Errorf("%w: precisa de ao menos %d caracteres, veio %q", ErrInvalidNSU, minNSULen, nsu)
	}
	if len(nsu) > maxNSULen {
		return "", fmt.Errorf("%w: máximo de %d caracteres", ErrInvalidNSU, maxNSULen)
	}
	return nsu, nil
}

// CheckSettlementIdempotency compara o NSU recebido com o já gravado no pedido.
//
// Três resultados, e a distinção entre eles é o que impede tanto o lançamento
// duplicado quanto o encobrimento de um erro:
//
//	settled=false          — pedido ainda não liquidado; siga.
//	settled=true, err=nil  — MESMO NSU: é retry (rede caiu, operador clicou
//	                         duas vezes). No-op idempotente, devolve 200.
//	err=ErrNSUMismatch     — OUTRO NSU no mesmo pedido: são dois comprovantes
//	                         para uma venda só. Pode ser cobrança em
//	                         duplicidade no cartão do cliente. Recusa e alguém
//	                         olha — jamais sobrescreve o NSU anterior, que é a
//	                         prova do primeiro comprovante.
func CheckSettlementIdempotency(existingNSU, incomingNSU string) (settled bool, err error) {
	if existingNSU == "" {
		return false, nil
	}
	if existingNSU == incomingNSU {
		return true, nil
	}
	return true, fmt.Errorf("%w: gravado %s, recebido %s", ErrNSUMismatch, existingNSU, incomingNSU)
}

// Bandeiras aceitas. Lista fechada porque este campo vai para relatório
// financeiro: texto livre viraria "Visa", "visa", "VISA " e "Vsia" na mesma
// coluna, e o agrupamento por bandeira deixaria de existir.
var knownBrands = map[string]string{
	"visa": "visa", "mastercard": "mastercard", "elo": "elo",
	"amex": "amex", "hipercard": "hipercard", "cabal": "cabal",
	"diners": "diners", "outros": "outros",
}

// NormalizeBrand valida a bandeira. Vazio é aceito (nem todo comprovante traz,
// e o NSU é o que importa de verdade) — desconhecido vira erro para que a
// digitação errada apareça na hora, e não no fechamento do mês.
func NormalizeBrand(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return "", nil
	}
	if b, ok := knownBrands[v]; ok {
		return b, nil
	}
	return "", fmt.Errorf("bandeira desconhecida %q", raw)
}
