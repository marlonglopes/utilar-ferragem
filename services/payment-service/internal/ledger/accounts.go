// Package ledger é o livro contábil do payment-service: partidas dobradas,
// imutável, em centavos inteiros.
//
// # Por que um ledger e não "somar a tabela payments"
//
// `payments` responde "esse pagamento foi confirmado?". Não responde "quanto
// entrou de receita em junho, quanto foi taxa de gateway, quanto voltou em
// estorno e quanto devemos aos vendedores". Um SELECT SUM(amount) sobre
// payments erra por construção: ignora a taxa do PSP (que sai do mesmo
// dinheiro), conta estorno como se não tivesse acontecido, e não tem como
// separar o que é nosso do que é do vendedor.
//
// # As três regras que não se negociam
//
//  1. Centavos inteiros (int64). Nunca float. Ver internal/psp/appmaxv1/money_test.go.
//  2. Toda transação soma zero. Garantido no banco por constraint trigger.
//  3. Imutável. Erro se corrige com lançamento de estorno, jamais com UPDATE.
package ledger

// Account é o código de uma conta do plano de contas (migration 005).
type Account string

const (
	// Ativo
	AcctCaixaPSP Account = "1.1.1" // dinheiro capturado pelo PSP, ainda não sacado
	AcctBanco    Account = "1.1.2" // saldo já liquidado na conta da empresa

	// Passivo
	AcctRepasseVendedor Account = "2.1.1" // o que devemos ao vendedor pelo split

	// Receita (3.1.8 e 3.1.9 são REDUTORAS — saldo devedor é o esperado)
	AcctReceitaBruta Account = "3.1.1"
	AcctEstornos     Account = "3.1.8"
	AcctChargebacks  Account = "3.1.9"

	// Despesa
	AcctTaxaPSP         Account = "4.1.1"
	AcctTaxaAntecipacao Account = "4.1.2"
	AcctCustoRepasse    Account = "4.2.1"
)

// Side é o lado da partida.
type Side string

const (
	Debit  Side = "debit"
	Credit Side = "credit"
)

// Kind é a natureza econômica do lançamento. Entra na chave de idempotência
// (kind, source_type, source_id), então mudar um valor destes depois de já
// existir dado em produção quebra a deduplicação — não mexa sem migration.
type Kind string

const (
	KindSale             Kind = "sale"              // captura do pedido
	KindPSPFee           Kind = "psp_fee"           // MDR/taxa fixa do gateway
	KindRefund           Kind = "refund"            // estorno ao comprador
	KindChargeback       Kind = "chargeback"        // contestação perdida
	KindSellerSplit      Kind = "seller_split"      // obrigação com o vendedor
	KindSellerWithdrawal Kind = "seller_withdrawal" // saque do vendedor (quita a obrigação)
	KindPayout           Kind = "payout"            // nosso saque PSP → banco
	KindAnticipationFee  Kind = "anticipation_fee"  // custo de antecipar recebível
	KindReversal         Kind = "reversal"          // correção de lançamento errado
	KindAdjustment       Kind = "adjustment"        // ajuste manual (exige justificativa)
)

// SourceType identifica o documento de origem — todo lançamento aponta pra um.
// Lançamento sem origem rastreável é chute, e chute não vai pro livro.
type SourceType string

const (
	SourcePayment      SourceType = "payment"
	SourceWebhookEvent SourceType = "webhook_event"
	SourceOrder        SourceType = "order"
	SourceRecipient    SourceType = "recipient"
	SourceLedgerTx     SourceType = "ledger_transaction" // usado por reversões
	SourceManual       SourceType = "manual"
)

// AccountMeta descreve uma conta pro relatório e pra exportação ao contador.
type AccountMeta struct {
	Code       Account
	Name       string
	Type       string
	NormalSide Side
}

// ChartOfAccounts espelha o que a migration 005 insere. Duplicado em Go pra
// que relatórios e export CSV/OFX rodem sem um round-trip ao banco por linha.
// O teste TestPlanoDeContasBateComAMigration garante que os dois não divergem.
var ChartOfAccounts = []AccountMeta{
	{AcctCaixaPSP, "Caixa em trânsito no PSP", "asset", Debit},
	{AcctBanco, "Banco - conta movimento", "asset", Debit},
	{AcctRepasseVendedor, "Repasses a vendedores", "liability", Credit},
	{AcctReceitaBruta, "Receita bruta de vendas", "revenue", Credit},
	{AcctEstornos, "Estornos e devoluções", "revenue", Debit},
	{AcctChargebacks, "Chargebacks", "revenue", Debit},
	{AcctTaxaPSP, "Taxas do gateway (PSP)", "expense", Debit},
	{AcctTaxaAntecipacao, "Taxa de antecipação", "expense", Debit},
	{AcctCustoRepasse, "Custo de repasse a vendedor", "expense", Debit},
}

// MetaOf devolve os metadados de uma conta.
func MetaOf(a Account) (AccountMeta, bool) {
	for _, m := range ChartOfAccounts {
		if m.Code == a {
			return m, true
		}
	}
	return AccountMeta{}, false
}
