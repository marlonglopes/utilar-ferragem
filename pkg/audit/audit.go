// Package audit implementa uma trilha de auditoria append-only encadeada por
// hash, compartilhada pelos 5 serviços do Utilar via go.work.
//
// # O que é uma trilha de auditoria de verdade
//
// Log estruturado NÃO é trilha de auditoria: log é best-effort, roda fora da
// transação e some no rotation. Trilha de auditoria é:
//
//  1. APPEND-ONLY de verdade — garantido no banco por trigger que bloqueia
//     UPDATE/DELETE/TRUNCATE, não por convenção de código (ver Schema()).
//  2. ENCADEADA — cada registro carrega o hash do anterior. Adulterar o
//     registro N obriga a recomputar N+1..fim; se alguém mexeu no meio sem
//     recomputar, VerifyChain aponta exatamente onde.
//  3. TRANSACIONAL — Record() aceita um *sql.Tx, então "mudei a linha" e
//     "registrei que mudei" commitam juntos ou não acontecem.
//  4. SEM DADO SENSÍVEL — senha, token, PAN, CVV nunca entram (ver Scrub).
//
// # Limite honesto do encadeamento
//
// Hash chain detecta adulteração RETROATIVA por quem não recomputa a cadeia.
// NÃO protege contra um DBA com acesso total que apague a tabela inteira ou
// recompute tudo — pra isso é preciso âncora externa (publicar o hash do
// último registro num sistema fora do alcance do mesmo operador). O campo
// Hash do último registro do dia é exatamente o que se publica. Ver
// docs/ledger-api.md.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Action é o verbo do registro. Lista aberta — cada serviço define os seus.
type Action string

const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionLogin   Action = "login"
	ActionAccess  Action = "access"
	ActionApprove Action = "approve"
	ActionReject  Action = "reject"
	ActionExport  Action = "export"
)

// Entry é o que o serviço PREENCHE. O Recorder deriva Seq/PrevHash/Hash.
type Entry struct {
	// Quem
	ActorID   string // user_id do JWT; "system" pra jobs
	ActorRole string // role do JWT ("admin", "customer", "system")
	// ActorIP recebe o IP cru (c.ClientIP()). O Recorder MASCARA para o
	// prefixo de rede antes de hashear e gravar — o IP completo nunca chega ao
	// banco. Ver MaskIP em ip.go para o porquê e para o cuidado com a cadeia.
	ActorIP        string
	ActorUserAgent string // User-Agent (truncado em 512)

	// O quê
	EntityType string // "payment", "order", "ledger_transaction", "user"
	EntityID   string
	Action     Action

	// Valor antigo → novo. Qualquer coisa serializável em JSON.
	// Passe nil quando não se aplica (create não tem "antes").
	OldValue any
	NewValue any

	// Correlação — venha do pkg/requestid (header X-Request-Id).
	RequestID string

	// OccurredAt é opcional; zero = now(). Explícito quando o evento é
	// reprocessado a partir de uma fila e a hora real é a da origem.
	OccurredAt time.Time
}

// Record é a linha persistida, com os campos derivados da cadeia.
type Record struct {
	Seq        int64
	OccurredAt time.Time
	Service    string
	ActorID    string
	ActorRole  string
	// ActorIP em registros novos é o PREFIXO de rede ("203.0.113.0/24"), não um
	// endereço individual. Registros anteriores ao mascaramento ainda trazem o
	// IP completo — o formato é heterogêneo por projeto, e tem que continuar
	// assim: normalizar na leitura invalidaria o hash dos antigos.
	ActorIP        string
	ActorUserAgent string
	EntityType     string
	EntityID       string
	Action         string
	OldValue       json.RawMessage
	NewValue       json.RawMessage
	RequestID      string
	PrevHash       string
	Hash           string
}

// execQuerier cobre *sql.DB e *sql.Tx — é o que permite gravar a auditoria
// DENTRO da transação de negócio.
type execQuerier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// Recorder grava na trilha. Um por serviço.
type Recorder struct {
	db      *sql.DB
	service string
}

// New cria o Recorder. `service` identifica a origem ("payment-service").
func New(db *sql.DB, service string) *Recorder {
	return &Recorder{db: db, service: service}
}

var (
	// ErrClosedChain é devolvido quando a linha anterior não pôde ser lida.
	ErrClosedChain = errors.New("audit: não foi possível ler o topo da cadeia")
	// ErrEmptyEntry protege contra registro inútil (sem entidade nem ação).
	ErrEmptyEntry = errors.New("audit: entry precisa de EntityType e Action")
)

const maxUserAgent = 512

// Record grava usando uma transação própria.
func (r *Recorder) Record(ctx context.Context, e Entry) (*Record, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("audit: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op após commit
	rec, err := r.RecordTx(ctx, tx, e)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("audit: commit: %w", err)
	}
	return rec, nil
}

// RecordTx grava DENTRO da transação do chamador — é a forma preferida.
//
// Serialização: pega um advisory lock transacional antes de ler o topo da
// cadeia. Sem isso, duas goroutines leriam o mesmo prev_hash e produziriam
// dois registros com o mesmo elo — cadeia bifurcada, verificação impossível.
// O lock cai sozinho no commit/rollback.
func (r *Recorder) RecordTx(ctx context.Context, tx execQuerier, e Entry) (*Record, error) {
	if e.EntityType == "" || e.Action == "" {
		return nil, ErrEmptyEntry
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	// Trunca pra precisão do TIMESTAMPTZ ANTES de hashear e gravar: se o hash
	// for calculado com nanossegundos e o banco guardar microssegundos, todo
	// registro parece adulterado ao ser relido.
	e.OccurredAt = e.OccurredAt.UTC().Truncate(TimePrecision)
	if len(e.ActorUserAgent) > maxUserAgent {
		e.ActorUserAgent = e.ActorUserAgent[:maxUserAgent]
	}
	// LGPD: IP é dado pessoal. Mascaramos AQUI — antes do ComputeHash e antes do
	// INSERT — porque a tabela é append-only: gravar o IP completo é uma decisão
	// irreversível (o trigger recusa UPDATE/DELETE). Este é o ÚNICO ponto onde o
	// IP é transformado; nada no caminho de leitura pode mexer nele, sob pena de
	// invalidar o hash dos registros antigos. Ver ip.go.
	e.ActorIP = MaskIP(e.ActorIP)

	oldJSON, oldPaths := marshalScrubbed(e.OldValue)
	newJSON, newPaths := marshalScrubbed(e.NewValue)
	if n := len(oldPaths) + len(newPaths); n > 0 {
		// Não abortamos: mascarar e gravar > perder a trilha. Mas isso é bug do
		// chamador e tem que aparecer em alerta.
		slog.Error("audit: campo sensível bloqueado no payload da trilha",
			"service", r.service, "entity", e.EntityType, "action", e.Action,
			"fields_old", oldPaths, "fields_new", newPaths, "request_id", e.RequestID)
	}

	// advisory lock por serviço: chaves distintas não bloqueiam entre si, mas
	// a cadeia é global por banco — então usamos uma constante única.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, auditLockKey); err != nil {
		return nil, fmt.Errorf("audit: advisory lock: %w", err)
	}

	var lastSeq int64
	var lastHash string
	err := tx.QueryRowContext(ctx,
		`SELECT seq, hash FROM audit_log ORDER BY seq DESC LIMIT 1`).Scan(&lastSeq, &lastHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, lastHash = 0, GenesisHash
	case err != nil:
		return nil, fmt.Errorf("%w: %v", ErrClosedChain, err)
	}

	rec := Record{
		Seq:            lastSeq + 1,
		OccurredAt:     e.OccurredAt.UTC(),
		Service:        r.service,
		ActorID:        e.ActorID,
		ActorRole:      e.ActorRole,
		ActorIP:        e.ActorIP,
		ActorUserAgent: e.ActorUserAgent,
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		Action:         string(e.Action),
		OldValue:       oldJSON,
		NewValue:       newJSON,
		RequestID:      e.RequestID,
		PrevHash:       lastHash,
	}
	rec.Hash = ComputeHash(rec)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_log (
			seq, occurred_at, service, actor_id, actor_role, actor_ip, actor_user_agent,
			entity_type, entity_id, action, old_value, new_value, request_id, prev_hash, hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		rec.Seq, rec.OccurredAt, rec.Service, rec.ActorID, rec.ActorRole, rec.ActorIP,
		rec.ActorUserAgent, rec.EntityType, rec.EntityID, rec.Action,
		nullJSON(rec.OldValue), nullJSON(rec.NewValue), rec.RequestID, rec.PrevHash, rec.Hash)
	if err != nil {
		return nil, fmt.Errorf("audit: insert: %w", err)
	}
	return &rec, nil
}

// auditLockKey é a chave do pg_advisory_xact_lock que serializa a cadeia.
// Valor arbitrário e fixo — o que importa é ser único no banco.
const auditLockKey int64 = 7_413_902_115

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}

func marshalScrubbed(v any) (json.RawMessage, []string) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		b, _ := json.Marshal(map[string]any{"_unmarshalable": fmt.Sprintf("%T", v)})
		return b, []string{"_unmarshalable"}
	}
	return ScrubJSON(raw)
}

// List devolve registros da trilha em ordem crescente de seq, com filtros
// opcionais. Usado pelas telas de auditoria e pela verificação da cadeia.
type ListFilter struct {
	EntityType string
	EntityID   string
	ActorID    string
	FromSeq    int64
	Limit      int
}

func (r *Recorder) List(ctx context.Context, f ListFilter) ([]Record, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT seq, occurred_at, service, actor_id, actor_role, actor_ip, actor_user_agent,
		       entity_type, entity_id, action,
		       COALESCE(old_value,'null'::jsonb), COALESCE(new_value,'null'::jsonb),
		       request_id, prev_hash, hash
		FROM audit_log
		WHERE seq >= $1
		  AND ($2 = '' OR entity_type = $2)
		  AND ($3 = '' OR entity_id  = $3)
		  AND ($4 = '' OR actor_id   = $4)
		ORDER BY seq ASC
		LIMIT $5`, f.FromSeq, f.EntityType, f.EntityID, f.ActorID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func scanRecords(rows *sql.Rows) ([]Record, error) {
	var out []Record
	for rows.Next() {
		var r Record
		var oldV, newV []byte
		if err := rows.Scan(&r.Seq, &r.OccurredAt, &r.Service, &r.ActorID, &r.ActorRole,
			&r.ActorIP, &r.ActorUserAgent, &r.EntityType, &r.EntityID, &r.Action,
			&oldV, &newV, &r.RequestID, &r.PrevHash, &r.Hash); err != nil {
			return nil, err
		}
		// COALESCE devolve o literal JSON `null` quando a coluna é NULL; o hash
		// foi calculado sobre AUSÊNCIA de valor, então normalizamos de volta.
		if string(oldV) != "null" {
			r.OldValue = oldV
		}
		if string(newV) != "null" {
			r.NewValue = newV
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// VerifyAll relê a cadeia inteira do banco e valida. É O(n) em registros —
// pra tabela grande, rode como job noturno e guarde o último seq verificado.
func (r *Recorder) VerifyAll(ctx context.Context) error {
	const page = 1000
	prev := GenesisHash
	var from int64
	for {
		recs, err := r.List(ctx, ListFilter{FromSeq: from, Limit: page})
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			return nil
		}
		if err := VerifyChain(recs, prev); err != nil {
			return err
		}
		prev = recs[len(recs)-1].Hash
		from = recs[len(recs)-1].Seq + 1
	}
}

// Head devolve seq e hash do topo da cadeia — é o que se publica externamente
// como âncora (ver doc do pacote).
func (r *Recorder) Head(ctx context.Context) (int64, string, error) {
	var seq int64
	var hash string
	err := r.db.QueryRowContext(ctx,
		`SELECT seq, hash FROM audit_log ORDER BY seq DESC LIMIT 1`).Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, GenesisHash, nil
	}
	return seq, hash, err
}
