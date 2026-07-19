// Package returns concentra as REGRAS de devolução e troca como funções puras.
//
// PORQUÊ um pacote separado, e não `if`s dentro do handler: estas regras são
// obrigação legal (CDC), não política comercial. Regra enterrada em handler só
// é testável com banco de pé, e teste que precisa de banco é teste que alguém
// pula — e o que se estaria pulando aqui é a prova de que a loja cumpre a lei.
//
// # As duas bases legais, que NÃO são a mesma coisa
//
// CDC art. 49 — ARREPENDIMENTO (KindRegret)
//
//	"O consumidor pode desistir do contrato, no prazo de 7 dias a contar de sua
//	assinatura ou do ato de recebimento do produto ou serviço, sempre que a
//	contratação de fornecimento de produtos e serviços ocorrer fora do
//	estabelecimento comercial."
//
//	Toda venda online é "fora do estabelecimento". O direito é INCONDICIONAL:
//	não se pergunta motivo, não se avalia estado de uso, não se recusa. O
//	produto não precisa ter defeito nenhum. O frete de volta é da loja.
//	Prazo conta do RECEBIMENTO, não da compra.
//
// CDC art. 26 — VÍCIO DO PRODUTO (KindDefect)
//
//	30 dias (não durável) ou 90 dias (durável) a contar de quando o vício ficou
//	evidente. Aqui SIM existe análise: o produto precisa ter defeito, e a loja
//	tem 30 dias para saná-lo antes de dever troca ou devolução do valor.
//
// Tratar as duas como "solicitação com um motivo" faz o atendente avaliar um
// arrependimento — que não se avalia. É assim que se toma multa do Procon.
//
// ⚠️ AMBIGUIDADE EM ABERTO: este pacote implementa o caminho do LOJISTA (a
// Utilar vende o próprio produto e responde pela devolução). Se a Utilar for
// MARKETPLACE, quem responde muda. Ver docs/devolucao-e-troca.md.
package returns

import (
	"errors"
	"fmt"
	"time"
)

// Kind é a base legal do pedido de devolução.
type Kind string

const (
	// KindRegret — CDC art. 49. Arrependimento. Incondicional.
	KindRegret Kind = "regret"
	// KindDefect — CDC art. 26. Vício do produto. Com análise.
	KindDefect Kind = "defect"
)

// Prazos legais.
const (
	// RegretDays — 7 dias CORRIDOS (não úteis) do art. 49.
	RegretDays = 7
	// DefectDaysDurable — 90 dias do art. 26, II (bem durável). Ferramenta e
	// material de construção são duráveis; é o caso de praticamente todo o
	// catálogo da Utilar. Usamos o prazo MAIOR quando a durabilidade não está
	// classificada: errar a favor do consumidor é o lado certo para errar num
	// prazo do CDC.
	DefectDaysDurable = 90
)

// Papéis (espelham os do balcao/authz.go — mantidos aqui para o pacote ser
// independente e testável sozinho).
const (
	RoleCustomer      = "customer"
	RoleAdmin         = "admin"
	RoleOperator      = "operator"
	RoleStoreOperator = "store_operator"
	RoleService       = "service"
)

// Erros sentinela. O handler os traduz em HTTP sem reinterpretar mensagem.
var (
	// ErrNotOwner — o cliente tentou devolver pedido de OUTRA pessoa.
	// Vira 404 no handler, nunca 403: 403 confirma que o pedido existe e
	// transforma a rota em enumerador de pedidos alheios.
	ErrNotOwner = errors.New("returns: pedido não pertence a este usuário")

	// ErrDeadlineExpired — fora do prazo do art. 49. NÃO significa "não tem
	// direito a nada": significa que o caminho é o do vício (art. 26), com
	// análise. A mensagem para o cliente tem que dizer isso.
	ErrDeadlineExpired = errors.New("returns: prazo de arrependimento (7 dias) encerrado")

	// ErrDefectWindowExpired — fora também do prazo do art. 26.
	ErrDefectWindowExpired = errors.New("returns: prazo para reclamar vício do produto encerrado")

	// ErrOrderNotReturnable — o pedido está num estado que não comporta
	// devolução (não pago, já cancelado).
	ErrOrderNotReturnable = errors.New("returns: pedido não está em estado que permita devolução")

	// ErrNoItems — pedido de devolução sem item.
	ErrNoItems = errors.New("returns: informe ao menos um item a devolver")

	// ErrItemNotInOrder — item que não pertence ao pedido.
	ErrItemNotInOrder = errors.New("returns: item não pertence a este pedido")

	// ErrQuantityExceeded — devolver mais do que comprou (ou mais do que
	// sobrou, descontado o que já foi devolvido antes).
	ErrQuantityExceeded = errors.New("returns: quantidade devolvida excede a comprada")

	// ErrReasonRequired — vício sem motivo. Arrependimento NUNCA cai aqui.
	ErrReasonRequired = errors.New("returns: descreva o defeito do produto")

	// ⚠️ ErrSplitPartialRefund — pedido com Payment Split só aceita estorno
	// TOTAL na Appmax (docs/appmax-v1-appstore.md § 5). Recusado AQUI, na hora
	// do pedido de devolução, e não lá no PSP: falhar no PSP significaria o
	// produto já devolvido, o estoque já reposto e o dinheiro preso.
	ErrSplitPartialRefund = errors.New("returns: pedido com split de pagamento só aceita devolução total")

	// ErrRegretCannotBeRejected — tentativa de indeferir um arrependimento.
	// Direito incondicional não se recusa.
	ErrRegretCannotBeRejected = errors.New("returns: arrependimento é direito incondicional e não pode ser recusado")

	// ErrDecisionNoteRequired — recusa sem justificativa.
	ErrDecisionNoteRequired = errors.New("returns: recusa exige justificativa")

	// ErrNotReviewer — quem não pode decidir tentou decidir.
	ErrNotReviewer = errors.New("returns: papel não pode decidir sobre devoluções")
)

// OrderRef é o subconjunto do pedido que as regras precisam. Struct própria (e
// não model.Order) para o pacote não depender do schema e para os testes serem
// declarativos.
type OrderRef struct {
	ID      string
	UserID  string
	Status  string // pending_payment | paid | picking | shipped | delivered | cancelled
	Channel string // web | balcao

	// PaidAt / ShippedAt / DeliveredAt alimentam a contagem do prazo.
	// Ponteiros porque a ausência é justamente o caso interessante.
	PaidAt      *time.Time
	ShippedAt   *time.Time
	DeliveredAt *time.Time

	// PaymentSplit indica Payment Split na Appmax. Ver ErrSplitPartialRefund.
	PaymentSplit bool

	// ShippingCost entra no reembolso do arrependimento.
	ShippingCost float64
}

// OrderItemRef é um item do pedido, com o quanto dele JÁ foi devolvido em
// pedidos de devolução anteriores.
type OrderItemRef struct {
	ID        string
	ProductID string
	Quantity  int
	UnitPrice float64
	// AlreadyReturned é a soma das quantidades deste item em devoluções que não
	// foram recusadas nem canceladas. Sem isto, dez pedidos de devolução de 1
	// unidade cada devolveriam 10 de um item comprado 1 vez.
	AlreadyReturned int
}

// RequestedItem é o que o cliente pediu para devolver.
type RequestedItem struct {
	OrderItemID string
	Quantity    int
}

// Request é o pedido de devolução a avaliar.
type Request struct {
	Actor  Actor
	Order  OrderRef
	Items  map[string]OrderItemRef // chave: OrderItemID
	Wanted []RequestedItem
	Reason string
	Now    time.Time
}

// Actor é quem está pedindo/decidindo.
type Actor struct {
	UserID string
	Role   string
}

// ResolvedItem é um item já validado, com o valor calculado.
type ResolvedItem struct {
	OrderItemID string
	ProductID   string
	Quantity    int
	UnitPrice   float64
	LineAmount  float64
}

// Decision é o resultado da avaliação.
type Decision struct {
	// Kind é a base legal DERIVADA, não escolhida pelo cliente.
	//
	// PORQUÊ derivada: se o cliente escolhesse, todo mundo marcaria
	// "arrependimento" (que não tem análise) — inclusive fora dos 7 dias. E se
	// a loja escolhesse, todo arrependimento viraria "vício" para poder ser
	// analisado. A base legal é consequência da DATA, e a data é fato.
	Kind Kind

	// AutoApproved — o arrependimento é deferido na hora, sem análise humana.
	// É o que a lei manda: direito incondicional não passa por aprovação.
	AutoApproved bool

	Items []ResolvedItem

	// ItemsAmount é a soma das linhas devolvidas.
	ItemsAmount float64
	// ShippingRefund é o frete devolvido. Só no arrependimento, e só quando a
	// devolução é TOTAL: devolver o frete inteiro numa devolução parcial daria
	// à loja um prejuízo que a lei não impõe (a entrega dos demais itens de
	// fato aconteceu).
	ShippingRefund float64
	// TotalRefund = ItemsAmount + ShippingRefund.
	TotalRefund float64

	// IsFullReturn indica que todos os itens (e todas as quantidades) estão
	// sendo devolvidos.
	IsFullReturn bool

	// Deadline é o prazo aplicado e de onde ele saiu — congelado no registro.
	Deadline Deadline
}

// Evaluate é a porta única de entrada: valida autorização, prazo, itens e
// split, e devolve a decisão pronta para virar linha no banco.
//
// A ORDEM das verificações é deliberada:
//  1. AUTORIZAÇÃO primeiro, antes de revelar qualquer coisa sobre o pedido;
//  2. estado do pedido;
//  3. prazo (que decide a base legal);
//  4. itens e quantidades;
//  5. split (a trava do PSP), por último, porque depende de saber se a
//     devolução é total.
func Evaluate(req Request) (Decision, error) {
	// 1. AUTORIZAÇÃO — o cliente só devolve o PRÓPRIO pedido.
	//
	// A verificação vem antes de tudo e devolve ErrNotOwner (que o handler
	// traduz em 404). Um atendente/admin pode abrir devolução em nome do
	// cliente — é o fluxo real do balcão e do SAC.
	if !canActOnBehalf(req.Actor.Role) && req.Order.UserID != req.Actor.UserID {
		return Decision{}, ErrNotOwner
	}

	// 2. ESTADO. Pedido não pago não tem o que estornar; cancelado já foi
	// desfeito por outro caminho.
	if !returnableStatus(req.Order.Status) {
		return Decision{}, fmt.Errorf("%w: status %q", ErrOrderNotReturnable, req.Order.Status)
	}

	// 3. PRAZO — é ele que decide a base legal.
	dl := ResolveDeadline(req.Order, req.Now)
	kind := KindRegret
	if !dl.RegretOpen {
		kind = KindDefect
		if !dl.DefectOpen {
			// Fora dos dois prazos. Não há caminho.
			return Decision{}, ErrDefectWindowExpired
		}
	}

	// Vício exige descrição do defeito. Arrependimento NÃO — exigir motivo em
	// direito incondicional é criar atrito onde a lei proíbe atrito.
	if kind == KindDefect && !hasText(req.Reason) {
		return Decision{}, ErrReasonRequired
	}

	// 4. ITENS.
	items, amount, full, err := resolveItems(req)
	if err != nil {
		return Decision{}, err
	}

	// 5. SPLIT. Depende de saber se a devolução é total.
	if req.Order.PaymentSplit && !full {
		return Decision{}, ErrSplitPartialRefund
	}

	// Frete: só volta no arrependimento e só na devolução total.
	shipping := 0.0
	if kind == KindRegret && full {
		shipping = req.Order.ShippingCost
	}

	return Decision{
		Kind: kind,
		// Arrependimento é deferido automaticamente. Mandar para uma fila de
		// aprovação humana um direito que não se avalia só cria o lugar onde
		// ele vai ser negado por engano.
		AutoApproved:   kind == KindRegret,
		Items:          items,
		ItemsAmount:    round2(amount),
		ShippingRefund: round2(shipping),
		TotalRefund:    round2(amount + shipping),
		IsFullReturn:   full,
		Deadline:       dl,
	}, nil
}

// resolveItems valida os itens pedidos e calcula o valor.
func resolveItems(req Request) ([]ResolvedItem, float64, bool, error) {
	if len(req.Wanted) == 0 {
		return nil, 0, false, ErrNoItems
	}

	// Soma quantidades repetidas do mesmo item ANTES de validar. Mandar o mesmo
	// item duas vezes com quantidade 1 num item comprado 1 vez tem que falhar;
	// validar linha a linha deixaria passar. Mesmo bug que checkStock já
	// resolve na criação de pedido.
	wanted := make(map[string]int, len(req.Wanted))
	order := make([]string, 0, len(req.Wanted))
	for _, w := range req.Wanted {
		if w.Quantity <= 0 {
			return nil, 0, false, fmt.Errorf("%w: quantidade inválida para o item %s",
				ErrQuantityExceeded, w.OrderItemID)
		}
		if _, seen := wanted[w.OrderItemID]; !seen {
			order = append(order, w.OrderItemID)
		}
		wanted[w.OrderItemID] += w.Quantity
	}

	resolved := make([]ResolvedItem, 0, len(order))
	total := 0.0
	for _, id := range order {
		it, ok := req.Items[id]
		if !ok {
			return nil, 0, false, fmt.Errorf("%w: %s", ErrItemNotInOrder, id)
		}
		qty := wanted[id]
		// O saldo devolvível desconta o que JÁ foi devolvido antes: comprou 10,
		// devolveu 3 na semana passada, só pode devolver 7 agora.
		remaining := it.Quantity - it.AlreadyReturned
		if qty > remaining {
			return nil, 0, false, fmt.Errorf(
				"%w: item %s — comprados %d, já devolvidos %d, disponível %d, pedido %d",
				ErrQuantityExceeded, id, it.Quantity, it.AlreadyReturned, remaining, qty)
		}
		line := round2(float64(qty) * it.UnitPrice)
		resolved = append(resolved, ResolvedItem{
			OrderItemID: id, ProductID: it.ProductID, Quantity: qty,
			UnitPrice: it.UnitPrice, LineAmount: line,
		})
		total += line
	}

	return resolved, total, isFullReturn(req.Items, wanted), nil
}

// isFullReturn diz se TODO o saldo devolvível do pedido está nesta devolução.
//
// Considera o que já foi devolvido antes: se o cliente devolveu 3 de 10 na
// semana passada e agora devolve os 7 restantes, isso É devolução total do
// saldo — e é o que importa para a trava de split e para o frete.
func isFullReturn(items map[string]OrderItemRef, wanted map[string]int) bool {
	for id, it := range items {
		remaining := it.Quantity - it.AlreadyReturned
		if remaining <= 0 {
			continue // item já integralmente devolvido antes
		}
		if wanted[id] != remaining {
			return false
		}
	}
	return true
}

// canActOnBehalf diz se o papel pode abrir/consultar devolução de outra pessoa.
//
// `seller` NÃO está na lista, de propósito: lojista do marketplace não é
// atendente da loja (a mesma confusão que já custou caro no PDV — ver o aviso
// sobre `seller` no CLAUDE.md).
func canActOnBehalf(role string) bool {
	switch role {
	case RoleAdmin, RoleOperator, RoleStoreOperator, RoleService:
		return true
	}
	return false
}

// CanDecide diz se o papel pode deferir/indeferir uma devolução.
//
// O CLIENTE não pode — nem a própria. Aprovar a própria devolução é o
// equivalente exato de aprovar o próprio desconto no balcão, e essa regra já
// custou uma constraint de banco lá (orders_no_self_approval).
func CanDecide(role string) bool {
	switch role {
	case RoleAdmin, RoleOperator, RoleStoreOperator:
		return true
	}
	return false
}

// returnableStatus — de quais estados de pedido se pode pedir devolução.
//
// `pending_payment` fica de fora: não houve pagamento, então não há estorno; o
// caminho é cancelar. `cancelled` idem.
func returnableStatus(s string) bool {
	switch s {
	case "paid", "picking", "shipped", "delivered":
		return true
	}
	return false
}

func hasText(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// round2 arredonda para centavos. Mesma função (e mesma limitação de float64)
// do handler de pedido — ver o aviso sobre dinheiro em float64 no CLAUDE.md.
func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}
