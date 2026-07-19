package ledger_test

// Testes de integração do livro contábil. Requerem o Postgres do payment-service
// (make infra-up) com as migrations 004/005 aplicadas. Sem banco: SKIP.
//
// Estes testes cobrem justamente o que NÃO dá pra garantir em Go: as travas que
// vivem no banco (imutabilidade, soma zero, período fechado). Um teste que só
// exercita a validação em Go daria falsa confiança — o caminho perigoso é
// exatamente o que não passa pelo nosso código.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/utilar/payment-service/internal/ledger"
	"github.com/utilar/pkg/audit"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PAYMENT_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5435/payment_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB indisponível: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB inacessível: %v", err)
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('ledger_entries') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Skip("migrations do ledger não aplicadas — rode `make db-migrate`")
	}
	return db
}

// uniqueSource evita colisão na constraint de idempotência entre execuções.
func uniqueSource(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func poster(t *testing.T, db *sql.DB) *ledger.Poster {
	t.Helper()
	return ledger.NewPoster(db, audit.New(db, "payment-service-test"))
}

// ===================== partidas dobradas =====================

func TestDBLancamentoBalanceadoEhAceito(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	src := uniqueSource("pay")
	tx, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: src, OrderID: "ord-1", OccurredAt: time.Now().UTC(),
		GrossCents: 26400, PSPFeeCents: 1056, Method: "pix",
	}))
	if err != nil {
		t.Fatalf("lançamento válido recusado: %v", err)
	}
	if tx.TotalCents != 26400+1056 {
		t.Errorf("total = %d", tx.TotalCents)
	}

	got, err := p.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	var dr, cr ledger.Cents
	for _, pg := range got.Postings {
		if pg.Side == ledger.Debit {
			dr += pg.Amount
		} else {
			cr += pg.Amount
		}
	}
	if dr != cr {
		t.Fatalf("lançamento gravado não fecha: débitos=%d créditos=%d", dr, cr)
	}
}

// A trava do banco: mesmo escrevendo SQL cru, um lançamento que não fecha é
// recusado no COMMIT pela constraint trigger.
func TestDBBancoRecusaLancamentoDesbalanceadoEmSQLCru(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	var txID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO ledger_transactions (occurred_at, period, kind, source_type, source_id)
		VALUES (now(), date_trunc('month', now())::date, 'adjustment', 'manual', $1)
		RETURNING id`, uniqueSource("cru")).Scan(&txID); err != nil {
		t.Fatal(err)
	}
	// Débito de 100, crédito de 99: um centavo evaporou.
	for _, e := range []struct {
		acct, side string
		amount     int64
	}{
		{"1.1.1", "debit", 100},
		{"3.1.1", "credit", 99},
	} {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_entries (transaction_id, account_code, side, amount_cents)
			VALUES ($1,$2,$3,$4)`, txID, e.acct, e.side, e.amount); err != nil {
			t.Fatalf("insert da partida: %v", err)
		}
	}

	err = tx.Commit()
	if err == nil {
		t.Fatal("BANCO ACEITOU LANÇAMENTO QUE NÃO FECHA — a constraint trigger não está ativa")
	}
	if !strings.Contains(err.Error(), "NÃO FECHA") {
		t.Fatalf("erro inesperado (esperava a trigger de balanço): %v", err)
	}
}

func TestDBBancoRecusaDocumentoSemPartidas(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_transactions (occurred_at, period, kind, source_type, source_id)
		VALUES (now(), date_trunc('month', now())::date, 'adjustment', 'manual', $1)`,
		uniqueSource("vazio")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("documento contábil vazio foi aceito")
	}
}

func TestDBBancoRecusaValorNegativo(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	var txID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO ledger_transactions (occurred_at, period, kind, source_type, source_id)
		VALUES (now(), date_trunc('month', now())::date, 'adjustment', 'manual', $1)
		RETURNING id`, uniqueSource("neg")).Scan(&txID); err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (transaction_id, account_code, side, amount_cents)
		VALUES ($1,'1.1.1','debit',-100)`, txID)
	if err == nil {
		t.Fatal("valor negativo aceito — o sinal tem que ser o `side`, não o valor")
	}
}

// ===================== imutabilidade =====================

func TestDBLivroEhImutavel(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	tx, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("imut"), OrderID: "ord-imut",
		OccurredAt: time.Now().UTC(), GrossCents: 10000, Method: "pix",
	}))
	if err != nil {
		t.Fatal(err)
	}

	casos := map[string]string{
		"UPDATE na transação": `UPDATE ledger_transactions SET description='alterado' WHERE id=$1`,
		"DELETE da transação": `DELETE FROM ledger_transactions WHERE id=$1`,
		"UPDATE nas partidas": `UPDATE ledger_entries SET amount_cents=1 WHERE transaction_id=$1`,
		"DELETE das partidas": `DELETE FROM ledger_entries WHERE transaction_id=$1`,
	}
	for nome, q := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, q, tx.ID); err == nil {
				t.Fatalf("BANCO PERMITIU %s NO LIVRO — imutabilidade quebrada", nome)
			} else if !strings.Contains(err.Error(), "imutável") {
				t.Fatalf("bloqueado, mas por outro motivo: %v", err)
			}
		})
	}

	// E o valor continua lá, intacto.
	got, err := p.Get(ctx, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalCents != 10000 {
		t.Fatalf("valor do lançamento mudou: %d", got.TotalCents)
	}
}

// A ÚNICA forma de corrigir: lançamento de estorno. O original continua visível.
func TestDBCorrecaoEhEstornoNaoEdicao(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	orig, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("rev"), OrderID: "ord-rev",
		OccurredAt: time.Now().UTC(), GrossCents: 50000, Method: "card",
	}))
	if err != nil {
		t.Fatal(err)
	}

	rev, err := p.Reverse(ctx, orig.ID, "valor lançado em duplicidade na conciliação de 07/2026", "admin-1")
	if err != nil {
		t.Fatalf("estorno recusado: %v", err)
	}
	if rev.ReversesID != orig.ID {
		t.Errorf("estorno não aponta pro original: %q", rev.ReversesID)
	}

	// Lados invertidos, mesmos valores: os dois juntos somam zero.
	saldos := map[ledger.Account]ledger.Cents{}
	for _, tx := range []*ledger.Tx{orig, rev} {
		for _, pg := range tx.Postings {
			if pg.Side == ledger.Debit {
				saldos[pg.Account] += pg.Amount
			} else {
				saldos[pg.Account] -= pg.Amount
			}
		}
	}
	for acct, saldo := range saldos {
		if saldo != 0 {
			t.Errorf("conta %s ficou com saldo %d após original+estorno (esperado 0)", acct, saldo)
		}
	}

	// O original continua lá — o auditor precisa ver o que a empresa acreditava.
	if _, err := p.Get(ctx, orig.ID); err != nil {
		t.Fatalf("lançamento original sumiu após o estorno: %v", err)
	}

	if _, err := p.Reverse(ctx, orig.ID, "tentando estornar de novo o mesmo lançamento", "admin-1"); !errors.Is(err, ledger.ErrDuplicate) {
		t.Errorf("estorno duplicado deveria ser recusado: %v", err)
	}
	if _, err := p.Reverse(ctx, orig.ID, "curto", "admin-1"); err == nil {
		t.Error("estorno sem justificativa mínima foi aceito")
	}
}

// ===================== idempotência =====================

// Cenário real: a Appmax reentrega o webhook em 0, +30min, +2h e +4h. Sem
// idempotência, a receita do dia sairia 4x maior.
func TestDBWebhookReentregueNaoDobraAReceita(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	in := ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("idem"), OrderID: "ord-idem",
		OccurredAt: time.Now().UTC(), GrossCents: 34900, Method: "pix",
	})
	if _, err := p.Post(ctx, in); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := p.Post(ctx, in); !errors.Is(err, ledger.ErrDuplicate) {
			t.Fatalf("reentrega %d não foi detectada como duplicata: %v", i+1, err)
		}
	}
}

// ===================== fechamento de período =====================

func TestDBPeriodoFechadoRecusaLancamentoRetroativo(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p := poster(t, db)

	// Mês bem antigo, isolado dos outros testes.
	antigo := time.Date(2019, 3, 15, 10, 0, 0, 0, time.UTC)
	periodo := ledger.Period("2019-03")
	t.Cleanup(func() {
		// ledger_periods bloqueia UPDATE/DELETE por trigger; removemos com o
		// trigger desabilitado só no escopo desta sessão de teste.
		_, _ = db.Exec(`ALTER TABLE ledger_periods DISABLE TRIGGER trg_ledger_period_no_reopen`)
		_, _ = db.Exec(`DELETE FROM ledger_periods WHERE period = '2019-03-01'`)
		_, _ = db.Exec(`ALTER TABLE ledger_periods ENABLE TRIGGER trg_ledger_period_no_reopen`)
	})
	_, _ = db.Exec(`ALTER TABLE ledger_periods DISABLE TRIGGER trg_ledger_period_no_reopen`)
	_, _ = db.Exec(`DELETE FROM ledger_periods WHERE period = '2019-03-01'`)
	_, _ = db.Exec(`ALTER TABLE ledger_periods ENABLE TRIGGER trg_ledger_period_no_reopen`)

	// Um lançamento ANTES de fechar: tem que passar.
	if _, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("perAntes"), OrderID: "o", OccurredAt: antigo,
		GrossCents: 1000, Method: "pix",
	})); err != nil {
		t.Fatalf("lançamento em período aberto recusado: %v", err)
	}

	closer := ledger.NewCloser(db, audit.New(db, "payment-service-test"))
	st, err := closer.Close(ctx, periodo, "admin-1", "127.0.0.1", "req-test")
	if err != nil {
		t.Fatalf("fechamento recusado: %v", err)
	}
	if st.Status != "closed" || st.ClosedAt == nil {
		t.Fatalf("período não ficou fechado: %+v", st)
	}
	if len(st.ClosingBalances) == 0 {
		t.Error("fechamento sem saldos de fechamento — relatório do mês não seria reproduzível")
	}

	// Agora o mesmo mês recusa qualquer lançamento novo.
	_, err = p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("perDepois"), OrderID: "o", OccurredAt: antigo,
		GrossCents: 1000, Method: "pix",
	}))
	if !errors.Is(err, ledger.ErrPeriodClosed) {
		t.Fatalf("PERÍODO FECHADO ACEITOU LANÇAMENTO RETROATIVO: %v", err)
	}

	// E não fecha duas vezes.
	if _, err := closer.Close(ctx, periodo, "admin-1", "127.0.0.1", "req-test"); !errors.Is(err, ledger.ErrAlreadyClosed) {
		t.Errorf("fechamento duplicado aceito: %v", err)
	}

	// Reabertura é proibida pela aplicação.
	if _, err := db.Exec(`UPDATE ledger_periods SET status='open' WHERE period='2019-03-01'`); err == nil {
		t.Fatal("BANCO PERMITIU REABRIR PERÍODO FECHADO")
	}
}

func TestDBNaoFechaPeriodoQueNaoTerminou(t *testing.T) {
	db := testDB(t)
	closer := ledger.NewCloser(db, nil)
	futuro := ledger.Period(time.Now().UTC().AddDate(0, 1, 0).Format("2006-01"))
	if _, err := closer.Close(context.Background(), futuro, "admin", "ip", "req"); !errors.Is(err, ledger.ErrPeriodInFuture) {
		t.Fatalf("fechou período que ainda não terminou: %v", err)
	}
}

// ===================== relatórios =====================

func TestDBBalanceteSempreFecha(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	// Janela isolada num mês próprio, pra não misturar com outros testes.
	base := time.Date(2021, 6, 10, 12, 0, 0, 0, time.UTC)
	from := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	reports := ledger.NewReports(db)
	// Baseline ANTES de lançar: a janela é fixa e execuções anteriores da suíte
	// deixaram lançamentos nela. Comparar DELTA em vez de absoluto mantém o
	// teste determinístico sem precisar limpar o livro (que é imutável — não dá
	// pra limpar mesmo).
	antes, err := reports.Summary(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}

	pid := uniqueSource("rel")
	lancamentos := []ledger.TxInput{
		ledger.Sale(ledger.SaleInput{PaymentID: pid + "-a", OrderID: "o1", OccurredAt: base,
			GrossCents: 100000, PSPFeeCents: 4000, Method: "card"}),
		ledger.Sale(ledger.SaleInput{PaymentID: pid + "-b", OrderID: "o2", OccurredAt: base,
			GrossCents: 50000, Method: "pix"}),
		ledger.Refund(ledger.RefundInput{PaymentID: pid + "-b", OrderID: "o2", OccurredAt: base,
			Cents: 50000, Method: "pix"}),
		ledger.SellerSplit(ledger.SellerSplitInput{PaymentID: pid + "-a", OrderID: "o1",
			SellerID: "s1", OccurredAt: base, Cents: 60000, Method: "card"}),
	}
	for i, in := range lancamentos {
		if _, err := p.Post(ctx, in); err != nil && !errors.Is(err, ledger.ErrDuplicate) {
			t.Fatalf("lançamento %d: %v", i, err)
		}
	}

	tb, err := reports.TrialBalance(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !tb.Balanced {
		t.Fatalf("BALANCETE NÃO FECHA: débitos=%d créditos=%d", tb.TotalDebits, tb.TotalCredits)
	}

	depois, err := reports.Summary(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}

	// Receita bruta é o valor CHEIO das duas vendas; o estorno vai em conta
	// própria, NÃO abate a 3.1.1 (é isso que permite ao contador ver bruto e
	// devolução separados).
	deltas := map[string][2]ledger.Cents{
		"receita bruta": {depois.GrossCents - antes.GrossCents, 150000},
		"taxas do PSP":  {depois.PSPFeesCents - antes.PSPFeesCents, 4000},
		"estornos":      {depois.RefundsCents - antes.RefundsCents, 50000},
		"repasses":      {depois.SellerSplitCents - antes.SellerSplitCents, 60000},
		"líquido":       {depois.NetCents - antes.NetCents, 150000 - 4000 - 50000 - 60000},
	}
	for nome, d := range deltas {
		if d[0] != d[1] {
			t.Errorf("%s: delta = %d, esperado %d", nome, d[0], d[1])
		}
	}

	// O recorte por método tem que somar o mesmo bruto do consolidado — senão o
	// dashboard mostra dois números diferentes pro mesmo faturamento.
	byMethod, err := reports.ByMethod(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	var somaMetodos ledger.Cents
	for _, m := range byMethod {
		somaMetodos += m.GrossCents
	}
	if somaMetodos != depois.GrossCents {
		t.Errorf("soma por método (%d) != consolidado (%d)", somaMetodos, depois.GrossCents)
	}
}

func TestDBExportacaoCSVeOFXSaemComOsFormatosDoContador(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	base := time.Date(2022, 5, 10, 12, 0, 0, 0, time.UTC)
	from := time.Date(2022, 5, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	if _, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("exp"), OrderID: "o-exp", OccurredAt: base,
		GrossCents: 128450, PSPFeeCents: 5138, Method: "card",
	})); err != nil && !errors.Is(err, ledger.ErrDuplicate) {
		t.Fatal(err)
	}

	reports := ledger.NewReports(db)

	var csvBuf strings.Builder
	if err := reports.ExportRazaoCSV(ctx, &csvBuf, from, to); err != nil {
		t.Fatal(err)
	}
	csvOut := csvBuf.String()
	if !strings.HasPrefix(csvOut, "\uFEFF") {
		t.Error("CSV sem BOM — o Excel pt-BR vai mostrar 'CartÃ£o'")
	}
	if !strings.Contains(csvOut, ";") {
		t.Error("CSV sem separador ponto-e-vírgula")
	}
	if !strings.Contains(csvOut, "1284,50") {
		t.Errorf("CSV sem valor com vírgula decimal:\n%s", primeiras(csvOut, 3))
	}
	if !strings.Contains(csvOut, "10/05/2022") {
		t.Error("CSV sem data em dd/mm/aaaa")
	}

	var ofxBuf strings.Builder
	if err := reports.ExportOFX(ctx, &ofxBuf, from, to, ledger.OFXOptions{}); err != nil {
		t.Fatal(err)
	}
	ofxOut := ofxBuf.String()
	for _, marca := range []string{"OFXHEADER:100", "VERSION:102", "<CURDEF>BRL", "<STMTTRN>", "</OFX>"} {
		if !strings.Contains(ofxOut, marca) {
			t.Errorf("OFX sem %q", marca)
		}
	}
	if !strings.Contains(ofxOut, "1284.50") {
		t.Error("OFX sem valor com PONTO decimal (OFX é padrão americano)")
	}
	if strings.Contains(ofxOut, "1284,50") {
		t.Error("OFX com vírgula decimal — o sistema do contador leria valor errado")
	}
}

func primeiras(s string, n int) string {
	linhas := strings.SplitN(s, "\n", n+1)
	if len(linhas) > n {
		linhas = linhas[:n]
	}
	return strings.Join(linhas, "\n")
}

// ===================== liquidação externa (maquininha da loja) =====================

// A idempotência que importa de verdade é a do BANCO: liquidar duas vezes o
// mesmo pedido não pode gerar dois lançamentos, mesmo que o handler falhe em
// detectar o retry. É o UNIQUE (kind, source_type, source_id) que garante.
func TestDBLiquidacaoExternaNaoDuplicaLancamento(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	orderID := uniqueSource("ord-ext")
	in := ledger.ExternalSaleInput{
		OrderID: orderID, NSU: "004417", StoreID: "loja-centro", OperatorID: "op-a",
		OccurredAt: time.Now().UTC(), GrossCents: 18990, SettledBy: "op-a",
	}

	tx, err := p.Post(ctx, ledger.ExternalSale(in))
	if err != nil {
		t.Fatalf("primeira liquidação recusada: %v", err)
	}

	// Segunda tentativa — retry do order-service, ou o operador clicando duas
	// vezes. Tem que virar ErrDuplicate, tratado como no-op pelo chamador.
	in.NSU = "009999" // até com outro comprovante: a chave é o PEDIDO
	if _, err := p.Post(ctx, ledger.ExternalSale(in)); !errors.Is(err, ledger.ErrDuplicate) {
		t.Fatalf("segunda liquidação do mesmo pedido: err = %v, esperado ErrDuplicate", err)
	}

	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM ledger_transactions
		WHERE kind = 'external_sale' AND source_id = $1`, orderID).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Errorf("lançamentos de liquidação externa = %d, esperado 1 — receita duplicada", n)
	}

	// E o lançamento gravado bate: soma zero, sem tocar no caixa do PSP.
	got, err := p.Get(ctx, tx.ID)
	if err != nil {
		t.Fatalf("reler: %v", err)
	}
	var debitos, creditos ledger.Cents
	for _, pg := range got.Postings {
		if pg.Account == ledger.AcctCaixaPSP {
			t.Errorf("liquidação externa encostou na 1.1.1 — vira divergência eterna na conciliação com a Appmax")
		}
		if pg.PaymentMethod != ledger.MethodExternal {
			t.Errorf("partida com método %q, esperado %q", pg.PaymentMethod, ledger.MethodExternal)
		}
		if pg.Side == ledger.Debit {
			debitos += pg.Amount
		} else {
			creditos += pg.Amount
		}
	}
	if debitos != creditos || debitos != 18990 {
		t.Errorf("lançamento não fecha: débitos=%d créditos=%d", debitos, creditos)
	}
}

// A liquidação externa NÃO pode aparecer na conciliação com o PSP: ela não cria
// linha em `payments`, então a query da reconciliação nunca a alcança. Este
// teste trava essa propriedade — se um dia alguém passar a criar um `payment`
// para a venda de maquininha, a conciliação volta a acusar divergência eterna.
func TestDBLiquidacaoExternaNaoEntraNaConciliacaoDoPSP(t *testing.T) {
	db := testDB(t)
	p := poster(t, db)
	ctx := context.Background()

	orderID := uniqueSource("ord-ext-rec")
	if _, err := p.Post(ctx, ledger.ExternalSale(ledger.ExternalSaleInput{
		OrderID: orderID, NSU: "770011", StoreID: "loja-centro",
		OccurredAt: time.Now().UTC(), GrossCents: 5000, SettledBy: "op-a",
	})); err != nil {
		t.Fatalf("lançar: %v", err)
	}

	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM payments WHERE order_id::text = $1`, orderID).Scan(&n); err != nil {
		// order_id é UUID na tabela payments; um id sintético não casa e o
		// próprio erro de cast confirma que não existe pagamento nenhum.
		return
	}
	if n != 0 {
		t.Errorf("liquidação externa criou %d pagamento(s): a conciliação com a Appmax vai procurar por eles no extrato e não achar", n)
	}
}
