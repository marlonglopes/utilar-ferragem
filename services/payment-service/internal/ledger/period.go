package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/utilar/pkg/audit"
)

// Fechamento de período.
//
// POR QUE FECHAR
// Enquanto o mês está aberto, qualquer lançamento novo muda um número que já
// foi entregue ao contador. Fechar é dizer "junho acabou, o saldo é este, e
// ninguém mais lança em junho". Sem isso, nenhum relatório é reproduzível: a
// mesma consulta rodada em dois dias diferentes dá resultados diferentes e não
// há como saber qual estava certo.
//
// A trava é NO BANCO (trigger trg_ledger_tx_set_period, migration 005), não
// num if do Go — inclusive um job antigo ainda rodando é recusado.

// Period é "2026-07".
type Period string

// PeriodOf devolve o período de um instante (UTC).
func PeriodOf(t time.Time) Period { return Period(t.UTC().Format("2006-01")) }

// Range devolve a janela [from, to) do período.
func (p Period) Range() (time.Time, time.Time, error) {
	from, err := time.ParseInLocation("2006-01", string(p), time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("período inválido %q (esperado YYYY-MM): %w", p, err)
	}
	return from, from.AddDate(0, 1, 0), nil
}

// PeriodStatus é o estado de um mês contábil.
type PeriodStatus struct {
	Period          Period           `json:"period"`
	Status          string           `json:"status"` // open | closed
	ClosedAt        *time.Time       `json:"closedAt,omitempty"`
	ClosedBy        string           `json:"closedBy,omitempty"`
	EntriesCount    int64            `json:"entriesCount"`
	ClosingBalances map[string]Cents `json:"closingBalances,omitempty"`
	Totals          *Summary         `json:"totals,omitempty"`
}

// Closer fecha e consulta períodos.
type Closer struct {
	db      *sql.DB
	reports *Reports
	audit   *audit.Recorder
}

func NewCloser(db *sql.DB, rec *audit.Recorder) *Closer {
	return &Closer{db: db, reports: NewReports(db), audit: rec}
}

var (
	ErrAlreadyClosed  = errors.New("ledger: período já está fechado")
	ErrPeriodInFuture = errors.New("ledger: não se fecha período que ainda não terminou")
	ErrPeriodNotFound = errors.New("ledger: período não encontrado")
)

// Status devolve o estado do período.
func (c *Closer) Status(ctx context.Context, p Period) (*PeriodStatus, error) {
	from, to, err := p.Range()
	if err != nil {
		return nil, err
	}
	out := &PeriodStatus{Period: p, Status: "open"}

	var closedAt sql.NullTime
	var balances []byte
	var totals []byte
	err = c.db.QueryRowContext(ctx, `
		SELECT status, closed_at, closed_by, entries_count,
		       COALESCE(closing_balances,'null'::jsonb), COALESCE(totals_cents,'null'::jsonb)
		FROM ledger_periods WHERE period = $1`, from).
		Scan(&out.Status, &closedAt, &out.ClosedBy, &out.EntriesCount, &balances, &totals)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Período sem linha = aberto e ainda não fechado. Contamos ao vivo.
		if err := c.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
			WHERE t.occurred_at >= $1 AND t.occurred_at < $2`, from, to).Scan(&out.EntriesCount); err != nil {
			return nil, err
		}
		return out, nil
	case err != nil:
		return nil, err
	}
	if closedAt.Valid {
		out.ClosedAt = &closedAt.Time
	}
	_ = json.Unmarshal(balances, &out.ClosingBalances)
	_ = json.Unmarshal(totals, &out.Totals)
	return out, nil
}

// Close trava o período e persiste o saldo de fechamento.
//
// Regras:
//   - só fecha período JÁ TERMINADO (fechar o mês corrente esconderia vendas
//     que ainda vão acontecer nele e o lançamento seria recusado depois);
//   - recusa fechar se o balancete não bate — período não fecha em cima de
//     livro corrompido;
//   - grava os saldos de todas as contas em closing_balances, pra que o
//     relatório do mês seja reproduzível mesmo que a base cresça;
//   - registra na trilha de auditoria QUEM fechou.
func (c *Closer) Close(ctx context.Context, p Period, actor, actorIP, requestID string) (*PeriodStatus, error) {
	from, to, err := p.Range()
	if err != nil {
		return nil, err
	}
	if !to.Before(time.Now().UTC()) {
		return nil, fmt.Errorf("%w: %s termina em %s", ErrPeriodInFuture, p, to.Format("2006-01-02"))
	}

	st, err := c.Status(ctx, p)
	if err != nil {
		return nil, err
	}
	if st.Status == "closed" {
		return nil, fmt.Errorf("%w: %s (fechado em %v por %s)", ErrAlreadyClosed, p, st.ClosedAt, st.ClosedBy)
	}

	tb, err := c.reports.TrialBalance(ctx, from, to)
	if err != nil {
		return nil, err
	}
	if !tb.Balanced {
		// Isto não deveria ser possível (constraint trigger), então se acontecer
		// é corrupção de dados — não se fecha um mês em cima disso.
		return nil, fmt.Errorf("%w: balancete de %s não fecha (débitos=%d, créditos=%d) — INCIDENTE, não feche o período",
			ErrUnbalanced, p, tb.TotalDebits, tb.TotalCredits)
	}

	summary, err := c.reports.Summary(ctx, from, to)
	if err != nil {
		return nil, err
	}

	balances := make(map[string]Cents, len(tb.Lines))
	for _, l := range tb.Lines {
		balances[string(l.Account)] = l.BalanceCents
	}
	balancesJSON, _ := json.Marshal(balances)
	totalsJSON, _ := json.Marshal(summary)

	var entriesCount int64
	if err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.occurred_at >= $1 AND t.occurred_at < $2`, from, to).Scan(&entriesCount); err != nil {
		return nil, err
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	// INSERT direto como 'closed': o trigger de não-reabertura bloqueia UPDATE
	// de linha já fechada, então nunca criamos a linha 'open' primeiro.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_periods (period, status, closed_at, closed_by, closing_balances, totals_cents, entries_count)
		VALUES ($1, 'closed', $2, $3, $4, $5, $6)`,
		from, now, actor, balancesJSON, totalsJSON, entriesCount); err != nil {
		return nil, fmt.Errorf("ledger: fechar período: %w", err)
	}

	if c.audit != nil {
		if _, err := c.audit.RecordTx(ctx, tx, audit.Entry{
			ActorID: actor, ActorRole: "admin", ActorIP: actorIP,
			EntityType: "ledger_period", EntityID: string(p),
			Action:   audit.ActionApprove,
			OldValue: map[string]any{"status": "open"},
			NewValue: map[string]any{
				"status": "closed", "closing_balances": balances,
				"totals": summary, "entries_count": entriesCount,
			},
			RequestID:  requestID,
			OccurredAt: now,
		}); err != nil {
			return nil, fmt.Errorf("ledger: auditoria do fechamento: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &PeriodStatus{
		Period: p, Status: "closed", ClosedAt: &now, ClosedBy: actor,
		EntriesCount: entriesCount, ClosingBalances: balances, Totals: summary,
	}, nil
}

// List devolve os períodos já fechados, mais recente primeiro.
func (c *Closer) List(ctx context.Context, limit int) ([]PeriodStatus, error) {
	if limit <= 0 || limit > 120 {
		limit = 24
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT period, status, closed_at, closed_by, entries_count
		FROM ledger_periods ORDER BY period DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PeriodStatus{}
	for rows.Next() {
		var ps PeriodStatus
		var period time.Time
		var closedAt sql.NullTime
		if err := rows.Scan(&period, &ps.Status, &closedAt, &ps.ClosedBy, &ps.EntriesCount); err != nil {
			return nil, err
		}
		ps.Period = Period(period.Format("2006-01"))
		if closedAt.Valid {
			ps.ClosedAt = &closedAt.Time
		}
		out = append(out, ps)
	}
	return out, rows.Err()
}
