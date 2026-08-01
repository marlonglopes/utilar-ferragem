package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditHandler expõe a LEITURA da trilha de operação do pedido — as duas
// tabelas de auditoria do order UNIDAS: balcao_audit_events (venda de PDV,
// desconto, aprovação, cancelamento) e return_audit_events (devolução: pedido,
// aprovação, recebimento, ESTORNO). A gravação já existe fail-closed; faltava
// ler para a atividade unificada do painel.
//
// Mesmo shape do catalog/auth (/admin/audit) para a tela juntar as fontes sem
// tradutor. `amount` é opcional (só eventos que mexem dinheiro têm).
type AuditHandler struct{ db *sql.DB }

func NewAuditHandler(db *sql.DB) *AuditHandler { return &AuditHandler{db: db} }

type auditRow struct {
	ID        string          `json:"id"`
	ActorID   *string         `json:"actorId"`
	ActorRole *string         `json:"actorRole"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *string         `json:"entityId"`
	Changes   json.RawMessage `json:"changes"`
	Amount    *float64        `json:"amount,omitempty"`
	RequestID *string         `json:"requestId"`
	CreatedAt string          `json:"createdAt"`
}

// AuditList GET /api/v1/admin/audit?action=&entity=&entityId=&actor=&page=&per_page=
//
// `entityId` filtra pelo id do PEDIDO (order_id) — presente nas duas tabelas —,
// o drill-down natural ("tudo sobre este pedido"). `entity` é derivado do
// prefixo da ação (discount.applied → discount; return.refunded → return).
// Só admin (grupo de rota): a trilha inclui valor de desconto e de estorno.
func (h *AuditHandler) AuditList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	// WHERE compartilhado pelas duas metades do UNION: as duas tabelas têm as
	// MESMAS colunas usadas aqui (action, order_id, actor_id), então os mesmos
	// $N valem nos dois SELECTs.
	where := "1=1"
	var args []any
	if a := strings.TrimSpace(c.Query("action")); a != "" {
		args = append(args, a)
		where += fmt.Sprintf(" AND action = $%d", len(args))
	}
	if e := strings.TrimSpace(c.Query("entity")); e != "" {
		args = append(args, e+".%")
		where += fmt.Sprintf(" AND action LIKE $%d", len(args))
	}
	if eid := strings.TrimSpace(c.Query("entityId")); eid != "" {
		args = append(args, eid)
		where += fmt.Sprintf(" AND order_id::text = $%d", len(args))
	}
	if ac := strings.TrimSpace(c.Query("actor")); ac != "" {
		args = append(args, "%"+ac+"%")
		where += fmt.Sprintf(" AND actor_id ILIKE $%d", len(args))
	}

	// #nosec G201 — `where` é montado só de literais hardcoded com placeholders
	// posicionais ($N); os valores continuam em args. Mesmo padrão do resto do
	// serviço (ver product.go). O único texto interpolado são os índices dos
	// placeholders, calculados aqui.
	unionSelect := func(table string) string {
		return `SELECT id, actor_id, actor_role, action, order_id::text AS entity_id,
		               old_value, new_value, amount, request_id, created_at
		        FROM ` + table + ` WHERE ` + where
	}
	limitPH, offsetPH := len(args)+1, len(args)+2
	query := unionSelect("balcao_audit_events") + " UNION ALL " + unionSelect("return_audit_events") +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", limitPH, offsetPH)

	offset := (page - 1) * perPage
	qArgs := append(append([]any{}, args...), perPage, offset)

	rows, err := h.db.Query(query, qArgs...) // #nosec G201 — ver acima
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]auditRow, 0, perPage)
	for rows.Next() {
		var r auditRow
		var oldRaw, newRaw []byte
		var amount sql.NullFloat64
		var created time.Time
		if err := rows.Scan(&r.ID, &r.ActorID, &r.ActorRole, &r.Action, &r.EntityID,
			&oldRaw, &newRaw, &amount, &r.RequestID, &created); err != nil {
			DBError(c, err)
			return
		}
		r.Entity = entityFromAction(r.Action)
		r.Changes = diffChanges(oldRaw, newRaw)
		if amount.Valid {
			r.Amount = &amount.Float64
		}
		r.CreatedAt = created.Format(time.RFC3339)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	// Total = soma das duas metades sob o mesmo filtro.
	countQ := "SELECT (SELECT count(*) FROM balcao_audit_events WHERE " + where +
		") + (SELECT count(*) FROM return_audit_events WHERE " + where + ")"
	var total int
	if err := h.db.QueryRow(countQ, args...).Scan(&total); err != nil { // #nosec G201 — ver acima
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page": page, "per_page": perPage, "total": total,
			"total_pages": (total + perPage - 1) / perPage,
		},
	})
}

func entityFromAction(action string) string {
	if i := strings.IndexByte(action, '.'); i > 0 {
		return action[:i]
	}
	return action
}

// diffChanges transforma old_value/new_value (objetos JSON inteiros) no formato
// campo→{old,new} do catalog, para a tela unificada renderizar igual. Ausência
// vira diff vazio, não erro.
func diffChanges(oldRaw, newRaw []byte) json.RawMessage {
	var oldM, newM map[string]any
	_ = json.Unmarshal(oldRaw, &oldM)
	_ = json.Unmarshal(newRaw, &newM)
	diff := map[string]map[string]any{}
	for k := range oldM {
		diff[k] = map[string]any{"old": oldM[k], "new": newM[k]}
	}
	for k := range newM {
		if _, seen := diff[k]; !seen {
			diff[k] = map[string]any{"old": oldM[k], "new": newM[k]}
		}
	}
	b, err := json.Marshal(diff)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
