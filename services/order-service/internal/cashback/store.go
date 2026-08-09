package cashback

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// rowQ / execQ são satisfeitos tanto por *sql.DB quanto por *sql.Tx — as funções
// de escrita rodam DENTRO de transações já abertas (consumer de pagamento, Create
// do pedido); as de leitura, no *sql.DB.
type rowQ interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
type execQ interface {
	rowQ
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Entry é uma linha do histórico (o que /conta/cashback mostra).
type Entry struct {
	Kind      string    `json:"kind"`   // earn | redeem | reverse | expire
	Amount    float64   `json:"amount"` // assinado: earn +, resto −
	OrderID   string    `json:"orderId,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// LoadConfig lê o singleton. Ausente (não deveria, a migration semeia) → devolve
// um config DESLIGADO, que é o padrão seguro (não acumula nem deixa resgatar).
func LoadConfig(ctx context.Context, q rowQ) (Config, error) {
	var c Config
	err := q.QueryRowContext(ctx, `
		SELECT active, earn_rate_pct, redeem_max_pct, validity_days
		FROM cashback_config WHERE id = 1`).
		Scan(&c.Active, &c.EarnRatePct, &c.RedeemMaxPct, &c.ValidityDays)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, nil
	}
	return c, err
}

// SaveConfig grava o singleton (admin). updatedBy = quem mudou (auditoria leve).
func SaveConfig(ctx context.Context, q execQ, c Config, updatedBy string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE cashback_config SET
			active=$1, earn_rate_pct=$2, redeem_max_pct=$3, validity_days=$4,
			updated_at=now(), updated_by=$5
		WHERE id = 1`,
		c.Active, c.EarnRatePct, c.RedeemMaxPct, c.ValidityDays, updatedBy)
	return err
}

// BalanceFor é o saldo disponível: soma do `remaining` dos lotes VIVOS (não
// vencidos). A validade é aplicada aqui, na leitura — cashback vencido nunca
// entra no saldo, mesmo antes de o sweeper marcar o histórico.
func BalanceFor(ctx context.Context, q rowQ, customerID string) (float64, error) {
	var bal float64
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(remaining), 0) FROM cashback_lots
		WHERE customer_id = $1 AND remaining > 0 AND expires_at > now()`, customerID).
		Scan(&bal)
	return bal, err
}

// CreditEarn credita um lote de cashback pelo pedido pago. Idempotente: um lote
// por pedido (UNIQUE em order_id); replay do evento não credita de novo. Devolve
// quanto foi creditado (0 se nada/replay). basis = mercadoria paga (sem frete).
func CreditEarn(ctx context.Context, tx execQ, cfg Config, customerID, orderID string, basis float64) (float64, error) {
	amount := Earn(cfg, basis)
	if amount <= 0 {
		return 0, nil
	}
	expires := time.Now().Add(time.Duration(cfg.ValidityDays) * 24 * time.Hour)
	var id string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO cashback_lots (customer_id, order_id, earned, remaining, expires_at)
		VALUES ($1, $2, $3, $3, $4)
		ON CONFLICT (order_id) DO NOTHING
		RETURNING id`, customerID, orderID, amount, expires).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil // replay: lote do pedido já existe
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cashback_entries (customer_id, order_id, kind, amount, note)
		VALUES ($1, $2, 'earn', $3, 'cashback do pedido')`, customerID, orderID, amount); err != nil {
		return 0, err
	}
	return amount, nil
}

// Redeem consome cashback FIFO (vence primeiro, gasta primeiro) até `amount`,
// travando os lotes (FOR UPDATE) pra não haver corrida com outro resgate. Devolve
// o quanto REALMENTE foi consumido — pode ser menor que `amount` se o saldo caiu
// entre a leitura e aqui; o chamador usa esse valor como o desconto de verdade
// (fecha o TOCTOU). `amount` já vem limitado pelo ClampRedeem.
func Redeem(ctx context.Context, tx execQ, customerID, orderID string, amount float64) (float64, error) {
	if amount <= 0 {
		return 0, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining FROM cashback_lots
		WHERE customer_id = $1 AND remaining > 0 AND expires_at > now()
		ORDER BY expires_at ASC
		FOR UPDATE`, customerID)
	if err != nil {
		return 0, err
	}
	type lot struct {
		id        string
		remaining float64
	}
	var lots []lot
	for rows.Next() {
		var l lot
		if err := rows.Scan(&l.id, &l.remaining); err != nil {
			rows.Close()
			return 0, err
		}
		lots = append(lots, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	left := amount
	consumed := 0.0
	for _, l := range lots {
		if left <= 0 {
			break
		}
		take := l.remaining
		if take > left {
			take = left
		}
		take = round2(take)
		if _, err := tx.ExecContext(ctx,
			`UPDATE cashback_lots SET remaining = remaining - $2 WHERE id = $1`, l.id, take); err != nil {
			return 0, err
		}
		left = round2(left - take)
		consumed = round2(consumed + take)
	}
	if consumed <= 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cashback_entries (customer_id, order_id, kind, amount, note)
		VALUES ($1, $2, 'redeem', $3, 'resgate no checkout')`, customerID, orderID, -consumed); err != nil {
		return 0, err
	}
	return consumed, nil
}

// Reverse desfaz o acúmulo de um pedido (devolução/estorno): zera o que SOBROU do
// lote (só dá pra estornar o que o cliente ainda não gastou) e registra no
// histórico. Idempotente: sem lote ou lote já zerado → no-op.
func Reverse(ctx context.Context, tx execQ, orderID string) (float64, error) {
	var customerID string
	var remaining float64
	err := tx.QueryRowContext(ctx, `
		SELECT customer_id, remaining FROM cashback_lots WHERE order_id = $1 FOR UPDATE`, orderID).
		Scan(&customerID, &remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if remaining <= 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE cashback_lots SET remaining = 0 WHERE order_id = $1`, orderID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cashback_entries (customer_id, order_id, kind, amount, note)
		VALUES ($1, $2, 'reverse', $3, 'estorno por devolução')`, customerID, orderID, -remaining); err != nil {
		return 0, err
	}
	return remaining, nil
}

// History devolve as últimas entradas do cliente (o extrato de /conta/cashback).
func History(ctx context.Context, q execQ, customerID string, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := q.QueryContext(ctx, `
		SELECT kind, amount, COALESCE(order_id, ''), note, created_at
		FROM cashback_entries WHERE customer_id = $1
		ORDER BY created_at DESC LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Kind, &e.Amount, &e.OrderID, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
