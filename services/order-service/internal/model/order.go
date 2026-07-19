package model

import "time"

// JSON em camelCase para match direto com app/src/lib/mockOrders.ts

type OrderStatus string
type PaymentMethod string

const (
	StatusPendingPayment OrderStatus = "pending_payment"
	StatusPaid           OrderStatus = "paid"
	StatusPicking        OrderStatus = "picking"
	StatusShipped        OrderStatus = "shipped"
	StatusDelivered      OrderStatus = "delivered"
	StatusCancelled      OrderStatus = "cancelled"

	MethodPix    PaymentMethod = "pix"
	MethodBoleto PaymentMethod = "boleto"
	MethodCard   PaymentMethod = "card"
	// MethodExternal — venda de balcão paga na MAQUININHA DA LOJA, de um
	// adquirente próprio, fora da Appmax. O dinheiro entra por fora do nosso
	// PSP e nenhuma cobrança é criada.
	//
	// PORQUÊ um método próprio e não `card`: a venda de balcão na maquininha
	// era gravada como `card` — valor e desconto certos, meio de pagamento
	// errado. Com isso o livro contábil registrava uma transação de PSP que
	// nunca existiu, a conciliação com a Appmax acusava divergência para
	// sempre e o relatório por método de pagamento mentia.
	//
	// Este enum é fechado em TRÊS lugares que precisam andar juntos:
	//   1. esta constante;
	//   2. o binding `oneof` de CreateOrderRequest, logo abaixo;
	//   3. o tipo Postgres `payment_method` (migrations 001, estendido pela 004).
	MethodExternal PaymentMethod = "external"
)

// Canal de venda. Default 'web' em todo lugar: nenhum pedido histórico e
// nenhum request antigo (sem o campo) vira balcão por acidente.
type OrderChannel string

const (
	ChannelWeb    OrderChannel = "web"
	ChannelBalcao OrderChannel = "balcao"
)

// OrderItem é o item de um pedido. Validação de binding é OBRIGATÓRIA aqui pra
// evitar tamper de preço/quantidade pelo cliente (audit O1-C1):
//   - Quantity > 0 e <= 999 (limites razoáveis pra hardware/ferramentas)
//   - UnitPrice > 0 e <= 999999.99 (cliente não envia preço grátis nem absurdo)
//
// NOTA: o ideal é o backend buscar o preço do catalog-service (audit O2-H5),
// mas até lá pelo menos travamos o range plausível.
type OrderItem struct {
	ProductID  string  `json:"productId" binding:"required,max=64"`
	Name       string  `json:"name" binding:"required,max=255"`
	Icon       string  `json:"icon" binding:"max=16"`
	SellerID   string  `json:"sellerId" binding:"required,max=64"`
	SellerName string  `json:"sellerName" binding:"required,max=255"`
	Quantity   int     `json:"quantity" binding:"required,gt=0,lte=999"`
	UnitPrice  float64 `json:"unitPrice" binding:"required,gt=0,lte=999999.99"`
}

// OrderAddress — campos de endereço com limites de tamanho pra prevenir DoS por
// payload absurdo + XSS (audit O1-C2). CEP é validado por regex (8 dígitos com
// hífen opcional). Estado é UF de 2 chars.
type OrderAddress struct {
	Street       string  `json:"street" binding:"required,max=255"`
	Number       string  `json:"number" binding:"required,max=20"`
	Complement   *string `json:"complement,omitempty" binding:"omitempty,max=100"`
	Neighborhood string  `json:"neighborhood" binding:"required,max=100"`
	City         string  `json:"city" binding:"required,max=100"`
	State        string  `json:"state" binding:"required,len=2"`
	CEP          string  `json:"cep" binding:"required,max=9"`
}

type TrackingEvent struct {
	Status      OrderStatus `json:"status"`
	Location    *string     `json:"location,omitempty"`
	Description string      `json:"description"`
	OccurredAt  time.Time   `json:"occurredAt"`
}

type Order struct {
	ID              string        `json:"id"`
	Number          string        `json:"number"`
	UserID          string        `json:"userId"`
	Status          OrderStatus   `json:"status"`
	PaymentMethod   PaymentMethod `json:"paymentMethod"`
	PaymentID       *string       `json:"paymentId,omitempty"`
	PaymentInfo     *string       `json:"paymentInfo,omitempty"`
	Items           []OrderItem   `json:"items"`
	Subtotal        float64       `json:"subtotal"`
	ShippingCost    float64       `json:"shippingCost"`
	ShippingService string        `json:"shippingService"`
	Total           float64       `json:"total"`
	// Ponteiro porque venda de balcão é retirada no ato: não existe endereço.
	// Para pedido web o JSON continua idêntico ao de antes.
	Address      *OrderAddress `json:"address,omitempty"`
	TrackingCode *string       `json:"trackingCode,omitempty"`

	// -- balcão --------------------------------------------------------------
	// Todos omitempty: a resposta de um pedido web não muda de forma.
	Channel          OrderChannel `json:"channel"`
	StoreID          *string      `json:"storeId,omitempty"`
	OperatorID       *string      `json:"operatorId,omitempty"`
	CustomerID       *string      `json:"customerId,omitempty"`
	CustomerName     *string      `json:"customerName,omitempty"`
	CustomerDocument *string      `json:"customerDocument,omitempty"`
	CustomerPhone    *string      `json:"customerPhone,omitempty"`
	DiscountPct      float64      `json:"discountPct"`
	DiscountAmount   float64      `json:"discountAmount"`
	ApprovalStatus   string       `json:"approvalStatus"`
	ApprovedBy       *string      `json:"approvedBy,omitempty"`
	ApprovedAt       *time.Time   `json:"approvedAt,omitempty"`
	ApprovalNote     *string      `json:"approvalNote,omitempty"`

	// -- liquidação externa (maquininha da loja) ------------------------------
	// Todos omitempty: pedido web e pedido de balcão pago pelo PSP continuam
	// com a mesma forma de JSON de antes.
	ExternalNSU           *string    `json:"externalNsu,omitempty"`
	ExternalBrand         *string    `json:"externalBrand,omitempty"`
	ExternalAuthorization *string    `json:"externalAuthorizationCode,omitempty"`
	ExternalSettledBy     *string    `json:"externalSettledBy,omitempty"`
	ExternalSettledAt     *time.Time `json:"externalSettledAt,omitempty"`

	TrackingEvents []TrackingEvent `json:"trackingEvents,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	PaidAt         *time.Time      `json:"paidAt,omitempty"`
	PickedAt       *time.Time      `json:"pickedAt,omitempty"`
	ShippedAt      *time.Time      `json:"shippedAt,omitempty"`
	DeliveredAt    *time.Time      `json:"deliveredAt,omitempty"`
	CancelledAt    *time.Time      `json:"cancelledAt,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// CreateOrderRequest — payload de POST /api/v1/orders.
// Limites de tamanho previnem DoS por payload absurdo (audit O3-M1).
//
// ShippingCost é ACEITO mas NÃO usado no total: o servidor recalcula o frete a
// partir da tabela `shipping_rates` e do CEP do endereço. O campo continua no
// contrato só pra detectar divergência (frontend com tabela velha, ou tentativa
// de tamper) e logar — remover o campo quebraria o app hoje em produção.
// O cliente escolhe QUAL serviço quer via ShippingService; o preço é do servidor.
//
// BALCÃO — o que muda no contrato:
//
//	Channel  ausente ou "web" = comportamento idêntico ao de sempre.
//	Address  virou ponteiro e a obrigatoriedade saiu do binding para o handler,
//	         que exige endereço quando o canal é `web` e o proíbe quando é
//	         `balcao`. Antes, o PDV mandava um endereço falso ("Retirada no
//	         balcão", CEP 00000-000) só para passar no `binding:"required"` —
//	         e esse endereço falso ia parar na etiqueta de entrega e na cotação
//	         de frete.
//	Discount o cliente manda a PORCENTAGEM pretendida; o valor em reais é
//	         sempre derivado no servidor (balcao.ResolveDiscount). Não existe
//	         campo para o cliente informar o valor do desconto — de propósito.
type CreateOrderRequest struct {
	// `external` é aceito no binding, mas NÃO é liberado por ele: o handler
	// exige canal `balcao` e operador autenticado (ver resolveSaleContext).
	// Um cliente do site que mande paymentMethod=external leva 400 — e o
	// pedido continua nascendo em pending_payment de qualquer forma. Quem o
	// marca como pago é o endpoint de liquidação, nunca a criação.
	PaymentMethod   PaymentMethod `json:"paymentMethod" binding:"required,oneof=pix boleto card external"`
	Items           []OrderItem   `json:"items" binding:"required,min=1,max=100,dive"`
	ShippingCost    float64       `json:"shippingCost" binding:"gte=0,lte=99999.99"`
	ShippingService string        `json:"shippingService" binding:"omitempty,oneof=standard express"`
	Address         *OrderAddress `json:"address" binding:"omitempty"`

	// -- balcão --------------------------------------------------------------
	Channel OrderChannel `json:"channel" binding:"omitempty,oneof=web balcao"`
	// StoreID é opcional e serve só para admin escolher a filial: para o
	// operador, a loja vem do vínculo, nunca do request (ver
	// balcao.CanCreateBalcaoOrder).
	StoreID string `json:"storeId" binding:"omitempty,max=64"`
	// DiscountPct é INTENÇÃO. O servidor recalcula o valor e compara com o teto
	// do cargo — mesma política já aplicada a preço de item e a frete.
	DiscountPct float64 `json:"discountPct" binding:"omitempty,gte=0,lte=100"`
	// Customer identifica para QUEM é a venda. Pode ser um cadastro leve
	// existente (CustomerID) e/ou o snapshot dos dados no ato.
	CustomerID       string `json:"customerId" binding:"omitempty,max=64"`
	CustomerName     string `json:"customerName" binding:"omitempty,max=120"`
	CustomerDocument string `json:"customerDocument" binding:"omitempty,max=18"`
	// Telefone é exigido pela Appmax na cobrança; validado no handler para
	// pedidos de balcão.
	CustomerPhone string `json:"customerPhone" binding:"omitempty,max=20"`
}

// ApprovalRequest — payload de aprovar/recusar desconto.
// Recusa exige justificativa: "recusado" sem motivo obriga o vendedor a voltar
// ao gerente para saber o que fazer, e não deixa rastro do porquê na auditoria.
type ApprovalRequest struct {
	Note *string `json:"note" binding:"omitempty,max=500"`
}

// SettleExternalRequest — payload de POST /api/v1/balcao/orders/:id/settle-external.
//
// Não existe campo de VALOR, e a ausência é deliberada: o valor liquidado é o
// total do pedido, calculado pelo servidor. Deixar o operador informar quanto
// entrou permitiria liquidar um pedido de R$ 2.000 declarando R$ 20 — e a
// diferença sairia pela porta como mercadoria.
//
// Também não existe campo de "pago em" que o cliente escolha livremente: o
// OccurredAt do lançamento é a hora do servidor. Datar a venda para trás é
// como se maquia o fechamento de um dia que já foi conferido.
type SettleExternalRequest struct {
	// NSU é o número do comprovante do adquirente — OBRIGATÓRIO. É o único
	// campo que amarra esta venda à linha do extrato da maquininha; sem ele a
	// liquidação vira "o operador disse que pagou", que não é conciliável.
	NSU string `json:"nsu" binding:"required,max=40"`
	// Bandeira e código de autorização são opcionais: nem todo comprovante
	// traz, e a conciliação se faz pelo NSU. Quando vêm, entram na trilha.
	Brand             string  `json:"brand" binding:"omitempty,max=32"`
	AuthorizationCode string  `json:"authorizationCode" binding:"omitempty,max=32"`
	Note              *string `json:"note" binding:"omitempty,max=500"`
}

// ShippingQuoteRequest — payload de POST /api/v1/shipping/quote.
// O carrinho chama isso com o CEP antes do checkout.
type ShippingQuoteRequest struct {
	CEP       string  `json:"cep" binding:"required,max=9"`
	Subtotal  float64 `json:"subtotal" binding:"gte=0,lte=9999999.99"`
	ItemCount int     `json:"itemCount" binding:"gte=0,lte=9999"`
}

// FulfillmentRequest — payload dos endpoints de operação (separar/enviar/entregar).
type FulfillmentRequest struct {
	TrackingCode *string `json:"trackingCode" binding:"omitempty,max=64"`
	Location     *string `json:"location" binding:"omitempty,max=255"`
	Note         *string `json:"note" binding:"omitempty,max=500"`
}
