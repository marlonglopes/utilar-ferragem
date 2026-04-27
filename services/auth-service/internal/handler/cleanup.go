package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// TokenCleanupInterval define a frequência do GC de tokens expirados.
// 1h é razoável: tokens expirados não são exploits ativos (já passaram do TTL),
// mas acumular indefinidamente vira leak de espaço em disco e degradação
// da performance do índice. A14-M5.
const TokenCleanupInterval = 1 * time.Hour

// StartTokenCleanup inicia uma goroutine que periodicamente apaga tokens
// expirados das 3 tabelas de tokens. Cancela quando ctx fecha.
//
// Estratégia: DELETE em loop simples — volume baixo (alguns milhares/dia),
// não vale Lua scripts ou batching. Em prod com volume alto, pode virar
// um job dedicado / pg_cron.
func StartTokenCleanup(ctx context.Context, db *sql.DB) {
	go func() {
		// Roda uma vez no boot (limpa qualquer acúmulo do downtime).
		runTokenCleanup(ctx, db)

		t := time.NewTicker(TokenCleanupInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runTokenCleanup(ctx, db)
			}
		}
	}()
}

// runTokenCleanup faz uma única passada. Loga total apagado.
// Erros são logados mas não derrubam o serviço.
func runTokenCleanup(ctx context.Context, db *sql.DB) {
	cutoff := time.Now()
	queries := []struct {
		name  string
		query string
	}{
		{"refresh_tokens", `DELETE FROM refresh_tokens WHERE expires_at < $1`},
		{"password_reset_tokens", `DELETE FROM password_reset_tokens WHERE expires_at < $1`},
		{"email_verification_tokens", `DELETE FROM email_verification_tokens WHERE expires_at < $1`},
	}
	total := int64(0)
	for _, q := range queries {
		res, err := db.ExecContext(ctx, q.query, cutoff)
		if err != nil {
			slog.Warn("token cleanup failed", "table", q.name, "error", err)
			continue
		}
		n, _ := res.RowsAffected()
		total += n
	}
	if total > 0 {
		slog.Info("token cleanup", "rows_deleted", total)
	}
}
