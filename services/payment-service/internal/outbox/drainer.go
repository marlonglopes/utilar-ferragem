package outbox

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Drainer struct {
	db     *sql.DB
	kafka  *kgo.Client
	ticker *time.Ticker
}

func NewDrainer(db *sql.DB, brokers []string) (*Drainer, error) {
	kafka, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return nil, err
	}
	return &Drainer{
		db:     db,
		kafka:  kafka,
		ticker: time.NewTicker(2 * time.Second),
	}, nil
}

func (d *Drainer) Run(ctx context.Context) {
	slog.Info("outbox drainer started")
	for {
		select {
		case <-ctx.Done():
			d.ticker.Stop()
			d.kafka.Close()
			slog.Info("outbox drainer stopped")
			return
		case <-d.ticker.C:
			d.drain(ctx)
		}
	}
}

func (d *Drainer) drain(ctx context.Context) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, event_type, payload_json, attempts
		FROM payments_outbox
		WHERE published_at IS NULL
		  AND next_attempt_at <= now()
		ORDER BY created_at
		LIMIT 50
	`)
	if err != nil {
		slog.Error("outbox: query", "error", err)
		return
	}
	defer rows.Close()

	type outboxRow struct {
		id        string
		eventType string
		payload   []byte
		attempts  int
	}

	var pending []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.id, &r.eventType, &r.payload, &r.attempts); err != nil {
			slog.Error("outbox: scan", "error", err)
			continue
		}
		pending = append(pending, r)
	}

	for _, r := range pending {
		record := &kgo.Record{
			Topic: r.eventType,
			Value: r.payload,
		}

		if err := d.kafka.ProduceSync(ctx, record).FirstErr(); err != nil {
			slog.Error("outbox: publish", "event", r.eventType, "error", err)
			// Exponential backoff: 1s → 5s → 30s
			delays := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}
			next := delays[min(r.attempts, len(delays)-1)]
			d.db.ExecContext(ctx, `
				UPDATE payments_outbox
				SET attempts = attempts + 1, next_attempt_at = now() + $1
				WHERE id = $2
			`, next, r.id)
			continue
		}

		d.db.ExecContext(ctx, `
			UPDATE payments_outbox SET published_at = now() WHERE id = $1
		`, r.id)
		slog.Info("outbox: published", "event", r.eventType, "id", r.id)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
