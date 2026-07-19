package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/utilar/pkg/audit"
	"github.com/utilar/pkg/requestid"
)

// Cents é dinheiro. int64, sempre. O tipo existe pra que `Cents` num parâmetro
// não seja confundível com "reais" num code review.
type Cents int64

// Reais formata pra exibição/CSV (pt-BR usa vírgula; ver export.go).
func (c Cents) Reais() float64 { return float64(c) / 100 }

// String é R$ com duas casas — só pra log e mensagem de erro.
func (c Cents) String() string {
	neg := ""
	v := int64(c)
	if v < 0 {
		neg, v = "-", -v
	}
	return fmt.Sprintf("%sR$ %d,%02d", neg, v/100, v%100)
}

// Posting é uma partida.
type Posting struct {
	Account       Account
	Side          Side
	Amount        Cents // SEMPRE positivo — o sinal é o Side
	PaymentMethod string
	SellerID      string
	Memo          string
}

// TxInput é o documento contábil a ser lançado.
type TxInput struct {
	OccurredAt  time.Time
	Kind        Kind
	SourceType  SourceType
	SourceID    string
	Description string
	Currency    string
	RequestID   string
	CreatedBy   string
	ReversesID  string // preenchido só por Reverse()
	Postings    []Posting
}

var (
	// ErrUnbalanced é a falha central: se um lançamento não fecha, alguém
	// perdeu ou inventou dinheiro. Nunca "arredonde pra fechar".
	ErrUnbalanced = errors.New("ledger: lançamento não fecha (débitos != créditos)")
	// ErrDuplicate indica que o documento de origem já foi lançado. É um caso
	// NORMAL (webhook reentregue), tratado como no-op pelos chamadores.
	ErrDuplicate = errors.New("ledger: lançamento já existe para este documento de origem")
	// ErrPeriodClosed: tentativa de lançar em mês fechado.
	ErrPeriodClosed = errors.New("ledger: período fechado, lançamento retroativo proibido")
	ErrInvalidInput = errors.New("ledger: lançamento inválido")
)

// Validate checa as invariantes ANTES de tocar no banco.
//
// Sim, o banco também checa (constraint trigger). A duplicação é deliberada:
// o erro do Go é legível e testável sem Postgres; o do banco é a garantia real
// contra qualquer caminho que não passe por aqui (psql, job de outro time).
func (t TxInput) Validate() error {
	if t.Kind == "" || t.SourceType == "" || strings.TrimSpace(t.SourceID) == "" {
		return fmt.Errorf("%w: kind, source_type e source_id são obrigatórios (lançamento tem que apontar pro documento de origem)", ErrInvalidInput)
	}
	if len(t.Postings) < 2 {
		return fmt.Errorf("%w: partida dobrada exige ao menos 2 partidas, veio %d", ErrInvalidInput, len(t.Postings))
	}
	var debits, credits Cents
	for i, p := range t.Postings {
		if p.Amount <= 0 {
			return fmt.Errorf("%w: partida %d com valor %d — valor é sempre positivo, o sinal é o Side", ErrInvalidInput, i, p.Amount)
		}
		if _, ok := MetaOf(p.Account); !ok {
			return fmt.Errorf("%w: partida %d com conta desconhecida %q", ErrInvalidInput, i, p.Account)
		}
		switch p.Side {
		case Debit:
			debits += p.Amount
		case Credit:
			credits += p.Amount
		default:
			return fmt.Errorf("%w: partida %d com side inválido %q", ErrInvalidInput, i, p.Side)
		}
	}
	if debits != credits {
		return fmt.Errorf("%w: débitos=%d centavos, créditos=%d centavos, diferença=%d centavos",
			ErrUnbalanced, debits, credits, debits-credits)
	}
	if debits == 0 {
		return fmt.Errorf("%w: lançamento de valor zero não é fato contábil", ErrInvalidInput)
	}
	return nil
}

// Total é o valor do lançamento (soma dos débitos == soma dos créditos).
func (t TxInput) Total() Cents {
	var d Cents
	for _, p := range t.Postings {
		if p.Side == Debit {
			d += p.Amount
		}
	}
	return d
}

// Poster grava no livro.
type Poster struct {
	db    *sql.DB
	audit *auditRecorder
}

// auditRecorder é a fatia do pkg/audit que o ledger usa. Interface própria pra
// que o Poster seja testável sem banco e pra que auditoria opcional (nil) não
// derrube o lançamento.
type auditRecorder struct{ r *audit.Recorder }

// NewPoster. `rec` pode ser nil (dev sem trilha), mas em produção não deve ser:
// um lançamento contábil sem registro de quem lançou é meio caminho pra fraude.
func NewPoster(db *sql.DB, rec *audit.Recorder) *Poster {
	var a *auditRecorder
	if rec != nil {
		a = &auditRecorder{r: rec}
	}
	return &Poster{db: db, audit: a}
}

// Tx é o lançamento persistido.
type Tx struct {
	ID          string     `json:"id"`
	OccurredAt  time.Time  `json:"occurredAt"`
	Period      string     `json:"period"` // "2026-07"
	Kind        Kind       `json:"kind"`
	SourceType  SourceType `json:"sourceType"`
	SourceID    string     `json:"sourceId"`
	Description string     `json:"description"`
	Currency    string     `json:"currency"`
	RequestID   string     `json:"requestId,omitempty"`
	ReversesID  string     `json:"reversesId,omitempty"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	Postings    []Posting  `json:"postings,omitempty"`
	TotalCents  Cents      `json:"totalCents"`
}

// Post grava o lançamento numa transação de banco própria.
//
// Idempotente por (kind, source_type, source_id): reentrega de webhook devolve
// ErrDuplicate, e o chamador trata como sucesso. Sem isso, o mesmo pagamento
// reentregue 4 vezes (a Appmax reentrega em 0, +30min, +2h, +4h) quadruplicaria
// a receita do dia.
func (p *Poster) Post(ctx context.Context, in TxInput) (*Tx, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = time.Now().UTC()
	}
	if in.Currency == "" {
		in.Currency = "BRL"
	}
	if in.CreatedBy == "" {
		in.CreatedBy = "system"
	}
	if in.RequestID == "" {
		in.RequestID = requestid.FromContext(ctx)
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("ledger: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op depois do commit

	out, err := p.postTx(ctx, tx, in)
	if err != nil {
		return nil, err
	}

	// A trilha entra na MESMA transação do lançamento: ou os dois existem, ou
	// nenhum. Auditoria gravada "depois" é auditoria que some no crash.
	if p.audit != nil {
		if _, aerr := p.audit.r.RecordTx(ctx, tx, audit.Entry{
			ActorID:    in.CreatedBy,
			ActorRole:  "system",
			EntityType: "ledger_transaction",
			EntityID:   out.ID,
			Action:     audit.ActionCreate,
			NewValue:   out,
			RequestID:  in.RequestID,
			OccurredAt: in.OccurredAt,
		}); aerr != nil {
			return nil, fmt.Errorf("ledger: auditoria do lançamento: %w", aerr)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, translatePGError(err)
	}
	return out, nil
}

func (p *Poster) postTx(ctx context.Context, tx *sql.Tx, in TxInput) (*Tx, error) {
	var reverses any
	if in.ReversesID != "" {
		reverses = in.ReversesID
	}

	var id string
	var period time.Time
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `
		INSERT INTO ledger_transactions
			(occurred_at, period, kind, source_type, source_id, description,
			 currency, request_id, reverses_id, created_by)
		VALUES ($1, date_trunc('month', $1 AT TIME ZONE 'UTC')::date, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (kind, source_type, source_id) DO NOTHING
		RETURNING id, period, created_at`,
		in.OccurredAt.UTC(), in.Kind, in.SourceType, in.SourceID, in.Description,
		in.Currency, in.RequestID, reverses, in.CreatedBy,
	).Scan(&id, &period, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s/%s/%s", ErrDuplicate, in.Kind, in.SourceType, in.SourceID)
	}
	if err != nil {
		return nil, translatePGError(err)
	}

	for _, pg := range in.Postings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_entries
				(transaction_id, account_code, side, amount_cents, payment_method, seller_id, memo)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, pg.Account, pg.Side, int64(pg.Amount), pg.PaymentMethod, pg.SellerID, pg.Memo,
		); err != nil {
			return nil, translatePGError(err)
		}
	}

	return &Tx{
		ID: id, OccurredAt: in.OccurredAt.UTC(), Period: period.Format("2006-01"),
		Kind: in.Kind, SourceType: in.SourceType, SourceID: in.SourceID,
		Description: in.Description, Currency: in.Currency, RequestID: in.RequestID,
		ReversesID: in.ReversesID, CreatedBy: in.CreatedBy, CreatedAt: createdAt,
		Postings: in.Postings, TotalCents: in.Total(),
	}, nil
}

// Reverse cria o lançamento de estorno de um lançamento existente: mesmas
// contas e valores, lados invertidos.
//
// É a ÚNICA forma de corrigir o livro. UPDATE está bloqueado no banco, e é
// bloqueado de propósito: a versão errada tem que continuar visível, senão o
// auditor não consegue reconstruir o que a empresa acreditava naquele dia.
func (p *Poster) Reverse(ctx context.Context, txID, reason, actor string) (*Tx, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("%w: estorno exige justificativa", ErrInvalidInput)
	}
	orig, err := p.Get(ctx, txID)
	if err != nil {
		return nil, err
	}
	inverted := make([]Posting, 0, len(orig.Postings))
	for _, pg := range orig.Postings {
		side := Credit
		if pg.Side == Credit {
			side = Debit
		}
		inverted = append(inverted, Posting{
			Account: pg.Account, Side: side, Amount: pg.Amount,
			PaymentMethod: pg.PaymentMethod, SellerID: pg.SellerID,
			Memo: "estorno de " + orig.ID,
		})
	}
	// occurred_at é AGORA, não a data do original: estorno de um lançamento de
	// mês fechado tem que cair no mês aberto. Datar retroativamente furaria o
	// fechamento e é exatamente o que o trigger de período impede.
	return p.Post(ctx, TxInput{
		OccurredAt:  time.Now().UTC(),
		Kind:        KindReversal,
		SourceType:  SourceLedgerTx,
		SourceID:    orig.ID,
		Description: "Estorno de " + string(orig.Kind) + ": " + reason,
		Currency:    orig.Currency,
		RequestID:   requestid.FromContext(ctx),
		CreatedBy:   actor,
		ReversesID:  orig.ID,
		Postings:    inverted,
	})
}

// ErrNotFound — lançamento inexistente.
var ErrNotFound = errors.New("ledger: lançamento não encontrado")

// Get carrega um lançamento com suas partidas.
func (p *Poster) Get(ctx context.Context, txID string) (*Tx, error) {
	var t Tx
	var period time.Time
	var reverses sql.NullString
	err := p.db.QueryRowContext(ctx, `
		SELECT id, occurred_at, period, kind, source_type, source_id, description,
		       currency, request_id, reverses_id, created_by, created_at
		FROM ledger_transactions WHERE id = $1`, txID).
		Scan(&t.ID, &t.OccurredAt, &period, &t.Kind, &t.SourceType, &t.SourceID,
			&t.Description, &t.Currency, &t.RequestID, &reverses, &t.CreatedBy, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Period = period.Format("2006-01")
	t.ReversesID = reverses.String

	rows, err := p.db.QueryContext(ctx, `
		SELECT account_code, side, amount_cents, payment_method, seller_id, memo
		FROM ledger_entries WHERE transaction_id = $1 ORDER BY id`, txID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pg Posting
		var amt int64
		if err := rows.Scan(&pg.Account, &pg.Side, &amt, &pg.PaymentMethod, &pg.SellerID, &pg.Memo); err != nil {
			return nil, err
		}
		pg.Amount = Cents(amt)
		t.Postings = append(t.Postings, pg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	t.TotalCents = TxInput{Postings: t.Postings}.Total()
	return &t, nil
}

// translatePGError converte as mensagens dos nossos triggers em erros
// tipados. Sem driver-specific: casamos pelo texto que NÓS escrevemos nas
// RAISE EXCEPTION da migration 005.
func translatePGError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "está FECHADO"):
		return fmt.Errorf("%w: %v", ErrPeriodClosed, err)
	case strings.Contains(msg, "NÃO FECHA"):
		return fmt.Errorf("%w: %v", ErrUnbalanced, err)
	case strings.Contains(msg, "ledger_tx_source_unique"):
		return fmt.Errorf("%w: %v", ErrDuplicate, err)
	case strings.Contains(msg, "é imutável"):
		return fmt.Errorf("ledger: tentativa de mutar o livro (bloqueado pelo banco): %w", err)
	}
	return err
}
