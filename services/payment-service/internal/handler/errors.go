package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorEnvelope — shape compartilhado entre todos os serviços da plataforma.
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
func NotFound(c *gin.Context, msg string)      { Respond(c, http.StatusNotFound, "not_found", msg) }
func InternalError(c *gin.Context, msg string) { Respond(c, http.StatusInternalServerError, "internal", msg) }
func BadGateway(c *gin.Context, msg string)    { Respond(c, http.StatusBadGateway, "bad_gateway", msg) }
func DBError(c *gin.Context, err error) {
	Respond(c, http.StatusInternalServerError, "db_error", err.Error())
}
