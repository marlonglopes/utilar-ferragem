package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorEnvelope é o shape de erro retornado por todos os handlers.
// Compartilhado com payment-service (convenção interna da plataforma).
type ErrorEnvelope struct {
	Error     string `json:"error"`               // mensagem humana curta
	Code      string `json:"code"`                // slug estável: not_found, db_error, bad_request, ...
	RequestID string `json:"requestId,omitempty"` // para cruzar com logs/Sentry
}

// Respond envia um ErrorEnvelope + loga o erro com request_id.
func Respond(c *gin.Context, status int, code, msg string) {
	reqID := c.GetString("request_id")
	slog.Warn("handler.error",
		"request_id", reqID,
		"status", status,
		"code", code,
		"error", msg,
		"path", c.FullPath(),
	)
	c.JSON(status, ErrorEnvelope{Error: msg, Code: code, RequestID: reqID})
}

// Shortcuts para os erros mais comuns.
func BadRequest(c *gin.Context, msg string)     { Respond(c, http.StatusBadRequest, "bad_request", msg) }
func NotFound(c *gin.Context, msg string)       { Respond(c, http.StatusNotFound, "not_found", msg) }
func InternalError(c *gin.Context, msg string)  { Respond(c, http.StatusInternalServerError, "internal", msg) }
// DBError loga internamente, responde genérico (audit CT1-C2).
func DBError(c *gin.Context, err error) {
	slog.Error("db.error",
		"request_id", c.GetString("request_id"),
		"path", c.FullPath(),
		"error", err.Error(),
	)
	Respond(c, http.StatusInternalServerError, "db_error", "database error")
}
