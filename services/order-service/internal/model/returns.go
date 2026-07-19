package model

import "time"

// ============================================================================
// Devolução e troca — contrato HTTP
//
// JSON em camelCase, igual ao resto do serviço.
// As REGRAS (prazo, autorização, quantidade, split) moram em internal/returns,
// como funções puras. Aqui só o formato do que entra e do que sai.
// ============================================================================

// ReturnItemRequest é um item que o cliente quer devolver.
//
// PORQUÊ o cliente manda `orderItemId` e não `productId`: o mesmo produto pode
// aparecer em duas linhas do pedido (comprado em momentos diferentes do
// carrinho, com preços distintos por promoção). Devolver "o produto X" seria
// ambíguo sobre QUAL preço estornar.
//
// Não existe campo de VALOR, e a ausência é deliberada — mesma decisão de
// SettleExternalRequest. O valor a estornar é calculado no servidor a partir do
// preço SNAPSHOT gravado em order_items. Deixar o cliente informar quanto quer
// de volta é deixá-lo ditar quanto dinheiro sai.
type ReturnItemRequest struct {
	OrderItemID string `json:"orderItemId" binding:"required,max=64"`
	Quantity    int    `json:"quantity" binding:"required,gt=0,lte=999"`
}

// CreateReturnRequest — payload de POST /api/v1/orders/:id/returns.
//
// Não existe campo `kind`: a base legal (arrependimento art. 49 × vício
// art. 26) é DERIVADA da data, não escolhida.
//
// Se o cliente escolhesse, todo mundo marcaria "arrependimento" — que não tem
// análise — inclusive fora dos 7 dias. Se a loja escolhesse, todo
// arrependimento viraria "vício" para poder ser analisado. A base legal é
// consequência da data, e a data é fato.
type CreateReturnRequest struct {
	Items []ReturnItemRequest `json:"items" binding:"required,min=1,max=100,dive"`

	// Reason é OPCIONAL no contrato de propósito.
	//
	// ⚠️ Dentro dos 7 dias o motivo NÃO é exigido nem avaliado: o art. 49 é
	// direito incondicional. Marcar `binding:"required"` aqui obrigaria o
	// cliente a justificar uma desistência que a lei diz que não precisa ser
	// justificada. Fora do prazo (vício, art. 26) a exigência existe — e é
	// aplicada em internal/returns.Evaluate, onde a data é conhecida.
	Reason string `json:"reason" binding:"omitempty,max=1000"`
}

// ReturnDecisionRequest — payload de aprovar/recusar.
// Recusa exige justificativa (e arrependimento não pode ser recusado de jeito
// nenhum — ver returns.CanReject).
type ReturnDecisionRequest struct {
	Note string `json:"note" binding:"omitempty,max=1000"`
}

// ReturnReceiveRequest — payload de "mercadoria conferida na loja".
//
// ⚠️ É este endpoint que devolve o estoque. Não o de solicitação, não o de
// aprovação: conferir é o momento em que a loja sabe que o produto está
// fisicamente lá e em condição de ser revendido.
type ReturnReceiveRequest struct {
	Note string `json:"note" binding:"omitempty,max=1000"`
	// RestockableItems permite ao conferente marcar que parte do que voltou NÃO
	// volta ao estoque (chegou quebrado). Vazio = tudo volta.
	//
	// PORQUÊ existe: mandar de volta à vitrine um produto que voltou destruído
	// é vender lixo para o próximo cliente. O estorno ao consumidor continua
	// devido (é outra discussão, e a loja perde essa discussão quase sempre);
	// o que muda é o que vai para a prateleira.
	RestockableItems []ReturnItemRequest `json:"restockableItems" binding:"omitempty,max=100,dive"`
}

// ReturnItem é um item devolvido, na resposta.
type ReturnItem struct {
	OrderItemID string  `json:"orderItemId"`
	ProductID   string  `json:"productId"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"`
	LineAmount  float64 `json:"lineAmount"`
}

// Return é o pedido de devolução como o cliente e o atendente o veem.
type Return struct {
	ID      string `json:"id"`
	OrderID string `json:"orderId"`
	UserID  string `json:"userId"`

	// Kind e Status espelham os enums do banco.
	Kind   string `json:"kind"`   // regret | defect
	Status string `json:"status"` // requested | approved | ...

	// LegalBasis é o texto que o cliente lê ("Arrependimento — CDC art. 49").
	// Traduzir código legal em linguagem de gente é parte da obrigação de
	// informação do próprio CDC (art. 6º, III).
	LegalBasis string `json:"legalBasis"`

	Reason string `json:"reason,omitempty"`

	Items []ReturnItem `json:"items"`

	RefundAmount   float64 `json:"refundAmount"`
	RefundShipping float64 `json:"refundShipping"`
	RefundTotal    float64 `json:"refundTotal"`

	// DeadlineAt e DeadlineBasis são o prazo APLICADO, congelado no ato.
	// Aparecem na resposta porque o cliente tem direito de saber com base em
	// que data a loja contou o prazo dele.
	DeadlineAt    *time.Time `json:"deadlineAt,omitempty"`
	DeadlineBasis *time.Time `json:"deadlineBasis,omitempty"`
	BasisSource   string     `json:"basisSource"`

	DecidedBy    *string    `json:"decidedBy,omitempty"`
	DecidedAt    *time.Time `json:"decidedAt,omitempty"`
	DecisionNote *string    `json:"decisionNote,omitempty"`

	RequestedAt time.Time  `json:"requestedAt"`
	ShippedAt   *time.Time `json:"shippedAt,omitempty"`
	ReceivedAt  *time.Time `json:"receivedAt,omitempty"`
	RefundedAt  *time.Time `json:"refundedAt,omitempty"`

	// StockReturned e LedgerPosted são pendências operacionais visíveis.
	// Expostas só para papel interno (o handler as omite para o cliente): o
	// comprador não precisa saber que o lançamento contábil ficou atrasado.
	StockReturned bool `json:"stockReturned,omitempty"`
	LedgerPosted  bool `json:"ledgerPosted,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// LegalBasisText traduz a base legal para o cliente.
func LegalBasisText(kind string) string {
	switch kind {
	case "regret":
		return "Direito de arrependimento — CDC art. 49 (7 dias a contar do recebimento)"
	case "defect":
		return "Vício do produto — CDC art. 26"
	default:
		return ""
	}
}
