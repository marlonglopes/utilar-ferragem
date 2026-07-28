package scrape

import (
	"context"
	"database/sql"
	"fmt"
)

// Queue persiste as URLs a processar por fonte, tornando um run RETOMÁVEL e
// permitindo re-scraping incremental por cron. Fica no mesmo Postgres do
// catalog (tabela scrape_queue, migration 017).
type Queue struct{ db *sql.DB }

func NewQueue(db *sql.DB) *Queue { return &Queue{db: db} }

// Enqueue insere URLs como 'pending'. Idempotente: reenfileirar a mesma
// (fonte,url) não duplica nem reprocessa quem já terminou (ON CONFLICT nada faz).
// Devolve quantas linhas NOVAS entraram.
func (q *Queue) Enqueue(ctx context.Context, fonte string, urls []string) (int, error) {
	if len(urls) == 0 {
		return 0, nil
	}
	novas := 0
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback pós-commit é no-op
	for _, u := range urls {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO scrape_queue (fonte, url) VALUES ($1, $2)
			 ON CONFLICT (fonte, url) DO NOTHING`, fonte, u)
		if err != nil {
			return 0, fmt.Errorf("enqueue %s: %w", u, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			novas++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return novas, nil
}

// Claim marca até n URLs 'pending' desta fonte como 'claimed' e as devolve.
// Usa FOR UPDATE SKIP LOCKED para dois workers/execuções não pegarem a mesma
// linha — segurança de concorrência do "pegue o próximo lote".
func (q *Queue) Claim(ctx context.Context, fonte string, n int) ([]string, error) {
	if n <= 0 {
		n = 1
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
		SELECT id, url FROM scrape_queue
		WHERE fonte = $1 AND status = 'pending'
		ORDER BY created_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, fonte, n)
	if err != nil {
		return nil, err
	}
	type row struct {
		id  string
		url string
	}
	var claimed []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.url); err != nil {
			rows.Close()
			return nil, err
		}
		claimed = append(claimed, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	urls := make([]string, 0, len(claimed))
	for _, r := range claimed {
		if _, err := tx.ExecContext(ctx,
			`UPDATE scrape_queue SET status='claimed', attempts=attempts+1, claimed_at=now()
			 WHERE id=$1`, r.id); err != nil {
			return nil, err
		}
		urls = append(urls, r.url)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return urls, nil
}

// Done marca a URL como concluída.
func (q *Queue) Done(ctx context.Context, fonte, url string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE scrape_queue SET status='done', last_error=NULL WHERE fonte=$1 AND url=$2`, fonte, url)
	return err
}

// Fail marca a URL como erro (guarda a mensagem para o relatório/depuração).
func (q *Queue) Fail(ctx context.Context, fonte, url, msg string) error {
	_, err := q.db.ExecContext(ctx,
		`UPDATE scrape_queue SET status='error', last_error=$3 WHERE fonte=$1 AND url=$2`, fonte, url, msg)
	return err
}

// PendingCount diz quantas URLs ainda faltam nesta fonte (0 = fila drenada).
func (q *Queue) PendingCount(ctx context.Context, fonte string) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx,
		`SELECT count(*) FROM scrape_queue WHERE fonte=$1 AND status='pending'`, fonte).Scan(&n)
	return n, err
}
