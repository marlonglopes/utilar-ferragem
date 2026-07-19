package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Este arquivo é o contrato que o dashboard consome. Ver docs/ledger-api.md.
//
// Convenção de janela em TODO relatório: [from, to) — `from` inclusivo, `to`
// EXCLUSIVO. Assim "julho" é [2026-07-01, 2026-08-01) e não existe a dúvida
// clássica de o último dia entrar ou não.
//
// Convenção de sinal: todo valor sai em CENTAVOS inteiros e SEMPRE positivo,
// com o significado no nome do campo. `refundsCents: 1000` significa "mil
// centavos foram estornados", não "-1000 de receita". Quem exibe decide o sinal.

// Reports é o leitor do livro. Só leitura — não existe caminho de escrita aqui.
type Reports struct{ db *sql.DB }

func NewReports(db *sql.DB) *Reports { return &Reports{db: db} }

// Summary é o cartão principal do dashboard.
type Summary struct {
	From              time.Time `json:"from"`
	To                time.Time `json:"to"`
	Currency          string    `json:"currency"`
	GrossCents        Cents     `json:"grossCents"`            // receita bruta (3.1.1)
	PSPFeesCents      Cents     `json:"pspFeesCents"`          // taxas do gateway (4.1.1)
	AnticipationCents Cents     `json:"anticipationFeesCents"` // taxa de antecipação (4.1.2)
	RefundsCents      Cents     `json:"refundsCents"`          // estornos (3.1.8)
	ChargebacksCents  Cents     `json:"chargebacksCents"`      // chargebacks (3.1.9)
	SellerSplitCents  Cents     `json:"sellerSplitCents"`      // repasses reconhecidos (4.2.1)
	NetCents          Cents     `json:"netCents"`              // bruto - taxas - estornos - chargebacks - repasses
	TxCount           int64     `json:"transactionCount"`
}

// accountFlow soma o movimento LÍQUIDO de uma conta na janela, no sentido
// natural dela (débito - crédito pra conta devedora; o inverso pra credora).
//
// O PORQUÊ: somar só os débitos de 3.1.8 daria o total estornado, mas ignoraria
// um lançamento de reversão (estorno do estorno, quando o operador errou). O
// líquido é o número que o contador quer.
const accountFlowSQL = `
	SELECT COALESCE(SUM(
		CASE WHEN e.side = $1 THEN e.amount_cents ELSE -e.amount_cents END
	), 0)
	FROM ledger_entries e
	JOIN ledger_transactions t ON t.id = e.transaction_id
	WHERE e.account_code = $2 AND t.occurred_at >= $3 AND t.occurred_at < $4`

func (r *Reports) accountFlow(ctx context.Context, acct Account, from, to time.Time) (Cents, error) {
	meta, ok := MetaOf(acct)
	if !ok {
		return 0, fmt.Errorf("conta desconhecida %q", acct)
	}
	var v int64
	err := r.db.QueryRowContext(ctx, accountFlowSQL, string(meta.NormalSide), string(acct), from, to).Scan(&v)
	return Cents(v), err
}

// Summary devolve o consolidado da janela [from, to).
func (r *Reports) Summary(ctx context.Context, from, to time.Time) (*Summary, error) {
	s := &Summary{From: from, To: to, Currency: "BRL"}

	type slot struct {
		acct Account
		dst  *Cents
	}
	for _, sl := range []slot{
		{AcctReceitaBruta, &s.GrossCents},
		{AcctTaxaPSP, &s.PSPFeesCents},
		{AcctTaxaAntecipacao, &s.AnticipationCents},
		{AcctEstornos, &s.RefundsCents},
		{AcctChargebacks, &s.ChargebacksCents},
		{AcctCustoRepasse, &s.SellerSplitCents},
	} {
		v, err := r.accountFlow(ctx, sl.acct, from, to)
		if err != nil {
			return nil, err
		}
		*sl.dst = v
	}

	s.NetCents = s.GrossCents - s.PSPFeesCents - s.AnticipationCents -
		s.RefundsCents - s.ChargebacksCents - s.SellerSplitCents

	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ledger_transactions WHERE occurred_at >= $1 AND occurred_at < $2`,
		from, to).Scan(&s.TxCount); err != nil {
		return nil, err
	}
	return s, nil
}

// MethodBreakdown é o recorte por forma de pagamento.
type MethodBreakdown struct {
	Method       string `json:"method"` // pix | boleto | card | "" (sem método)
	GrossCents   Cents  `json:"grossCents"`
	PSPFeesCents Cents  `json:"pspFeesCents"`
	RefundsCents Cents  `json:"refundsCents"`
	NetCents     Cents  `json:"netCents"`
	SaleCount    int64  `json:"saleCount"`
}

// ByMethod agrupa a janela por forma de pagamento.
func (r *Reports) ByMethod(ctx context.Context, from, to time.Time) ([]MethodBreakdown, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.payment_method,
		       COALESCE(SUM(CASE WHEN e.account_code = $1 AND e.side='credit' THEN e.amount_cents
		                         WHEN e.account_code = $1 AND e.side='debit'  THEN -e.amount_cents ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN e.account_code = $2 AND e.side='debit'  THEN e.amount_cents
		                         WHEN e.account_code = $2 AND e.side='credit' THEN -e.amount_cents ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN e.account_code = $3 AND e.side='debit'  THEN e.amount_cents
		                         WHEN e.account_code = $3 AND e.side='credit' THEN -e.amount_cents ELSE 0 END), 0),
		       COUNT(DISTINCT CASE WHEN t.kind = 'sale' THEN t.id END)
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.occurred_at >= $4 AND t.occurred_at < $5
		GROUP BY e.payment_method
		ORDER BY 2 DESC`,
		AcctReceitaBruta, AcctTaxaPSP, AcctEstornos, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MethodBreakdown{}
	for rows.Next() {
		var m MethodBreakdown
		var gross, fees, refunds int64
		if err := rows.Scan(&m.Method, &gross, &fees, &refunds, &m.SaleCount); err != nil {
			return nil, err
		}
		m.GrossCents, m.PSPFeesCents, m.RefundsCents = Cents(gross), Cents(fees), Cents(refunds)
		m.NetCents = m.GrossCents - m.PSPFeesCents - m.RefundsCents
		out = append(out, m)
	}
	return out, rows.Err()
}

// TrialBalance é o balancete: saldo de cada conta. A soma de TODOS os saldos
// (débito - crédito) tem que dar ZERO. Se não der, o livro está corrompido —
// e isso é um incidente, não um bug de relatório.
type TrialBalanceLine struct {
	Account      Account `json:"account"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	DebitsCents  Cents   `json:"debitsCents"`
	CreditsCents Cents   `json:"creditsCents"`
	BalanceCents Cents   `json:"balanceCents"` // no sentido natural da conta
}

type TrialBalance struct {
	From         time.Time          `json:"from"`
	To           time.Time          `json:"to"`
	Lines        []TrialBalanceLine `json:"lines"`
	TotalDebits  Cents              `json:"totalDebitsCents"`
	TotalCredits Cents              `json:"totalCreditsCents"`
	Balanced     bool               `json:"balanced"`
}

func (r *Reports) TrialBalance(ctx context.Context, from, to time.Time) (*TrialBalance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.account_code,
		       COALESCE(SUM(e.amount_cents) FILTER (WHERE e.side='debit'), 0),
		       COALESCE(SUM(e.amount_cents) FILTER (WHERE e.side='credit'), 0)
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.occurred_at >= $1 AND t.occurred_at < $2
		GROUP BY e.account_code ORDER BY e.account_code`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tb := &TrialBalance{From: from, To: to, Lines: []TrialBalanceLine{}}
	for rows.Next() {
		var l TrialBalanceLine
		var d, c int64
		if err := rows.Scan(&l.Account, &d, &c); err != nil {
			return nil, err
		}
		l.DebitsCents, l.CreditsCents = Cents(d), Cents(c)
		if m, ok := MetaOf(l.Account); ok {
			l.Name, l.Type = m.Name, m.Type
			if m.NormalSide == Debit {
				l.BalanceCents = l.DebitsCents - l.CreditsCents
			} else {
				l.BalanceCents = l.CreditsCents - l.DebitsCents
			}
		}
		tb.TotalDebits += l.DebitsCents
		tb.TotalCredits += l.CreditsCents
		tb.Lines = append(tb.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tb.Balanced = tb.TotalDebits == tb.TotalCredits
	return tb, rows.Err()
}

// DailyPoint é um ponto da série temporal do dashboard.
type DailyPoint struct {
	Day          string `json:"day"` // YYYY-MM-DD (UTC)
	GrossCents   Cents  `json:"grossCents"`
	PSPFeesCents Cents  `json:"pspFeesCents"`
	RefundsCents Cents  `json:"refundsCents"`
	NetCents     Cents  `json:"netCents"`
}

// Daily devolve a série diária da janela. Limitada a 400 pontos — janela maior
// que isso é relatório, não gráfico, e deve sair pelo export CSV.
func (r *Reports) Daily(ctx context.Context, from, to time.Time) ([]DailyPoint, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT to_char(date_trunc('day', t.occurred_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD'),
		       COALESCE(SUM(CASE WHEN e.account_code = $1 AND e.side='credit' THEN e.amount_cents
		                         WHEN e.account_code = $1 AND e.side='debit'  THEN -e.amount_cents ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN e.account_code = $2 AND e.side='debit'  THEN e.amount_cents
		                         WHEN e.account_code = $2 AND e.side='credit' THEN -e.amount_cents ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN e.account_code = $3 AND e.side='debit'  THEN e.amount_cents
		                         WHEN e.account_code = $3 AND e.side='credit' THEN -e.amount_cents ELSE 0 END), 0)
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.occurred_at >= $4 AND t.occurred_at < $5
		GROUP BY 1 ORDER BY 1
		LIMIT 400`, AcctReceitaBruta, AcctTaxaPSP, AcctEstornos, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DailyPoint{}
	for rows.Next() {
		var p DailyPoint
		var g, f, rf int64
		if err := rows.Scan(&p.Day, &g, &f, &rf); err != nil {
			return nil, err
		}
		p.GrossCents, p.PSPFeesCents, p.RefundsCents = Cents(g), Cents(f), Cents(rf)
		p.NetCents = p.GrossCents - p.PSPFeesCents - p.RefundsCents
		out = append(out, p)
	}
	return out, rows.Err()
}

// EntryRow é uma linha do razão (usada em listagem e export).
type EntryRow struct {
	TxID        string     `json:"transactionId"`
	OccurredAt  time.Time  `json:"occurredAt"`
	Kind        Kind       `json:"kind"`
	SourceType  SourceType `json:"sourceType"`
	SourceID    string     `json:"sourceId"`
	Description string     `json:"description"`
	Account     Account    `json:"account"`
	AccountName string     `json:"accountName"`
	Side        Side       `json:"side"`
	AmountCents Cents      `json:"amountCents"`
	Method      string     `json:"paymentMethod,omitempty"`
	SellerID    string     `json:"sellerId,omitempty"`
	Memo        string     `json:"memo,omitempty"`
	RequestID   string     `json:"requestId,omitempty"`
}

// Entries lista o razão da janela, ordenado cronologicamente.
func (r *Reports) Entries(ctx context.Context, from, to time.Time, limit int) ([]EntryRow, error) {
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.occurred_at, t.kind, t.source_type, t.source_id, t.description,
		       e.account_code, e.side, e.amount_cents, e.payment_method, e.seller_id, e.memo, t.request_id
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.occurred_at >= $1 AND t.occurred_at < $2
		ORDER BY t.occurred_at, t.id, e.id
		LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EntryRow{}
	for rows.Next() {
		var e EntryRow
		var amt int64
		if err := rows.Scan(&e.TxID, &e.OccurredAt, &e.Kind, &e.SourceType, &e.SourceID,
			&e.Description, &e.Account, &e.Side, &amt, &e.Method, &e.SellerID, &e.Memo, &e.RequestID); err != nil {
			return nil, err
		}
		e.AmountCents = Cents(amt)
		if m, ok := MetaOf(e.Account); ok {
			e.AccountName = m.Name
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
