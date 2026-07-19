package ledger

import (
	"fmt"
	"time"
)

// Este arquivo traduz FATOS DE NEGÓCIO em lançamentos contábeis. É o lugar onde
// alguém que entende de contabilidade consegue conferir se o Utilar está
// registrando dinheiro do jeito certo, sem ler SQL.
//
// Convenção de leitura das partidas: D = débito, C = crédito.
//   * Ativo e despesa AUMENTAM no débito.
//   * Passivo e receita AUMENTAM no crédito.

// SaleInput é a captura de um pedido.
type SaleInput struct {
	PaymentID   string    // documento de origem
	OrderID     string    // referência de negócio
	OccurredAt  time.Time // quando o PSP capturou, não quando processamos
	GrossCents  Cents     // valor cheio pago pelo comprador
	PSPFeeCents Cents     // taxa retida pelo gateway (0 se ainda desconhecida)
	Method      string    // pix | boleto | card
	RequestID   string
}

// Sale — venda capturada.
//
//	D 1.1.1 Caixa em trânsito no PSP    bruto
//	    C 3.1.1 Receita bruta de vendas     bruto
//	D 4.1.1 Taxas do gateway            taxa
//	    C 1.1.1 Caixa em trânsito no PSP    taxa
//
// A receita é registrada BRUTA e a taxa vira despesa própria. Registrar
// líquido ("entrou 95") esconderia o custo do gateway do DRE e tornaria
// impossível responder "quanto pagamos de MDR neste trimestre" — que é a
// pergunta que mais paga a conta de renegociar com o PSP.
func Sale(in SaleInput) TxInput {
	postings := []Posting{
		{Account: AcctCaixaPSP, Side: Debit, Amount: in.GrossCents, PaymentMethod: in.Method,
			Memo: "captura do pedido " + in.OrderID},
		{Account: AcctReceitaBruta, Side: Credit, Amount: in.GrossCents, PaymentMethod: in.Method,
			Memo: "receita bruta do pedido " + in.OrderID},
	}
	if in.PSPFeeCents > 0 {
		postings = append(postings,
			Posting{Account: AcctTaxaPSP, Side: Debit, Amount: in.PSPFeeCents, PaymentMethod: in.Method,
				Memo: "taxa do gateway sobre o pedido " + in.OrderID},
			Posting{Account: AcctCaixaPSP, Side: Credit, Amount: in.PSPFeeCents, PaymentMethod: in.Method,
				Memo: "retenção de taxa pelo gateway"},
		)
	}
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindSale,
		SourceType:  SourcePayment,
		SourceID:    in.PaymentID,
		Description: fmt.Sprintf("Venda %s (pedido %s)", in.Method, in.OrderID),
		RequestID:   in.RequestID,
		Postings:    postings,
	}
}

// -- Liquidação externa (maquininha da loja) ---------------------------------

// MethodExternal é o rótulo de método das partidas de liquidação externa.
//
// Ele NÃO é "card". A venda de balcão paga na maquininha era gravada como
// `card`, com valor e desconto certos — e método errado. Consequência: o livro
// registrava uma transação de PSP que nunca existiu, a conciliação com a Appmax
// acusava divergência para sempre e o relatório por método de pagamento mentia.
const MethodExternal = "external"

// ExternalSaleInput é a venda de balcão liquidada FORA do nosso PSP.
type ExternalSaleInput struct {
	// OrderID é o documento de origem: não existe payment_id porque não
	// existe pagamento nosso. É também a chave de idempotência.
	OrderID string
	// NSU é o número sequencial único do comprovante do adquirente — o único
	// campo que amarra esta venda ao extrato da maquininha. Vai no memo das
	// duas partidas para que quem confere o extrato ache a linha pelo NSU sem
	// sair do CSV do contador.
	NSU           string
	StoreID       string
	OperatorID    string
	Brand         string // bandeira do cartão, quando informada
	Authorization string // código de autorização do adquirente, quando informado
	OccurredAt    time.Time
	GrossCents    Cents
	RequestID     string
	SettledBy     string // quem liquidou — vai para created_by do lançamento
}

// ExternalSale — venda liquidada por adquirente externo.
//
//	D 1.1.3 Caixa em trânsito no adquirente externo   bruto
//	    C 3.1.1 Receita bruta de vendas                   bruto
//
// Não há partida de taxa: o MDR da maquininha não passa pelo nosso sistema e é
// desconhecido no ato da venda. Ele entra depois, na conciliação do extrato do
// adquirente, como despesa própria — inventar aqui uma taxa estimada seria pôr
// no livro um número que ninguém pode conferir.
//
// A receita entra na MESMA 3.1.1 de propósito: faturamento é faturamento,
// independente de por onde o dinheiro entrou. O que separa as duas origens é a
// conta de ativo (1.1.3 vs 1.1.1), o Kind e o rótulo de método — que é
// exatamente a granularidade que o DRE e o relatório por método precisam.
func ExternalSale(in ExternalSaleInput) TxInput {
	memo := "venda de balcão liquidada na maquininha (NSU " + in.NSU + ")"
	return TxInput{
		OccurredAt: in.OccurredAt,
		Kind:       KindExternalSale,
		SourceType: SourceExternalSettlement,
		// Chave de idempotência = o pedido. Liquidar duas vezes o mesmo pedido
		// bate no UNIQUE (kind, source_type, source_id) e vira ErrDuplicate,
		// tratado como no-op pelo chamador — o mesmo mecanismo que impede o
		// webhook reentregue de dobrar a receita.
		SourceID: in.OrderID,
		Description: fmt.Sprintf("Venda externa (maquininha) do pedido %s — NSU %s, loja %s",
			in.OrderID, in.NSU, in.StoreID),
		RequestID: in.RequestID,
		CreatedBy: in.SettledBy,
		Postings: []Posting{
			{Account: AcctCaixaAdquirenteExterno, Side: Debit, Amount: in.GrossCents,
				PaymentMethod: MethodExternal, Memo: memo},
			{Account: AcctReceitaBruta, Side: Credit, Amount: in.GrossCents,
				PaymentMethod: MethodExternal, Memo: "receita bruta do pedido " + in.OrderID},
		},
	}
}

// PSPFeeInput registra a taxa DEPOIS, quando o valor só é conhecido na
// conciliação (comum em cartão parcelado, onde o MDR efetivo sai no extrato).
type PSPFeeInput struct {
	PaymentID  string
	OccurredAt time.Time
	FeeCents   Cents
	Method     string
	RequestID  string
}

// PSPFee — D 4.1.1 taxa / C 1.1.1 caixa.
func PSPFee(in PSPFeeInput) TxInput {
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindPSPFee,
		SourceType:  SourcePayment,
		SourceID:    in.PaymentID,
		Description: "Taxa do gateway (conciliação posterior)",
		RequestID:   in.RequestID,
		Postings: []Posting{
			{Account: AcctTaxaPSP, Side: Debit, Amount: in.FeeCents, PaymentMethod: in.Method},
			{Account: AcctCaixaPSP, Side: Credit, Amount: in.FeeCents, PaymentMethod: in.Method},
		},
	}
}

// RefundInput é o estorno ao comprador (total ou parcial).
type RefundInput struct {
	PaymentID  string
	OrderID    string
	OccurredAt time.Time
	Cents      Cents
	Method     string
	Partial    bool
	RequestID  string
}

// Refund — D 3.1.8 Estornos / C 1.1.1 Caixa.
//
// Vai numa conta REDUTORA de receita, não como estorno da 3.1.1: a venda
// aconteceu de fato e o contador precisa ver bruto e devolução separados
// (relevante inclusive pra apuração de imposto sobre vendas canceladas).
func Refund(in RefundInput) TxInput {
	tipo := "total"
	if in.Partial {
		tipo = "parcial"
	}
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindRefund,
		SourceType:  SourcePayment,
		SourceID:    in.PaymentID + ":" + tipo,
		Description: fmt.Sprintf("Estorno %s do pedido %s", tipo, in.OrderID),
		RequestID:   in.RequestID,
		Postings: []Posting{
			{Account: AcctEstornos, Side: Debit, Amount: in.Cents, PaymentMethod: in.Method,
				Memo: "estorno " + tipo},
			{Account: AcctCaixaPSP, Side: Credit, Amount: in.Cents, PaymentMethod: in.Method},
		},
	}
}

// ChargebackInput — contestação PERDIDA (chargeback_lost). Chargeback ganho
// não gera lançamento: o dinheiro nunca saiu.
type ChargebackInput struct {
	PaymentID  string
	OrderID    string
	OccurredAt time.Time
	Cents      Cents
	Method     string
	RequestID  string
}

// Chargeback — D 3.1.9 Chargebacks / C 1.1.1 Caixa.
func Chargeback(in ChargebackInput) TxInput {
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindChargeback,
		SourceType:  SourcePayment,
		SourceID:    in.PaymentID,
		Description: "Chargeback perdido do pedido " + in.OrderID,
		RequestID:   in.RequestID,
		Postings: []Posting{
			{Account: AcctChargebacks, Side: Debit, Amount: in.Cents, PaymentMethod: in.Method},
			{Account: AcctCaixaPSP, Side: Credit, Amount: in.Cents, PaymentMethod: in.Method},
		},
	}
}

// SellerSplitInput é o split: a parte do pedido que é do vendedor.
type SellerSplitInput struct {
	PaymentID  string
	OrderID    string
	SellerID   string
	OccurredAt time.Time
	Cents      Cents
	Method     string
	RequestID  string
}

// SellerSplit — D 4.2.1 Custo de repasse / C 2.1.1 Repasses a vendedores.
//
// Reconhece a OBRIGAÇÃO, não o pagamento: o dinheiro ainda está no caixa do
// PSP. Ele só sai do nosso passivo quando o vendedor saca (SellerWithdrawal).
// Marketplace que credita direto contra caixa some com o passivo e passa a
// reportar como próprio dinheiro que é de terceiro.
func SellerSplit(in SellerSplitInput) TxInput {
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindSellerSplit,
		SourceType:  SourcePayment,
		SourceID:    in.PaymentID + ":" + in.SellerID,
		Description: fmt.Sprintf("Split do pedido %s para o vendedor %s", in.OrderID, in.SellerID),
		RequestID:   in.RequestID,
		Postings: []Posting{
			{Account: AcctCustoRepasse, Side: Debit, Amount: in.Cents,
				PaymentMethod: in.Method, SellerID: in.SellerID},
			{Account: AcctRepasseVendedor, Side: Credit, Amount: in.Cents,
				PaymentMethod: in.Method, SellerID: in.SellerID},
		},
	}
}

// SellerWithdrawalInput é o saque do vendedor (quita a obrigação).
type SellerWithdrawalInput struct {
	WithdrawalID    string // id do saque no PSP — documento de origem
	SellerID        string
	RecipientHash   string
	OccurredAt      time.Time
	Cents           Cents // valor sacado (líquido de antecipação)
	AnticipationFee Cents // custo de antecipação, se houve
	RequestID       string
}

// SellerWithdrawal — D 2.1.1 Repasses (baixa do passivo) / C 1.1.1 Caixa.
// Com antecipação: D 4.1.2 taxa / C 1.1.1 caixa, no mesmo documento.
func SellerWithdrawal(in SellerWithdrawalInput) TxInput {
	postings := []Posting{
		{Account: AcctRepasseVendedor, Side: Debit, Amount: in.Cents, SellerID: in.SellerID,
			Memo: "baixa do repasse (saque)"},
		{Account: AcctCaixaPSP, Side: Credit, Amount: in.Cents, SellerID: in.SellerID},
	}
	if in.AnticipationFee > 0 {
		postings = append(postings,
			Posting{Account: AcctTaxaAntecipacao, Side: Debit, Amount: in.AnticipationFee, SellerID: in.SellerID},
			Posting{Account: AcctCaixaPSP, Side: Credit, Amount: in.AnticipationFee, SellerID: in.SellerID},
		)
	}
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindSellerWithdrawal,
		SourceType:  SourceRecipient,
		SourceID:    in.WithdrawalID,
		Description: "Saque do vendedor " + in.SellerID,
		RequestID:   in.RequestID,
		Postings:    postings,
	}
}

// PayoutInput é o NOSSO saque: PSP → conta bancária da empresa.
type PayoutInput struct {
	PayoutID   string
	OccurredAt time.Time
	Cents      Cents
	RequestID  string
}

// Payout — D 1.1.2 Banco / C 1.1.1 Caixa no PSP. Movimento entre ativos:
// não é receita, não passa pelo DRE. Confundir saque com receita é o erro
// clássico que faz o faturamento aparecer dobrado.
func Payout(in PayoutInput) TxInput {
	return TxInput{
		OccurredAt:  in.OccurredAt,
		Kind:        KindPayout,
		SourceType:  SourceManual,
		SourceID:    in.PayoutID,
		Description: "Transferência PSP → conta bancária",
		RequestID:   in.RequestID,
		Postings: []Posting{
			{Account: AcctBanco, Side: Debit, Amount: in.Cents},
			{Account: AcctCaixaPSP, Side: Credit, Amount: in.Cents},
		},
	}
}
