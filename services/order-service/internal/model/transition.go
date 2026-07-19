package model

import "fmt"

// ============================================================================
// Máquina de estados do pedido
// ----------------------------------------------------------------------------
// PORQUÊ isto existe como função pura, separada dos handlers:
//
// Antes, cada caminho que mexia em status carregava sua própria regra ad-hoc —
// o Cancel tinha uma lista de status proibidos escrita à mão, e não havia
// caminho nenhum que escrevesse 'paid'. Toda vez que um estado novo entrasse,
// alguém teria que lembrar de atualizar N condicionais espalhadas.
//
// Com a transição centralizada aqui, o handler e o consumer Kafka fazem a
// mesma pergunta ("posso ir de X pra Y?") e recebem a mesma resposta, e o teste
// da máquina roda sem banco, sem HTTP e sem Kafka.
//
//	pending_payment ──> paid ──> picking ──> shipped ──> delivered
//	       │              │         │
//	       └──────────────┴─────────┴────────> cancelled
//
// Regra de negócio: uma vez despachado (shipped), o pedido não pode mais ser
// cancelado pelo fluxo normal — a mercadoria está com a transportadora. A
// reversão vira caso de devolução, que é outro processo (fora de escopo aqui).
// ============================================================================

// allowedTransitions mapeia estado atual → estados alcançáveis.
var allowedTransitions = map[OrderStatus][]OrderStatus{
	StatusPendingPayment: {StatusPaid, StatusCancelled},
	StatusPaid:           {StatusPicking, StatusCancelled},
	StatusPicking:        {StatusShipped, StatusCancelled},
	StatusShipped:        {StatusDelivered},
	StatusDelivered:      {}, // terminal
	StatusCancelled:      {}, // terminal
}

// ErrInvalidTransition descreve uma transição rejeitada. Carrega os dois
// estados pra mensagem de erro ser acionável ("de X pra Y não pode") em vez de
// um "conflict" genérico que obriga o operador a adivinhar.
type ErrInvalidTransition struct {
	From OrderStatus
	To   OrderStatus
}

func (e ErrInvalidTransition) Error() string {
	if _, known := allowedTransitions[e.From]; !known {
		return fmt.Sprintf("unknown order status %q", e.From)
	}
	if _, known := allowedTransitions[e.To]; !known {
		return fmt.Sprintf("unknown target status %q", e.To)
	}
	if len(allowedTransitions[e.From]) == 0 {
		return fmt.Sprintf("order is in terminal status %q and cannot transition to %q", e.From, e.To)
	}
	return fmt.Sprintf("invalid transition from %q to %q", e.From, e.To)
}

// CanTransition valida uma mudança de estado. Retorna nil se permitida,
// ErrInvalidTransition caso contrário.
//
// Transição para o MESMO estado é rejeitada de propósito: quem precisa de
// tolerância a reentrega (o consumer Kafka) já detecta o replay pela tabela de
// eventos processados; deixar passar aqui esconderia bugs reais de fluxo.
func CanTransition(from, to OrderStatus) error {
	targets, known := allowedTransitions[from]
	if !known {
		return ErrInvalidTransition{From: from, To: to}
	}
	if _, knownTo := allowedTransitions[to]; !knownTo {
		return ErrInvalidTransition{From: from, To: to}
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return ErrInvalidTransition{From: from, To: to}
}

// IsTerminal indica se o pedido não pode mais mudar de estado.
func IsTerminal(s OrderStatus) bool {
	targets, known := allowedTransitions[s]
	return known && len(targets) == 0
}

// TimestampColumn devolve a coluna de timestamp que deve ser preenchida ao
// entrar em `s`. Segunda saída é false para estados sem coluna dedicada.
//
// Mantido junto da tabela de transições de propósito: quando alguém adicionar
// um estado novo, as duas coisas que precisam mudar estão na mesma tela.
func TimestampColumn(s OrderStatus) (string, bool) {
	switch s {
	case StatusPaid:
		return "paid_at", true
	case StatusPicking:
		return "picked_at", true
	case StatusShipped:
		return "shipped_at", true
	case StatusDelivered:
		return "delivered_at", true
	case StatusCancelled:
		return "cancelled_at", true
	default:
		return "", false
	}
}

// TrackingDescription é a frase em pt-BR mostrada ao cliente na timeline do
// pedido ao entrar em cada estado.
func TrackingDescription(s OrderStatus) string {
	switch s {
	case StatusPendingPayment:
		return "Pedido criado. Aguardando pagamento."
	case StatusPaid:
		return "Pagamento confirmado. Pedido em preparação."
	case StatusPicking:
		return "Pedido separado no estoque."
	case StatusShipped:
		return "Pedido despachado para entrega."
	case StatusDelivered:
		return "Pedido entregue."
	case StatusCancelled:
		return "Pedido cancelado."
	default:
		return string(s)
	}
}
