package ledger

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A invariante central do livro: TODA transação soma zero. Se este teste passar
// a falhar, o sistema está criando ou destruindo dinheiro.
func TestPartidasDobradasSomamZero(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	casos := map[string]TxInput{
		"venda sem taxa": Sale(SaleInput{
			PaymentID: "p1", OrderID: "o1", OccurredAt: now, GrossCents: 26400, Method: "pix"}),
		"venda com taxa do PSP": Sale(SaleInput{
			PaymentID: "p2", OrderID: "o2", OccurredAt: now, GrossCents: 128450,
			PSPFeeCents: 5138, Method: "card"}),
		"taxa lançada depois": PSPFee(PSPFeeInput{
			PaymentID: "p3", OccurredAt: now, FeeCents: 349, Method: "boleto"}),
		"estorno total": Refund(RefundInput{
			PaymentID: "p4", OrderID: "o4", OccurredAt: now, Cents: 3490, Method: "pix"}),
		"estorno parcial": Refund(RefundInput{
			PaymentID: "p5", OrderID: "o5", OccurredAt: now, Cents: 1000, Method: "pix", Partial: true}),
		"chargeback": Chargeback(ChargebackInput{
			PaymentID: "p6", OrderID: "o6", OccurredAt: now, Cents: 47500, Method: "card"}),
		"split para vendedor": SellerSplit(SellerSplitInput{
			PaymentID: "p7", OrderID: "o7", SellerID: "s1", OccurredAt: now, Cents: 20000, Method: "pix"}),
		"saque do vendedor": SellerWithdrawal(SellerWithdrawalInput{
			WithdrawalID: "w1", SellerID: "s1", OccurredAt: now, Cents: 20000}),
		"saque com antecipação": SellerWithdrawal(SellerWithdrawalInput{
			WithdrawalID: "w2", SellerID: "s1", OccurredAt: now, Cents: 20000, AnticipationFee: 800}),
		"transferência PSP→banco": Payout(PayoutInput{
			PayoutID: "po1", OccurredAt: now, Cents: 500000}),
	}

	for nome, tx := range casos {
		t.Run(nome, func(t *testing.T) {
			if err := tx.Validate(); err != nil {
				t.Fatalf("lançamento inválido: %v", err)
			}
			var debitos, creditos Cents
			for _, p := range tx.Postings {
				if p.Side == Debit {
					debitos += p.Amount
				} else {
					creditos += p.Amount
				}
			}
			if debitos != creditos {
				t.Fatalf("NÃO FECHA: débitos=%d créditos=%d diferença=%d centavos",
					debitos, creditos, debitos-creditos)
			}
			if debitos == 0 {
				t.Fatal("lançamento de valor zero não é fato contábil")
			}
			// Todo lançamento aponta pro documento de origem. Sem isso o livro
			// não é conciliável e o auditor rejeita.
			if tx.SourceID == "" || tx.SourceType == "" {
				t.Fatal("lançamento sem documento de origem")
			}
		})
	}
}

func TestValidateRecusaLancamentoDesbalanceado(t *testing.T) {
	tx := TxInput{
		Kind: KindSale, SourceType: SourcePayment, SourceID: "p1",
		Postings: []Posting{
			{Account: AcctCaixaPSP, Side: Debit, Amount: 10000},
			{Account: AcctReceitaBruta, Side: Credit, Amount: 9999}, // 1 centavo sumiu
		},
	}
	err := tx.Validate()
	if !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("um centavo de diferença passou: %v", err)
	}
	if !strings.Contains(err.Error(), "1 centavos") {
		t.Errorf("erro deve dizer o tamanho da diferença: %v", err)
	}
}

func TestValidateRecusaEntradasInvalidas(t *testing.T) {
	base := func(p ...Posting) TxInput {
		return TxInput{Kind: KindSale, SourceType: SourcePayment, SourceID: "p1", Postings: p}
	}
	casos := map[string]TxInput{
		"partida única": base(Posting{Account: AcctCaixaPSP, Side: Debit, Amount: 100}),
		"valor negativo": base(
			Posting{Account: AcctCaixaPSP, Side: Debit, Amount: -100},
			Posting{Account: AcctReceitaBruta, Side: Credit, Amount: -100}),
		"valor zero": base(
			Posting{Account: AcctCaixaPSP, Side: Debit, Amount: 0},
			Posting{Account: AcctReceitaBruta, Side: Credit, Amount: 0}),
		"conta inexistente": base(
			Posting{Account: "9.9.9", Side: Debit, Amount: 100},
			Posting{Account: AcctReceitaBruta, Side: Credit, Amount: 100}),
		"side inválido": base(
			Posting{Account: AcctCaixaPSP, Side: "meio-débito", Amount: 100},
			Posting{Account: AcctReceitaBruta, Side: Credit, Amount: 100}),
		"sem documento de origem": {Kind: KindSale, SourceType: SourcePayment, Postings: []Posting{
			{Account: AcctCaixaPSP, Side: Debit, Amount: 100},
			{Account: AcctReceitaBruta, Side: Credit, Amount: 100}}},
	}
	for nome, tx := range casos {
		t.Run(nome, func(t *testing.T) {
			if err := tx.Validate(); err == nil {
				t.Fatal("lançamento inválido foi aceito")
			}
		})
	}
}

// Valor negativo com side='debit' seria um crédito disfarçado e furaria todo
// relatório que agrupa por side. O sinal é o Side, nunca o valor.
func TestValorNegativoNuncaEhAceito(t *testing.T) {
	tx := TxInput{
		Kind: KindRefund, SourceType: SourcePayment, SourceID: "p1",
		Postings: []Posting{
			{Account: AcctCaixaPSP, Side: Debit, Amount: -5000},
			{Account: AcctEstornos, Side: Debit, Amount: 5000},
		},
	}
	if err := tx.Validate(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("valor negativo aceito: %v", err)
	}
}

// A venda é registrada BRUTA com a taxa como despesa própria. Registrar
// líquido esconderia o custo do gateway do DRE.
func TestVendaRegistraReceitaBrutaComTaxaSeparada(t *testing.T) {
	tx := Sale(SaleInput{PaymentID: "p1", OrderID: "o1", GrossCents: 10000,
		PSPFeeCents: 500, Method: "card"})

	saldos := map[Account]Cents{}
	for _, p := range tx.Postings {
		if p.Side == Debit {
			saldos[p.Account] += p.Amount
		} else {
			saldos[p.Account] -= p.Amount
		}
	}
	// Receita bruta creditada pelo valor CHEIO, não pelo líquido.
	if got := -saldos[AcctReceitaBruta]; got != 10000 {
		t.Errorf("receita bruta = %d, esperado 10000 (valor cheio, não líquido)", got)
	}
	if got := saldos[AcctTaxaPSP]; got != 500 {
		t.Errorf("taxa do PSP = %d, esperado 500", got)
	}
	// Caixa fica com o líquido.
	if got := saldos[AcctCaixaPSP]; got != 9500 {
		t.Errorf("caixa = %d, esperado 9500 (bruto - taxa)", got)
	}
}

// Split reconhece OBRIGAÇÃO (passivo), não saída de caixa: o dinheiro ainda
// está no PSP. Marketplace que credita direto contra caixa reporta como
// próprio dinheiro que é de terceiro.
func TestSplitCriaPassivoENaoMexeNoCaixa(t *testing.T) {
	tx := SellerSplit(SellerSplitInput{PaymentID: "p1", OrderID: "o1", SellerID: "s1", Cents: 7000})
	for _, p := range tx.Postings {
		if p.Account == AcctCaixaPSP {
			t.Fatal("split não pode mexer no caixa — o dinheiro ainda está no PSP")
		}
		if p.Account == AcctRepasseVendedor && p.Side != Credit {
			t.Error("repasse a vendedor é passivo: aumenta no CRÉDITO")
		}
		if p.SellerID != "s1" {
			t.Error("toda partida de split precisa carregar o seller_id pro relatório por vendedor")
		}
	}
}

// O saque quita o passivo criado pelo split — os dois têm que se anular.
func TestSplitSeguidoDeSaqueZeraOPassivo(t *testing.T) {
	split := SellerSplit(SellerSplitInput{PaymentID: "p1", SellerID: "s1", Cents: 7000})
	saque := SellerWithdrawal(SellerWithdrawalInput{WithdrawalID: "w1", SellerID: "s1", Cents: 7000})

	var passivo Cents
	for _, tx := range []TxInput{split, saque} {
		for _, p := range tx.Postings {
			if p.Account != AcctRepasseVendedor {
				continue
			}
			if p.Side == Credit {
				passivo += p.Amount
			} else {
				passivo -= p.Amount
			}
		}
	}
	if passivo != 0 {
		t.Fatalf("passivo com o vendedor ficou em %d centavos após o saque (esperado 0)", passivo)
	}
}

// Saque nosso é movimento entre ativos — não é receita e não passa pelo DRE.
func TestPayoutNaoTocaContaDeReceita(t *testing.T) {
	for _, p := range Payout(PayoutInput{PayoutID: "po1", Cents: 100000}).Postings {
		if m, _ := MetaOf(p.Account); m.Type == "revenue" {
			t.Fatalf("saque tocou conta de receita (%s) — faturamento apareceria dobrado", p.Account)
		}
	}
}

// Chave de idempotência: dois estornos do mesmo pagamento (um total, um
// parcial) NÃO podem colidir na constraint UNIQUE.
func TestEstornoTotalEParcialTemChavesDistintas(t *testing.T) {
	total := Refund(RefundInput{PaymentID: "p1", Cents: 1000})
	parcial := Refund(RefundInput{PaymentID: "p1", Cents: 300, Partial: true})
	if total.SourceID == parcial.SourceID {
		t.Fatalf("estorno total e parcial colidem na chave de idempotência (%q)", total.SourceID)
	}
}

func TestSplitDeVendedoresDiferentesTemChavesDistintas(t *testing.T) {
	a := SellerSplit(SellerSplitInput{PaymentID: "p1", SellerID: "s1", Cents: 100})
	b := SellerSplit(SellerSplitInput{PaymentID: "p1", SellerID: "s2", Cents: 100})
	if a.SourceID == b.SourceID {
		t.Fatal("dois vendedores no mesmo pedido colidiriam — só um split seria lançado")
	}
}

// ===================== dinheiro =====================

// Espelha internal/psp/appmaxv1/money_test.go: dinheiro em float64 é a classe
// de bug mais cara. Aqui a garantia é que o livro NUNCA converte de volta pra
// float pra somar.
func TestCentsNuncaPerdeCentavoEmSomaLonga(t *testing.T) {
	var soma Cents
	for i := 0; i < 100000; i++ {
		soma += Cents(1999) // R$ 19,99
	}
	if want := Cents(199900000); soma != want {
		t.Fatalf("soma = %d, esperado %d", soma, want)
	}

	// O mesmo em float64 acumula erro — é exatamente por isso que não usamos.
	var somaFloat float64
	for i := 0; i < 100000; i++ {
		somaFloat += 19.99
	}
	if int64(somaFloat*100) == int64(soma) {
		t.Log("float64 acertou nesta plataforma; o teste acima segue sendo a garantia real")
	}
}

func TestCentsStringFormataEmReais(t *testing.T) {
	casos := map[Cents]string{
		0:      "R$ 0,00",
		1:      "R$ 0,01",
		1999:   "R$ 19,99",
		128450: "R$ 1284,50",
		-3490:  "-R$ 34,90",
	}
	for c, want := range casos {
		if got := c.String(); got != want {
			t.Errorf("Cents(%d).String() = %q, esperado %q", c, got, want)
		}
	}
}

func TestPeriodRange(t *testing.T) {
	from, to, err := Period("2026-07").Range()
	if err != nil {
		t.Fatal(err)
	}
	if from.Format("2006-01-02") != "2026-07-01" || to.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("janela de 2026-07 = [%v, %v), esperado [2026-07-01, 2026-08-01)", from, to)
	}
	// `to` é EXCLUSIVO: 31/07 23:59:59 entra, 01/08 00:00:00 não.
	ultimoInstante := time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC)
	if !ultimoInstante.Before(to) {
		t.Error("último instante de julho ficou fora da janela")
	}
	if _, _, err := Period("julho de 2026").Range(); err == nil {
		t.Error("período mal formatado foi aceito")
	}
}

func TestPeriodOf(t *testing.T) {
	if got := PeriodOf(time.Date(2026, 7, 18, 23, 0, 0, 0, time.UTC)); got != "2026-07" {
		t.Fatalf("PeriodOf = %q", got)
	}
}
