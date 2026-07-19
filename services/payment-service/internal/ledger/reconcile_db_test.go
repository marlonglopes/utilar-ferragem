package ledger_test

// Testes de integração da reconciliação e da trilha de auditoria no banco.
// Sem Postgres: SKIP.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/utilar/payment-service/internal/ledger"
	"github.com/utilar/payment-service/internal/psp"
	"github.com/utilar/pkg/audit"
)

// fakeGateway é o PSP controlado pelo teste: o valor e o status que ele
// "confirma" são o que o teste quiser.
type fakeGateway struct {
	amounts  map[string]float64
	statuses map[string]psp.PaymentStatus
	notFound map[string]bool
}

func (f *fakeGateway) Name() string { return "fake" }
func (f *fakeGateway) CreatePayment(context.Context, psp.CreateRequest) (*psp.CreateResult, error) {
	return nil, errors.New("não usado")
}
func (f *fakeGateway) GetPayment(_ context.Context, id string) (*psp.GetResult, error) {
	if f.notFound[id] {
		return nil, psp.ErrNotFound
	}
	st, ok := f.statuses[id]
	if !ok {
		st = psp.StatusApproved
	}
	return &psp.GetResult{PSPID: id, Amount: f.amounts[id], Status: st, Currency: "BRL"}, nil
}
func (f *fakeGateway) VerifyWebhook([]byte, http.Header) error { return nil }
func (f *fakeGateway) ParseWebhookEvent([]byte) (*psp.WebhookEvent, error) {
	return nil, errors.New("não usado")
}

// seedPayment cria um pagamento local. Devolve o id.
func seedPayment(t *testing.T, db *sql.DB, pspID, status string, amount float64, createdAt time.Time) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id, created_at)
		VALUES (gen_random_uuid(), gen_random_uuid(), 'pix', $1, $2, $3, $4)
		RETURNING id`, status, amount, pspID, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("seed do pagamento: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM payments WHERE id=$1`, id) })
	return id
}

// A CAPACIDADE CENTRAL: divergência de valor é DETECTADA e REPORTADA, nunca
// corrigida sozinha.
func TestDBReconciliacaoReportaDivergenciaDeValorENaoCorrige(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	base := time.Date(2023, 4, 10, 12, 0, 0, 0, time.UTC)
	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)

	pspID := uniqueSource("psp-diverg")
	// Nós temos R$ 100,00. O PSP diz que cobrou R$ 1,00.
	paymentID := seedPayment(t, db, pspID, "confirmed", 100.00, base)

	gw := &fakeGateway{
		amounts:  map[string]float64{pspID: 1.00},
		statuses: map[string]psp.PaymentStatus{pspID: psp.StatusApproved},
	}
	rec := ledger.NewReconciler(db, gw, nil)
	t.Cleanup(func() { cleanRuns(db, from, to) })

	res, err := rec.Run(ctx, from, to, "req-recon")
	if err != nil {
		t.Fatalf("a rotina precisa RODAR mesmo achando divergência: %v", err)
	}
	if res.Status != "discrepancies" {
		t.Fatalf("status = %q, esperado discrepancies", res.Status)
	}

	var achou *ledger.Discrepancy
	for i := range res.Discrepancies {
		if res.Discrepancies[i].Kind == ledger.DiscAmount &&
			res.Discrepancies[i].PSPPaymentID == pspID {
			achou = &res.Discrepancies[i]
		}
	}
	if achou == nil {
		t.Fatalf("divergência de VALOR não detectada: %+v", res.Discrepancies)
	}
	if achou.Severity != "critical" {
		t.Errorf("divergência de dinheiro tem que ser critical, veio %q", achou.Severity)
	}
	// R$ 1,00 - R$ 100,00 = -9900 centavos. Em INTEIROS.
	if achou.DeltaCents != -9900 {
		t.Errorf("delta = %d centavos, esperado -9900", achou.DeltaCents)
	}
	// O erro não pode carregar o corpo cru do PSP (PII do comprador).
	if strings.Contains(achou.Detail, "@") {
		t.Errorf("detalhe da divergência parece conter dado do PSP: %q", achou.Detail)
	}

	// O PONTO PRINCIPAL: nada foi corrigido. O valor local continua o mesmo.
	var amount float64
	var status string
	if err := db.QueryRow(`SELECT amount, status FROM payments WHERE id=$1`, paymentID).
		Scan(&amount, &status); err != nil {
		t.Fatal(err)
	}
	if amount != 100.00 {
		t.Fatalf("A RECONCILIAÇÃO ALTEROU O VALOR LOCAL (%v) — ela NUNCA pode corrigir sozinha", amount)
	}
	if status != "confirmed" {
		t.Fatalf("a reconciliação alterou o status local (%q)", status)
	}

	// A divergência fica na fila aberta até um humano resolver.
	abertas, err := rec.OpenDiscrepancies(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	var naFila bool
	for _, d := range abertas {
		if d.ID == achou.ID {
			naFila = true
		}
	}
	if !naFila {
		t.Error("divergência não entrou na fila de trabalho do financeiro")
	}

	// Resolver exige nota — é decisão humana com consequência financeira.
	if err := rec.Resolve(ctx, achou.ID, "admin-1", ""); err == nil {
		t.Error("resolução sem justificativa foi aceita")
	}
	if err := rec.Resolve(ctx, achou.ID, "admin-1", "conferido com o extrato: cobrança parcial autorizada pelo cliente"); err != nil {
		t.Fatalf("resolução válida recusada: %v", err)
	}
	if err := rec.Resolve(ctx, achou.ID, "admin-1", "tentando resolver de novo a mesma divergência"); err == nil {
		t.Error("divergência resolvida duas vezes")
	}
}

func TestDBReconciliacaoDetectaPagamentoInexistenteNoPSP(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	base := time.Date(2023, 5, 10, 12, 0, 0, 0, time.UTC)
	from, to := base.Add(-time.Hour), base.Add(time.Hour)

	pspID := uniqueSource("psp-fantasma")
	seedPayment(t, db, pspID, "confirmed", 50.00, base)

	gw := &fakeGateway{notFound: map[string]bool{pspID: true}}
	rec := ledger.NewReconciler(db, gw, nil)
	t.Cleanup(func() { cleanRuns(db, from, to) })

	res, err := rec.Run(ctx, from, to, "req")
	if err != nil {
		t.Fatal(err)
	}
	var achou bool
	for _, d := range res.Discrepancies {
		if d.Kind == ledger.DiscMissingAtPSP && d.Severity == "critical" {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("psp_payment_id que o PSP não conhece não foi sinalizado: %+v", res.Discrepancies)
	}
}

// Pagamento confirmado sem lançamento no livro: é o que faz o faturamento do
// dashboard não bater com o extrato.
func TestDBReconciliacaoDetectaVendaConfirmadaSemLancamento(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	base := time.Date(2023, 6, 10, 12, 0, 0, 0, time.UTC)
	from, to := base.Add(-time.Hour), base.Add(time.Hour)

	pspID := uniqueSource("psp-semledger")
	seedPayment(t, db, pspID, "confirmed", 75.00, base)

	gw := &fakeGateway{amounts: map[string]float64{pspID: 75.00}}
	rec := ledger.NewReconciler(db, gw, nil)
	t.Cleanup(func() { cleanRuns(db, from, to) })

	res, err := rec.Run(ctx, from, to, "req")
	if err != nil {
		t.Fatal(err)
	}
	var achou bool
	for _, d := range res.Discrepancies {
		if d.Kind == ledger.DiscLedgerMissing {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("venda confirmada sem lançamento não foi detectada: %+v", res.Discrepancies)
	}
}

func TestDBReconciliacaoSemDivergenciaSaiLimpa(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	base := time.Date(2023, 7, 10, 12, 0, 0, 0, time.UTC)
	from, to := base.Add(-time.Hour), base.Add(time.Hour)

	pspID := uniqueSource("psp-ok")
	paymentID := seedPayment(t, db, pspID, "confirmed", 34.90, base)

	// Lança a venda no livro pra não cair no ledger_missing.
	p := poster(t, db)
	if _, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: paymentID, OrderID: "o", OccurredAt: base, GrossCents: 3490, Method: "pix",
	})); err != nil {
		t.Fatal(err)
	}

	gw := &fakeGateway{amounts: map[string]float64{pspID: 34.90}}
	rec := ledger.NewReconciler(db, gw, nil)
	t.Cleanup(func() { cleanRuns(db, from, to) })

	res, err := rec.Run(ctx, from, to, "req")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "ok" || len(res.Discrepancies) > 0 {
		t.Fatalf("falso positivo: %+v", res.Discrepancies)
	}
	if res.Checked != 1 {
		t.Errorf("verificou %d pagamentos, esperado 1", res.Checked)
	}
}

// 19.99 em float64 é 19.989999999999998. A comparação tem que ser em centavos
// inteiros — com tolerância de float, uma diferença real de 1 centavo passaria
// batido em toda transação.
func TestDBReconciliacaoComparaEmCentavosNaoEmFloat(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	base := time.Date(2023, 8, 10, 12, 0, 0, 0, time.UTC)
	from, to := base.Add(-time.Hour), base.Add(time.Hour)

	// Idênticos: 19.99 dos dois lados não pode gerar ruído por float.
	okID := uniqueSource("psp-1999")
	pid := seedPayment(t, db, okID, "confirmed", 19.99, base)
	p := poster(t, db)
	if _, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: pid, OrderID: "o", OccurredAt: base, GrossCents: 1999, Method: "pix",
	})); err != nil {
		t.Fatal(err)
	}

	// Diferença de UM CENTAVO: tem que ser pega.
	diffID := uniqueSource("psp-1998")
	pid2 := seedPayment(t, db, diffID, "confirmed", 19.99, base)
	if _, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: pid2, OrderID: "o", OccurredAt: base, GrossCents: 1999, Method: "pix",
	})); err != nil {
		t.Fatal(err)
	}

	gw := &fakeGateway{amounts: map[string]float64{okID: 19.99, diffID: 19.98}}
	rec := ledger.NewReconciler(db, gw, nil)
	t.Cleanup(func() { cleanRuns(db, from, to) })

	res, err := rec.Run(ctx, from, to, "req")
	if err != nil {
		t.Fatal(err)
	}
	var umCentavo bool
	for _, d := range res.Discrepancies {
		if d.PSPPaymentID == okID && d.Kind == ledger.DiscAmount {
			t.Error("19.99 vs 19.99 gerou divergência — ruído de float")
		}
		if d.PSPPaymentID == diffID && d.Kind == ledger.DiscAmount && d.DeltaCents == -1 {
			umCentavo = true
		}
	}
	if !umCentavo {
		t.Fatalf("diferença de 1 centavo NÃO foi detectada — é a sangria silenciosa: %+v", res.Discrepancies)
	}
}

func cleanRuns(db *sql.DB, from, to time.Time) {
	_, _ = db.Exec(`DELETE FROM reconciliation_runs WHERE window_from = $1 AND window_to = $2`, from, to)
}

// ===================== trilha de auditoria no banco =====================

func TestDBTrilhaEncadeiaEDetectaAdulteracao(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	rec := audit.New(db, "payment-service-test")

	entidade := uniqueSource("ent")
	for i := 0; i < 5; i++ {
		if _, err := rec.Record(ctx, audit.Entry{
			ActorID: "user-1", ActorRole: "admin", ActorIP: "203.0.113.9",
			EntityType: "teste_auditoria", EntityID: entidade,
			Action:    audit.ActionUpdate,
			OldValue:  map[string]any{"status": "pending"},
			NewValue:  map[string]any{"status": "confirmed", "iteracao": i},
			RequestID: "req-" + entidade,
		}); err != nil {
			t.Fatalf("gravação %d: %v", i, err)
		}
	}

	// A cadeia inteira tem que verificar.
	if err := rec.VerifyAll(ctx); err != nil {
		t.Fatalf("cadeia íntegra recusada: %v", err)
	}

	recs, err := rec.List(ctx, audit.ListFilter{EntityType: "teste_auditoria", EntityID: entidade})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 5 {
		t.Fatalf("esperava 5 registros, veio %d", len(recs))
	}
	// Cada elo aponta pro anterior.
	for i := 1; i < len(recs); i++ {
		if recs[i].PrevHash != recs[i-1].Hash {
			t.Fatalf("elo %d não aponta pro anterior", i)
		}
	}

	// Adulteração local (fora do banco) é detectada.
	recs[2].NewValue = json.RawMessage(`{"status":"confirmed","iteracao":999}`)
	if err := audit.VerifyChain(recs, recs[0].PrevHash); err == nil {
		t.Fatal("adulteração não detectada")
	}
}

// A trilha é APPEND-ONLY garantido pelo BANCO, não por convenção de código.
func TestDBTrilhaEhAppendOnlyNoBanco(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	rec := audit.New(db, "payment-service-test")

	r, err := rec.Record(ctx, audit.Entry{
		ActorID: "user-1", EntityType: "teste_append", EntityID: uniqueSource("ap"),
		Action: audit.ActionCreate, NewValue: map[string]any{"a": 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	casos := map[string]string{
		"UPDATE": `UPDATE audit_log SET action='delete' WHERE seq=$1`,
		"DELETE": `DELETE FROM audit_log WHERE seq=$1`,
	}
	for nome, q := range casos {
		if _, err := db.ExecContext(ctx, q, r.Seq); err == nil {
			t.Fatalf("BANCO PERMITIU %s NA TRILHA DE AUDITORIA", nome)
		} else if !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s bloqueado por outro motivo: %v", nome, err)
		}
	}

	// TRUNCATE também.
	if _, err := db.ExecContext(ctx, `TRUNCATE audit_log`); err == nil {
		t.Fatal("BANCO PERMITIU TRUNCATE NA TRILHA")
	}
}

// REGRESSÃO CRÍTICA: dado sensível NUNCA entra na trilha, nem quando o
// chamador manda por engano.
func TestDBTrilhaNuncaGravaDadoSensivel(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	rec := audit.New(db, "payment-service-test")

	const segredo = "SEGREDO-4111111111111111"
	entidade := uniqueSource("sens")
	r, err := rec.Record(ctx, audit.Entry{
		ActorID: "user-1", EntityType: "teste_sensivel", EntityID: entidade,
		Action: audit.ActionUpdate,
		OldValue: map[string]any{
			"password": segredo, "access_token": segredo, "cvv": segredo,
		},
		NewValue: map[string]any{
			"card_number": segredo, "client_secret": segredo,
			"amount_cents": 1999, // este SIM tem que sobreviver
			"nested":       map[string]any{"refresh_token": segredo},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var oldV, newV []byte
	if err := db.QueryRow(
		`SELECT COALESCE(old_value::text,'')::bytea, COALESCE(new_value::text,'')::bytea FROM audit_log WHERE seq=$1`,
		r.Seq).Scan(&oldV, &newV); err != nil {
		t.Fatal(err)
	}
	gravado := string(oldV) + string(newV)
	if strings.Contains(gravado, segredo) {
		t.Fatalf("DADO SENSÍVEL GRAVADO NA TRILHA IMUTÁVEL (não dá pra apagar): %s", gravado)
	}
	if !strings.Contains(gravado, "1999") {
		t.Errorf("campo legítimo foi perdido junto: %s", gravado)
	}
}

// Concorrência: N goroutines gravando ao mesmo tempo não podem bifurcar a
// cadeia (dois registros com o mesmo prev_hash).
func TestDBTrilhaNaoBifurcaSobConcorrencia(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	rec := audit.New(db, "payment-service-test")

	entidade := uniqueSource("conc")
	const n = 12
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			_, err := rec.Record(ctx, audit.Entry{
				ActorID: "user-conc", EntityType: "teste_concorrencia", EntityID: entidade,
				Action: audit.ActionCreate, NewValue: map[string]any{"i": i},
			})
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("gravação concorrente falhou: %v", err)
		}
	}

	recs, err := rec.List(ctx, audit.ListFilter{EntityType: "teste_concorrencia", EntityID: entidade, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != n {
		t.Fatalf("gravou %d de %d registros", len(recs), n)
	}
	// Nenhum prev_hash repetido: cadeia linear, não bifurcada.
	vistos := map[string]bool{}
	for _, r := range recs {
		if vistos[r.PrevHash] {
			t.Fatalf("CADEIA BIFURCADA: prev_hash %s aparece duas vezes", r.PrevHash)
		}
		vistos[r.PrevHash] = true
	}
	if err := rec.VerifyAll(ctx); err != nil {
		t.Fatalf("cadeia inválida após concorrência: %v", err)
	}
}

// Todo lançamento no livro deixa rastro de quem lançou.
func TestDBLancamentoContabilGeraRegistroNaTrilha(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	rec := audit.New(db, "payment-service-test")
	p := ledger.NewPoster(db, rec)

	tx, err := p.Post(ctx, ledger.Sale(ledger.SaleInput{
		PaymentID: uniqueSource("aud"), OrderID: "o", OccurredAt: time.Now().UTC(),
		GrossCents: 12345, Method: "pix",
	}))
	if err != nil {
		t.Fatal(err)
	}

	recs, err := rec.List(ctx, audit.ListFilter{EntityType: "ledger_transaction", EntityID: tx.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("lançamento sem registro na trilha (%d registros)", len(recs))
	}
	if recs[0].Action != string(audit.ActionCreate) {
		t.Errorf("ação = %q", recs[0].Action)
	}
	if !strings.Contains(string(recs[0].NewValue), "12345") {
		t.Errorf("trilha não guardou o valor do lançamento: %s", recs[0].NewValue)
	}
}
