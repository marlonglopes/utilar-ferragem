package ledger

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Liquidação externa (maquininha da loja) — testes do lançamento contábil.
//
// Rodam sem banco: são a garantia que sobrevive a qualquer ambiente. O que
// depende de Postgres (UNIQUE de idempotência, trigger de balanço) está em
// ledger_db_test.go e pula sem infra — e teste que pula não é garantia.
// ============================================================================

func externalInput() ExternalSaleInput {
	return ExternalSaleInput{
		OrderID:       "3f8b1d2e-0000-4000-8000-000000000001",
		NSU:           "004417",
		StoreID:       "loja-centro",
		OperatorID:    "op-a",
		Brand:         "visa",
		Authorization: "A1B2C3",
		OccurredAt:    time.Date(2026, 7, 18, 14, 32, 0, 0, time.UTC),
		GrossCents:    18990,
		RequestID:     "req-1",
		SettledBy:     "op-a",
	}
}

// A invariante que não se negocia: partida dobrada soma zero.
func TestExternalSaleFechaEmZero(t *testing.T) {
	tx := ExternalSale(externalInput())
	if err := tx.Validate(); err != nil {
		t.Fatalf("lançamento de liquidação externa não fecha: %v", err)
	}
	if tx.Total() != 18990 {
		t.Errorf("total = %d centavos, quero 18990", tx.Total())
	}
}

// Valores quebrados também têm que fechar — arredondamento é onde a partida
// dobrada costuma vazar centavo.
func TestExternalSaleFechaEmZeroParaQualquerValor(t *testing.T) {
	for _, cents := range []Cents{1, 33, 999, 100001, 987654321} {
		in := externalInput()
		in.GrossCents = cents
		tx := ExternalSale(in)
		if err := tx.Validate(); err != nil {
			t.Errorf("valor %d centavos não fecha: %v", cents, err)
		}
		var d, cr Cents
		for _, p := range tx.Postings {
			if p.Side == Debit {
				d += p.Amount
			} else {
				cr += p.Amount
			}
		}
		if d != cr {
			t.Errorf("valor %d: débitos=%d créditos=%d", cents, d, cr)
		}
	}
}

// O ponto central da feature: a liquidação externa NÃO pode encostar na 1.1.1.
// Se encostar, a conciliação com a Appmax passa a acusar divergência para
// sempre — exatamente o bug que esta feature existe pra corrigir.
func TestExternalSaleNaoTocaNoCaixaDoPSP(t *testing.T) {
	tx := ExternalSale(externalInput())

	var temCaixaExterno, temReceita bool
	for _, p := range tx.Postings {
		if p.Account == AcctCaixaPSP {
			t.Fatalf("liquidação externa lançou na 1.1.1 (caixa do PSP): "+
				"esse dinheiro nunca passou pela Appmax e vai virar divergência eterna na conciliação (%+v)", p)
		}
		if p.Account == AcctTaxaPSP {
			t.Errorf("liquidação externa não tem taxa de PSP: o MDR é do adquirente próprio e é desconhecido aqui")
		}
		switch p.Account {
		case AcctCaixaAdquirenteExterno:
			temCaixaExterno = true
			if p.Side != Debit {
				t.Errorf("caixa do adquirente externo é conta de ativo: entra a débito, veio %q", p.Side)
			}
		case AcctReceitaBruta:
			temReceita = true
			if p.Side != Credit {
				t.Errorf("receita bruta é credora, veio %q", p.Side)
			}
		}
	}
	if !temCaixaExterno {
		t.Error("faltou a partida de caixa no adquirente externo (1.1.3)")
	}
	if !temReceita {
		t.Error("faltou a partida de receita bruta (3.1.1)")
	}
}

// O rótulo de método é o que faz o relatório por método de pagamento parar de
// mentir. Se voltar a ser "card", a venda da maquininha some dentro do cartão
// online e a conciliação contábil quebra de novo.
func TestExternalSaleMarcaMetodoExternal(t *testing.T) {
	tx := ExternalSale(externalInput())
	for _, p := range tx.Postings {
		if p.PaymentMethod != MethodExternal {
			t.Errorf("partida em %s com método %q, esperado %q — "+
				"gravar 'card' aqui é o bug de conciliação original",
				p.Account, p.PaymentMethod, MethodExternal)
		}
	}
	if tx.Kind != KindExternalSale {
		t.Errorf("kind = %q, esperado %q", tx.Kind, KindExternalSale)
	}
}

// O NSU é o único campo que amarra a venda ao extrato do adquirente. Sem ele no
// livro, o financeiro não consegue casar a linha do extrato com o pedido.
func TestExternalSaleCarregaNSUeRastro(t *testing.T) {
	in := externalInput()
	tx := ExternalSale(in)

	if !strings.Contains(tx.Description, in.NSU) {
		t.Errorf("descrição sem NSU: %q", tx.Description)
	}
	if !strings.Contains(tx.Description, in.StoreID) {
		t.Errorf("descrição sem a loja: %q", tx.Description)
	}
	if tx.CreatedBy != in.SettledBy {
		t.Errorf("createdBy = %q, esperado quem liquidou (%q) — "+
			"lançamento de liquidação externa sem pessoa é o caminho natural de fraude interna",
			tx.CreatedBy, in.SettledBy)
	}
	if !strings.Contains(tx.Postings[0].Memo, in.NSU) {
		t.Errorf("memo da partida de caixa sem NSU: %q", tx.Postings[0].Memo)
	}
}

// A chave de idempotência é o pedido: liquidar duas vezes o mesmo pedido tem
// que produzir a MESMA chave, para o UNIQUE do banco recusar a segunda.
func TestExternalSaleIdempotentePorPedido(t *testing.T) {
	a := ExternalSale(externalInput())

	segunda := externalInput()
	// NSU diferente, hora diferente, outro operador — mesmo pedido.
	segunda.NSU = "009999"
	segunda.OccurredAt = segunda.OccurredAt.Add(time.Hour)
	segunda.SettledBy = "mgr-a"
	b := ExternalSale(segunda)

	if a.Kind != b.Kind || a.SourceType != b.SourceType || a.SourceID != b.SourceID {
		t.Fatalf("chave de idempotência mudou entre duas liquidações do mesmo pedido: %s/%s/%s vs %s/%s/%s",
			a.Kind, a.SourceType, a.SourceID, b.Kind, b.SourceType, b.SourceID)
	}
	if a.SourceID != externalInput().OrderID {
		t.Errorf("source_id = %q, esperado o id do pedido", a.SourceID)
	}
}

// A origem NÃO pode ser `payment`: não existe linha em `payments` para uma
// liquidação externa, e usar essa origem faria a conciliação procurar no PSP um
// pagamento que nunca existiu.
func TestExternalSaleNaoUsaOrigemDePagamento(t *testing.T) {
	tx := ExternalSale(externalInput())
	if tx.SourceType == SourcePayment {
		t.Error("liquidação externa não tem pagamento: source_type=payment faria a conciliação caçar um id inexistente no PSP")
	}
	if tx.SourceType != SourceExternalSettlement {
		t.Errorf("source_type = %q, esperado %q", tx.SourceType, SourceExternalSettlement)
	}
}

// Estorno de uma liquidação externa continua fechando em zero (o Reverse é
// genérico, mas o caso precisa estar coberto: é dinheiro de verdade).
func TestExternalSaleEstornoInverteAsMesmasContas(t *testing.T) {
	tx := ExternalSale(externalInput())
	invertidas := make([]Posting, 0, len(tx.Postings))
	for _, p := range tx.Postings {
		side := Credit
		if p.Side == Credit {
			side = Debit
		}
		invertidas = append(invertidas, Posting{
			Account: p.Account, Side: side, Amount: p.Amount, PaymentMethod: p.PaymentMethod,
		})
	}
	rev := TxInput{
		Kind: KindReversal, SourceType: SourceLedgerTx, SourceID: "tx-1",
		Postings: invertidas,
	}
	if err := rev.Validate(); err != nil {
		t.Fatalf("estorno da liquidação externa não fecha: %v", err)
	}
}

// A conta nova tem que existir no plano — Validate() recusa conta desconhecida,
// e o INSERT tem FK para ledger_accounts. Se alguém adicionar a constante em Go
// e esquecer a migration 006, este teste não pega o banco, mas pega o Go.
func TestContaDoAdquirenteExternoEstaNoPlano(t *testing.T) {
	meta, ok := MetaOf(AcctCaixaAdquirenteExterno)
	if !ok {
		t.Fatal("1.1.3 não está no ChartOfAccounts — Validate() recusaria todo lançamento de liquidação externa")
	}
	if meta.Type != "asset" || meta.NormalSide != Debit {
		t.Errorf("1.1.3 deveria ser ativo devedor, veio %s/%s", meta.Type, meta.NormalSide)
	}
}
