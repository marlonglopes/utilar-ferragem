package returns

import "fmt"

// Status é o estado do pedido de devolução (espelha o enum return_status).
type Status string

const (
	StatusRequested Status = "requested"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusInTransit Status = "in_transit"
	StatusReceived  Status = "received"
	StatusRefunded  Status = "refunded"
	StatusCancelled Status = "cancelled"
)

// ErrInvalidTransition — mudança de estado que a máquina não permite.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("returns: transição inválida %s → %s", e.From, e.To)
}

// allowed é a máquina de estados da devolução.
//
// ⚠️ As DUAS invariantes que esta tabela protege, e que valem dinheiro:
//
//  1. ESTOQUE só volta em `received` — quando a mercadoria foi CONFERIDA na
//     loja. Devolver estoque em `requested` ou em `approved` coloca à venda um
//     produto que ainda está na casa do cliente (ou que nunca vai ser postado).
//     O sistema venderia o que não tem, e a segunda venda é que descobre.
//
//  2. DINHEIRO só sai em `refunded`, e só a partir de `received`. Não existe
//     aresta approved → refunded: estornar antes de a mercadoria chegar é
//     entregar produto e dinheiro para a mesma pessoa. O caminho é
//     approved → in_transit → received → refunded.
//
// `rejected` só é alcançável a partir de `requested`, e — pelo CHECK
// `returns_regret_cannot_be_rejected` no banco e por CanReject aqui — nunca num
// arrependimento.
var allowed = map[Status][]Status{
	StatusRequested: {StatusApproved, StatusRejected, StatusCancelled},
	StatusApproved:  {StatusInTransit, StatusReceived, StatusCancelled},
	// in_transit → received é o único caminho adiante: a mercadoria postada
	// precisa ser conferida.
	StatusInTransit: {StatusReceived},
	StatusReceived:  {StatusRefunded},
	// Terminais.
	StatusRefunded:  {},
	StatusRejected:  {},
	StatusCancelled: {},
}

// CanTransition valida a mudança de estado.
func CanTransition(from, to Status) error {
	for _, s := range allowed[from] {
		if s == to {
			return nil
		}
	}
	return ErrInvalidTransition{From: from, To: to}
}

// StockReturnsAt é o estado em que o estoque volta. Constante nomeada para que
// a regra apareça no código que a usa, e não como string solta.
const StockReturnsAt = StatusReceived

// RefundHappensAt é o estado em que o dinheiro sai.
const RefundHappensAt = StatusRefunded

// CanReject valida uma recusa.
//
// ⚠️ Arrependimento (art. 49) é direito INCONDICIONAL: dentro do prazo não se
// pede motivo e não se avalia. Recusar é ilegal, e é a razão desta função
// existir separada de CanTransition — a máquina de estados sozinha permitiria
// a aresta requested → rejected, que é legítima para vício.
func CanReject(kind Kind, note string) error {
	if kind == KindRegret {
		return ErrRegretCannotBeRejected
	}
	if !hasText(note) {
		return ErrDecisionNoteRequired
	}
	return nil
}
