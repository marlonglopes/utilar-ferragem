package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/utilar/payment-service/internal/psp"
)

// Reconciliação: comparar o que TEMOS com o que o PSP DIZ.
//
// PRINCÍPIO INEGOCIÁVEL: esta rotina NUNCA corrige nada sozinha.
//
// Divergência de dinheiro tem exatamente duas causas possíveis: bug nosso ou
// fraude. Nos dois casos, a resposta certa é PARAR e chamar gente — jamais
// "ajustar o nosso valor pro do PSP". Uma reconciliação que auto-corrige é
// indistinguível de um atacante que consegue escrever no PSP: ela apaga a
// evidência do próprio ataque. Por isso o resultado aqui é sempre um registro
// em reconciliation_discrepancies + métrica + log de erro.

// DiscrepancyKind classifica a divergência.
type DiscrepancyKind string

const (
	// DiscAmount: valor local != valor no PSP. A mais grave.
	DiscAmount DiscrepancyKind = "amount_mismatch"
	// DiscStatus: nós achamos confirmado e o PSP diz outra coisa (ou vice-versa).
	DiscStatus DiscrepancyKind = "status_mismatch"
	// DiscMissingAtPSP: temos o pagamento, o PSP não conhece o id.
	DiscMissingAtPSP DiscrepancyKind = "missing_at_psp"
	// DiscPSPError: não deu pra consultar. Não é divergência provada, mas é
	// buraco de cobertura e precisa aparecer.
	DiscPSPError DiscrepancyKind = "psp_error"
	// DiscLedgerMissing: pagamento confirmado sem lançamento no livro. O caso
	// que faz o faturamento do dashboard não bater com o extrato.
	DiscLedgerMissing DiscrepancyKind = "ledger_missing"
)

// Discrepancy é uma divergência encontrada.
type Discrepancy struct {
	ID           string          `json:"id"`
	RunID        string          `json:"runId"`
	PaymentID    string          `json:"paymentId,omitempty"`
	PSPPaymentID string          `json:"pspPaymentId,omitempty"`
	Kind         DiscrepancyKind `json:"kind"`
	Severity     string          `json:"severity"`
	LocalValue   string          `json:"localValue"`
	PSPValue     string          `json:"pspValue"`
	DeltaCents   Cents           `json:"amountDeltaCents"`
	Detail       string          `json:"detail,omitempty"`
	DetectedAt   time.Time       `json:"detectedAt"`
	ResolvedAt   *time.Time      `json:"resolvedAt,omitempty"`
	ResolvedBy   string          `json:"resolvedBy,omitempty"`
	Resolution   string          `json:"resolutionNote,omitempty"`
}

// RunResult é o resumo de uma execução.
type RunResult struct {
	ID            string        `json:"id"`
	Provider      string        `json:"provider"`
	StartedAt     time.Time     `json:"startedAt"`
	FinishedAt    time.Time     `json:"finishedAt"`
	From          time.Time     `json:"from"`
	To            time.Time     `json:"to"`
	Checked       int           `json:"checkedCount"`
	Errors        int           `json:"errorCount"`
	Status        string        `json:"status"` // ok | discrepancies | failed
	Discrepancies []Discrepancy `json:"discrepancies"`
}

// Observer recebe os eventos da reconciliação pra virar métrica. Interface
// mínima pra que internal/ledger não dependa de Prometheus.
type Observer interface {
	ReconciliationChecked(provider string)
	ReconciliationDiscrepancy(provider string, kind string)
	ReconciliationRun(provider string, status string, duration time.Duration)
}

type nopObserver struct{}

func (nopObserver) ReconciliationChecked(string)                    {}
func (nopObserver) ReconciliationDiscrepancy(string, string)        {}
func (nopObserver) ReconciliationRun(string, string, time.Duration) {}

// Reconciler compara payments locais com o PSP.
type Reconciler struct {
	db      *sql.DB
	gateway psp.Gateway
	obs     Observer
}

func NewReconciler(db *sql.DB, gw psp.Gateway, obs Observer) *Reconciler {
	if obs == nil {
		obs = nopObserver{}
	}
	return &Reconciler{db: db, gateway: gw, obs: obs}
}

// maxCheckPerRun limita o custo de uma execução — o PSP tem rate limit e a
// reconciliação não pode virar o motivo de o checkout ser recusado por 429.
const maxCheckPerRun = 500

// Run executa a reconciliação da janela [from, to).
//
// Devolve o RunResult mesmo quando encontra divergências: divergência não é
// erro da rotina, é o produto dela. Erro retornado significa "a rotina não
// conseguiu rodar" — e aí ninguém sabe se está tudo bem, o que também é alerta.
func (r *Reconciler) Run(ctx context.Context, from, to time.Time, requestID string) (*RunResult, error) {
	started := time.Now().UTC()
	provider := r.gateway.Name()

	var runID string
	if err := r.db.QueryRowContext(ctx, `
		INSERT INTO reconciliation_runs (provider, window_from, window_to, request_id, status)
		VALUES ($1,$2,$3,$4,'running') RETURNING id`,
		provider, from, to, requestID).Scan(&runID); err != nil {
		return nil, fmt.Errorf("reconcile: abrir run: %w", err)
	}

	res := &RunResult{ID: runID, Provider: provider, StartedAt: started, From: from, To: to,
		Discrepancies: []Discrepancy{}}

	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, COALESCE(p.psp_payment_id,''), p.status, p.amount, p.method,
		       EXISTS (SELECT 1 FROM ledger_transactions t
		               WHERE t.source_type='payment' AND t.source_id = p.id::text AND t.kind='sale')
		FROM payments p
		WHERE p.created_at >= $1 AND p.created_at < $2
		  AND p.psp_payment_id IS NOT NULL AND p.psp_payment_id <> ''
		ORDER BY p.created_at
		LIMIT $3`, from, to, maxCheckPerRun)
	if err != nil {
		r.finishRun(ctx, runID, res, "failed")
		return nil, fmt.Errorf("reconcile: listar pagamentos: %w", err)
	}

	type local struct {
		id, pspID, status, method string
		amount                    float64
		hasLedger                 bool
	}
	var pending []local
	for rows.Next() {
		var l local
		if err := rows.Scan(&l.id, &l.pspID, &l.status, &l.amount, &l.method, &l.hasLedger); err != nil {
			rows.Close()
			r.finishRun(ctx, runID, res, "failed")
			return nil, err
		}
		pending = append(pending, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		r.finishRun(ctx, runID, res, "failed")
		return nil, err
	}

	for _, l := range pending {
		res.Checked++
		r.obs.ReconciliationChecked(provider)

		// Pagamento confirmado sem lançamento no livro: divergência interna,
		// nem precisa perguntar ao PSP.
		if l.status == "confirmed" && !l.hasLedger {
			r.record(ctx, res, Discrepancy{
				RunID: runID, PaymentID: l.id, PSPPaymentID: l.pspID,
				Kind: DiscLedgerMissing, Severity: "high",
				LocalValue: "confirmed, sem lançamento", PSPValue: "-",
				DeltaCents: toCents(l.amount),
				Detail:     "pagamento confirmado não foi lançado no livro — receita do período está subestimada",
			})
		}

		got, err := r.gateway.GetPayment(ctx, l.pspID)
		if err != nil {
			kind, sev := DiscPSPError, "medium"
			if errors.Is(err, psp.ErrNotFound) {
				// Temos um id que o PSP não conhece. Ou o id está corrompido, ou
				// alguém escreveu um psp_payment_id inventado no nosso banco.
				kind, sev = DiscMissingAtPSP, "critical"
			}
			r.record(ctx, res, Discrepancy{
				RunID: runID, PaymentID: l.id, PSPPaymentID: l.pspID,
				Kind: kind, Severity: sev,
				LocalValue: fmt.Sprintf("%s / %s", l.status, toCents(l.amount)),
				PSPValue:   "erro na consulta",
				// Nunca colocamos err.Error() aqui: a mensagem do gateway carrega
				// o corpo cru do PSP, que pode ter PII do comprador.
				Detail: "GetPayment falhou (ver logs pelo request_id)",
			})
			res.Errors++
			slog.Error("reconcile: GetPayment falhou",
				"provider", provider, "payment_id", l.id, "psp_id", l.pspID,
				"error", err, "request_id", requestID)
			continue
		}

		localC, pspC := toCents(l.amount), toCents(got.Amount)
		if localC != pspC {
			r.record(ctx, res, Discrepancy{
				RunID: runID, PaymentID: l.id, PSPPaymentID: l.pspID,
				Kind: DiscAmount, Severity: "critical",
				LocalValue: localC.String(), PSPValue: pspC.String(),
				DeltaCents: pspC - localC,
				Detail:     "divergência de VALOR — exige apuração humana, nada é corrigido automaticamente",
			})
			// Log em ERROR com destaque: é o evento que acorda alguém.
			slog.Error("reconcile: DIVERGÊNCIA DE VALOR",
				"provider", provider, "payment_id", l.id, "psp_id", l.pspID,
				"local_cents", int64(localC), "psp_cents", int64(pspC),
				"delta_cents", int64(pspC-localC), "request_id", requestID)
			continue
		}

		if want := normalizeLocalStatus(got.Status); want != l.status && !statusCompatible(l.status, want) {
			r.record(ctx, res, Discrepancy{
				RunID: runID, PaymentID: l.id, PSPPaymentID: l.pspID,
				Kind: DiscStatus, Severity: "high",
				LocalValue: l.status, PSPValue: want,
				Detail: "status local diverge do PSP — não sincronizamos automaticamente aqui",
			})
		}
	}

	status := "ok"
	if len(res.Discrepancies) > 0 {
		status = "discrepancies"
	}
	r.finishRun(ctx, runID, res, status)
	res.FinishedAt = time.Now().UTC()
	res.Status = status
	r.obs.ReconciliationRun(provider, status, res.FinishedAt.Sub(started))
	return res, nil
}

func (r *Reconciler) record(ctx context.Context, res *RunResult, d Discrepancy) {
	d.DetectedAt = time.Now().UTC()
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO reconciliation_discrepancies
			(run_id, payment_id, psp_payment_id, kind, severity, local_value, psp_value, amount_delta_cents, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		d.RunID, nullUUID(d.PaymentID), d.PSPPaymentID, string(d.Kind), d.Severity,
		d.LocalValue, d.PSPValue, int64(d.DeltaCents), d.Detail).Scan(&d.ID)
	if err != nil {
		// Não abortamos a run: perder UMA divergência no banco é ruim, perder
		// as outras 400 por causa dela é pior. O log garante que não some.
		slog.Error("reconcile: gravar divergência", "error", err, "kind", d.Kind, "payment_id", d.PaymentID)
	}
	res.Discrepancies = append(res.Discrepancies, d)
	r.obs.ReconciliationDiscrepancy(res.Provider, string(d.Kind))
}

func (r *Reconciler) finishRun(ctx context.Context, runID string, res *RunResult, status string) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE reconciliation_runs
		SET finished_at = now(), checked_count = $1, discrepancy_count = $2, error_count = $3, status = $4
		WHERE id = $5`, res.Checked, len(res.Discrepancies), res.Errors, status, runID); err != nil {
		slog.Error("reconcile: fechar run", "error", err, "run_id", runID)
	}
}

// Resolve marca uma divergência como tratada por um humano. NÃO mexe no
// pagamento nem no livro — só registra que alguém olhou e o que concluiu.
func (r *Reconciler) Resolve(ctx context.Context, discID, actor, note string) error {
	if note == "" {
		return errors.New("reconcile: resolução exige nota explicando a conclusão")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE reconciliation_discrepancies
		SET resolved_at = now(), resolved_by = $1, resolution_note = $2
		WHERE id = $3 AND resolved_at IS NULL`, actor, note, discID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("reconcile: divergência inexistente ou já resolvida")
	}
	return nil
}

// OpenDiscrepancies lista as divergências não resolvidas — é a fila de trabalho
// do financeiro e a fonte do alerta.
func (r *Reconciler) OpenDiscrepancies(ctx context.Context, limit int) ([]Discrepancy, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, COALESCE(payment_id::text,''), psp_payment_id, kind, severity,
		       local_value, psp_value, amount_delta_cents, detail, detected_at
		FROM reconciliation_discrepancies
		WHERE resolved_at IS NULL
		ORDER BY detected_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Discrepancy{}
	for rows.Next() {
		var d Discrepancy
		var delta int64
		if err := rows.Scan(&d.ID, &d.RunID, &d.PaymentID, &d.PSPPaymentID, &d.Kind, &d.Severity,
			&d.LocalValue, &d.PSPValue, &delta, &d.Detail, &d.DetectedAt); err != nil {
			return nil, err
		}
		d.DeltaCents = Cents(delta)
		out = append(out, d)
	}
	return out, rows.Err()
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// toCents converte o float64 que vem de NUMERIC(12,2)/psp.GetResult pra
// centavos com ARREDONDAMENTO. Truncar aqui perderia um centavo em 19.99
// (que em float64 é 19.989999999999998) — o mesmo bug documentado em
// internal/psp/appmaxv1/money_test.go. A comparação de dinheiro é feita
// SEMPRE em centavos inteiros, nunca com tolerância de float.
func toCents(reais float64) Cents { return Cents(math.Round(reais * 100)) }

func normalizeLocalStatus(s psp.PaymentStatus) string {
	switch s {
	case psp.StatusApproved:
		return "confirmed"
	case psp.StatusRejected:
		return "failed"
	case psp.StatusCancelled:
		return "cancelled"
	case psp.StatusExpired:
		return "expired"
	default:
		return "pending"
	}
}

// statusCompatible tolera as transições em voo: o PSP aprovou e o webhook
// ainda não chegou (ou vice-versa) é normal por alguns segundos e não deve
// gerar ruído. O que NÃO é tolerado: nós confirmados e o PSP recusado.
func statusCompatible(local, fromPSP string) bool {
	return local == "pending" && (fromPSP == "confirmed" || fromPSP == "pending")
}
