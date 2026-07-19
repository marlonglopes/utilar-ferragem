// Package reservation contém o sweeper que devolve ao estoque as reservas que
// expiraram sem virar pagamento.
//
// PORQUÊ um sweeper e não um TTL no banco: o Postgres não tem expiração de
// linha. Sem alguém varrendo, um carrinho abandonado prenderia estoque pra
// sempre e a loja pararia de vender o produto mais popular.
package reservation

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// Sweeper varre periodicamente reservas vencidas e devolve o saldo.
type Sweeper struct {
	db       *sql.DB
	interval time.Duration
	batch    int
}

// NewSweeper. interval=1min é folgado: a precisão da expiração não precisa ser
// melhor que isso (o TTL padrão é 30min) e o custo do scan é desprezível com o
// índice parcial `idx_stock_reservations_expiry`.
func NewSweeper(db *sql.DB) *Sweeper {
	return &Sweeper{db: db, interval: time.Minute, batch: 500}
}

func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	slog.Info("reservation sweeper started", "interval", s.interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("reservation sweeper stopped")
			return
		case <-ticker.C:
			n, err := s.SweepOnce(ctx)
			if err != nil {
				slog.Error("reservation sweeper", "error", err)
				continue
			}
			if n > 0 {
				slog.Info("reservations expired", "count", n)
			}
		}
	}
}

// SweepOnce libera até `batch` reservas vencidas e devolve quantas liberou.
// Exportado pra ser chamável de teste sem esperar o ticker.
//
// O CTE faz marcação e devolução na MESMA instrução: as linhas devolvidas ao
// estoque são exatamente as que este UPDATE conseguiu virar de 'active' pra
// 'released'. Duas instâncias do serviço rodando o sweeper em paralelo não
// devolvem a mesma reserva duas vezes — a segunda não acha nada 'active'.
func (s *Sweeper) SweepOnce(ctx context.Context) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
		WITH expired AS (
			SELECT id FROM stock_reservations
			WHERE status = 'active' AND expires_at <= now()
			ORDER BY expires_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		),
		freed AS (
			UPDATE stock_reservations r
			SET status = 'released', updated_at = now()
			FROM expired e
			WHERE r.id = e.id
			RETURNING r.product_id, r.quantity
		)
		SELECT product_id, quantity FROM freed
	`, s.batch)
	if err != nil {
		return 0, err
	}

	type freed struct {
		productID string
		qty       int
	}
	var released []freed
	for rows.Next() {
		var f freed
		if err := rows.Scan(&f.productID, &f.qty); err != nil {
			rows.Close()
			return 0, err
		}
		released = append(released, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	for _, f := range released {
		if _, err := tx.ExecContext(ctx,
			`UPDATE products SET stock = stock + $2 WHERE id = $1`,
			f.productID, f.qty,
		); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(released), nil
}
