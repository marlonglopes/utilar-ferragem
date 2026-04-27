package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorEnvelope struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"requestId,omitempty"`
}

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

func BadRequest(c *gin.Context, msg string)    { Respond(c, http.StatusBadRequest, "bad_request", msg) }
func Unauthorized(c *gin.Context, msg string)  { Respond(c, http.StatusUnauthorized, "unauthorized", msg) }
func Forbidden(c *gin.Context, msg string)     { Respond(c, http.StatusForbidden, "forbidden", msg) }
func NotFound(c *gin.Context, msg string)      { Respond(c, http.StatusNotFound, "not_found", msg) }
func Conflict(c *gin.Context, msg string)      { Respond(c, http.StatusConflict, "conflict", msg) }
func InternalError(c *gin.Context, msg string) { Respond(c, http.StatusInternalServerError, "internal", msg) }
// DBError loga o erro real internamente e responde com mensagem genérica
// pra evitar information disclosure (audit A1-C1). Postgres errors podem vazar
// schema, constraints, queries.
func DBError(c *gin.Context, err error) {
	slog.Error("db.error",
		"request_id", c.GetString("request_id"),
		"path", c.FullPath(),
		"error", err.Error(),
	)
	Respond(c, http.StatusInternalServerError, "db_error", "database error")
}
