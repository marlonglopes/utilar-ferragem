package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// AuthEventType — espelha o ENUM auth_event_type do DB.
type AuthEventType string

const (
	EventRegister               AuthEventType = "register"
	EventLoginSuccess           AuthEventType = "login_success"
	EventLoginFailure           AuthEventType = "login_failure"
	EventLogout                 AuthEventType = "logout"
	EventPasswordResetRequested AuthEventType = "password_reset_requested"
	EventPasswordChanged        AuthEventType = "password_changed"
	EventEmailVerified          AuthEventType = "email_verified"
)

// logAuthEvent insere uma linha em auth_events. Falla aberto: erros viram
// log warn em vez de derrubar o handler — auditoria nunca pode quebrar
// a feature operacional. L-AUTH-1.
//
// userID pode ser vazio (ex: login_failure pra email não cadastrado).
// metadata é opcional; se nil, vai NULL.
func logAuthEvent(ctx context.Context, db *sql.DB, c *gin.Context, eventType AuthEventType, userID string, metadata map[string]any) {
	// STRING, não []byte: lib/pq serializa []byte como bytea em hexadecimal e o
	// Postgres recusa isso na coluna jsonb ("invalid input syntax for type
	// json"). Como esta função falha aberto, o sintoma era silencioso — os
	// eventos COM metadata (login_failure, que é justamente o que se olha num
	// incidente) sumiam com um warn no log.
	var metaJSON any
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = string(b)
		}
	}

	var userIDArg any
	if userID != "" {
		userIDArg = userID
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO auth_events (event_type, user_id, ip, user_agent, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`, string(eventType), userIDArg, c.ClientIP(), c.GetHeader("User-Agent"), metaJSON)
	if err != nil {
		slog.Warn("audit event insert failed",
			"event_type", string(eventType),
			"error", err,
			"request_id", c.GetString("request_id"),
		)
	}
}
