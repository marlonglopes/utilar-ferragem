// API contábil — todas as rotas exigem role=admin (ver AdminOnly).
// O contrato completo está em docs/ledger-api.md; o dashboard depende dele.
package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/ledger"
	"github.com/utilar/pkg/audit"
)

type LedgerHandler struct {
	reports    *ledger.Reports
	poster     *ledger.Poster
	closer     *ledger.Closer
	reconciler *ledger.Reconciler
	audit      *audit.Recorder
}

func NewLedgerHandler(
	db *sql.DB,
	poster *ledger.Poster,
	closer *ledger.Closer,
	rec *ledger.Reconciler,
	aud *audit.Recorder,
) *LedgerHandler {
	return &LedgerHandler{
		reports: ledger.NewReports(db), poster: poster, closer: closer,
		reconciler: rec, audit: aud,
	}
}

// maxWindowDays limita a janela de qualquer relatório.
//
// Não é só performance: uma query de 5 anos segura uma conexão do pool por
// minutos e um punhado delas derruba o checkout, que divide o mesmo *sql.DB.
// Relatório longo sai por export, não por endpoint de dashboard.
const maxWindowDays = 400

// parseWindow lê ?from=&to= (RFC3339 ou YYYY-MM-DD) ou ?period=YYYY-MM.
// Janela é SEMPRE [from, to) — `to` exclusivo.
func parseWindow(c *gin.Context) (time.Time, time.Time, error) {
	if p := c.Query("period"); p != "" {
		return ledger.Period(p).Range()
	}
	from, err := parseDate(c.Query("from"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parâmetro `from` inválido: %w", err)
	}
	to, err := parseDate(c.Query("to"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parâmetro `to` inválido: %w", err)
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, errors.New("janela vazia: `to` precisa ser maior que `from` (janela é [from, to), com `to` exclusivo)")
	}
	if to.Sub(from) > maxWindowDays*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("janela maior que %d dias — use a exportação CSV", maxWindowDays)
	}
	return from, to, nil
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("obrigatório (use `period=YYYY-MM` ou `from`+`to`)")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return time.Time{}, errors.New("formato esperado: YYYY-MM-DD ou RFC3339")
	}
	return t, nil
}

// GET /api/v1/ledger/summary
func (h *LedgerHandler) Summary(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	s, err := h.reports.Summary(c.Request.Context(), from, to)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, s)
}

// GET /api/v1/ledger/by-method
func (h *LedgerHandler) ByMethod(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	out, err := h.reports.ByMethod(c.Request.Context(), from, to)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "methods": out})
}

// GET /api/v1/ledger/daily
func (h *LedgerHandler) Daily(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	out, err := h.reports.Daily(c.Request.Context(), from, to)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "points": out})
}

// GET /api/v1/ledger/trial-balance
func (h *LedgerHandler) TrialBalance(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	tb, err := h.reports.TrialBalance(c.Request.Context(), from, to)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, tb)
}

// GET /api/v1/ledger/entries
func (h *LedgerHandler) Entries(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	out, err := h.reports.Entries(c.Request.Context(), from, to, limit)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "entries": out})
}

// GET /api/v1/ledger/transactions/:id
func (h *LedgerHandler) GetTransaction(c *gin.Context) {
	tx, err := h.poster.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, ledger.ErrNotFound) {
		NotFound(c, "transaction not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, tx)
}

// POST /api/v1/ledger/transactions/:id/reverse
//
// Única forma de corrigir o livro. Exige justificativa: sem ela, o razão fica
// com um estorno órfão que ninguém consegue explicar seis meses depois.
func (h *LedgerHandler) Reverse(c *gin.Context) {
	var body struct {
		Reason string `json:"reason" binding:"required,min=10"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, "reason é obrigatório (mínimo 10 caracteres explicando a correção)")
		return
	}
	tx, err := h.poster.Reverse(c.Request.Context(), c.Param("id"), body.Reason, c.GetString("user_id"))
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		NotFound(c, "transaction not found")
		return
	case errors.Is(err, ledger.ErrDuplicate):
		Respond(c, http.StatusConflict, "conflict", "este lançamento já foi estornado")
		return
	case err != nil:
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, tx)
}

// GET /api/v1/ledger/periods
func (h *LedgerHandler) ListPeriods(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	out, err := h.closer.List(c.Request.Context(), limit)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"periods": out})
}

// GET /api/v1/ledger/periods/:period
func (h *LedgerHandler) GetPeriod(c *gin.Context) {
	st, err := h.closer.Status(c.Request.Context(), ledger.Period(c.Param("period")))
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, st)
}

// POST /api/v1/ledger/periods/:period/close
func (h *LedgerHandler) ClosePeriod(c *gin.Context) {
	st, err := h.closer.Close(c.Request.Context(), ledger.Period(c.Param("period")),
		c.GetString("user_id"), c.ClientIP(), c.GetString("request_id"))
	switch {
	case errors.Is(err, ledger.ErrAlreadyClosed):
		Respond(c, http.StatusConflict, "conflict", err.Error())
		return
	case errors.Is(err, ledger.ErrPeriodInFuture), errors.Is(err, ledger.ErrUnbalanced):
		BadRequest(c, err.Error())
		return
	case err != nil:
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

// POST /api/v1/ledger/reconcile
//
// Síncrono de propósito: o operador clica e vê o resultado. A janela é limitada
// e a rotina só lê — não há caminho de correção automática (ver reconcile.go).
func (h *LedgerHandler) Reconcile(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	if h.reconciler == nil {
		Respond(c, http.StatusServiceUnavailable, "unavailable", "reconciliação não configurada")
		return
	}
	res, err := h.reconciler.Run(c.Request.Context(), from, to, c.GetString("request_id"))
	if err != nil {
		DBError(c, err)
		return
	}
	// 200 mesmo com divergências: a rotina RODOU. Divergência é o resultado,
	// não uma falha da chamada — quem alerta é a métrica, não o HTTP status.
	c.JSON(http.StatusOK, res)
}

// GET /api/v1/ledger/discrepancies
func (h *LedgerHandler) Discrepancies(c *gin.Context) {
	if h.reconciler == nil {
		c.JSON(http.StatusOK, gin.H{"discrepancies": []any{}})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	out, err := h.reconciler.OpenDiscrepancies(c.Request.Context(), limit)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"discrepancies": out})
}

// POST /api/v1/ledger/discrepancies/:id/resolve
func (h *LedgerHandler) ResolveDiscrepancy(c *gin.Context) {
	var body struct {
		Note string `json:"note" binding:"required,min=10"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, "note é obrigatória (mínimo 10 caracteres com a conclusão da apuração)")
		return
	}
	if h.reconciler == nil {
		Respond(c, http.StatusServiceUnavailable, "unavailable", "reconciliação não configurada")
		return
	}
	if err := h.reconciler.Resolve(c.Request.Context(), c.Param("id"), c.GetString("user_id"), body.Note); err != nil {
		BadRequest(c, err.Error())
		return
	}
	// Resolver uma divergência de dinheiro é decisão humana com consequência
	// financeira — vai pra trilha imutável, com quem, quando e por quê.
	if h.audit != nil {
		_, _ = h.audit.Record(c.Request.Context(), audit.Entry{
			ActorID: c.GetString("user_id"), ActorRole: c.GetString("user_role"),
			ActorIP: c.ClientIP(), ActorUserAgent: c.Request.UserAgent(),
			EntityType: "reconciliation_discrepancy", EntityID: c.Param("id"),
			Action:    audit.ActionApprove,
			NewValue:  map[string]any{"resolved": true, "note": body.Note},
			RequestID: c.GetString("request_id"),
		})
	}
	c.Status(http.StatusNoContent)
}

// GET /api/v1/ledger/export?format=csv|balancete|ofx
//
// A exportação em si é um evento auditável: leva o faturamento inteiro pra fora
// do sistema. Registrar quem exportou o quê é exigência básica de LGPD e a
// primeira pergunta de qualquer investigação de vazamento.
func (h *LedgerHandler) Export(c *gin.Context) {
	from, to, err := parseWindow(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	format := c.DefaultQuery("format", "csv")
	ctx := c.Request.Context()

	if h.audit != nil {
		_, _ = h.audit.Record(ctx, audit.Entry{
			ActorID: c.GetString("user_id"), ActorRole: c.GetString("user_role"),
			ActorIP: c.ClientIP(), ActorUserAgent: c.Request.UserAgent(),
			EntityType: "ledger_export", EntityID: format,
			Action:    audit.ActionExport,
			NewValue:  map[string]any{"format": format, "from": from, "to": to},
			RequestID: c.GetString("request_id"),
		})
	}

	stamp := from.Format("20060102") + "-" + to.Format("20060102")
	switch format {
	case "csv", "razao":
		attach(c, "utilar-razao-"+stamp+".csv", "text/csv; charset=utf-8")
		err = h.reports.ExportRazaoCSV(ctx, c.Writer, from, to)
	case "balancete":
		attach(c, "utilar-balancete-"+stamp+".csv", "text/csv; charset=utf-8")
		err = h.reports.ExportBalanceteCSV(ctx, c.Writer, from, to)
	case "ofx":
		attach(c, "utilar-extrato-"+stamp+".ofx", "application/x-ofx")
		err = h.reports.ExportOFX(ctx, c.Writer, from, to, ledger.OFXOptions{})
	default:
		BadRequest(c, "format inválido (csv | balancete | ofx)")
		return
	}
	if err != nil {
		// O header já foi enviado; não dá pra trocar o status. Loga e corta.
		DBError(c, err)
	}
}

func attach(c *gin.Context, filename, contentType string) {
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// Relatório financeiro nunca em cache compartilhado.
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
}

// GET /api/v1/ledger/audit/verify
//
// Reprocessa a cadeia inteira e diz se ela é íntegra. É o endpoint que o
// auditor externo pede pra rodar na frente dele.
func (h *LedgerHandler) VerifyAudit(c *gin.Context) {
	if h.audit == nil {
		Respond(c, http.StatusServiceUnavailable, "unavailable", "trilha de auditoria não configurada")
		return
	}
	ctx := c.Request.Context()
	seq, head, err := h.audit.Head(ctx)
	if err != nil {
		DBError(c, err)
		return
	}
	if verr := h.audit.VerifyAll(ctx); verr != nil {
		var ce *audit.ChainError
		out := gin.H{"valid": false, "headSeq": seq, "headHash": head, "error": verr.Error()}
		if errors.As(verr, &ce) {
			out["brokenAtSeq"] = ce.Seq
			out["kind"] = ce.Kind
		}
		// 200 com valid:false: a VERIFICAÇÃO funcionou. 5xx aqui faria o
		// dashboard mostrar "erro de rede" no exato momento em que precisa
		// gritar "a trilha foi adulterada".
		c.JSON(http.StatusOK, out)
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "headSeq": seq, "headHash": head})
}

// GET /api/v1/ledger/audit
func (h *LedgerHandler) ListAudit(c *gin.Context) {
	if h.audit == nil {
		c.JSON(http.StatusOK, gin.H{"records": []any{}})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	fromSeq, _ := strconv.ParseInt(c.DefaultQuery("fromSeq", "0"), 10, 64)
	recs, err := h.audit.List(c.Request.Context(), audit.ListFilter{
		EntityType: c.Query("entityType"),
		EntityID:   c.Query("entityId"),
		ActorID:    c.Query("actorId"),
		FromSeq:    fromSeq,
		Limit:      limit,
	})
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": recs})
}
