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

// AuditHandler expõe a LEITURA da trilha do catálogo (catalog_audit_log) para o
// painel — "quem fez o quê, quando" nos produtos. A gravação já existe (audit.go);
// o que faltava era ler: as linhas estavam invisíveis fora do banco.
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
	RequestID *string         `json:"requestId"`
	CreatedAt string          `json:"createdAt"`
}

// AuditList GET /api/v1/admin/audit?action=&entity=&entityId=&actor=&page=&per_page=
//
// Só admin (grupo de rota). Filtra por ação, entidade, id da entidade e ator.
// Ordena do mais recente. NÃO expõe custo — as `changes` já vêm da trilha, que
// pode conter custo em `product.update`; por isso este endpoint é admin-only,
// como o resto da leitura sensível.
func (h *AuditHandler) AuditList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	where := "1=1"
	var args []any
	if a := strings.TrimSpace(c.Query("action")); a != "" {
		args = append(args, a)
		where += fmt.Sprintf(" AND action = $%d", len(args))
	}
	if e := strings.TrimSpace(c.Query("entity")); e != "" {
		args = append(args, e)
		where += fmt.Sprintf(" AND entity = $%d", len(args))
	}
	if eid := strings.TrimSpace(c.Query("entityId")); eid != "" {
		args = append(args, eid)
		where += fmt.Sprintf(" AND entity_id = $%d", len(args))
	}
	if ac := strings.TrimSpace(c.Query("actor")); ac != "" {
		args = append(args, "%"+ac+"%")
		where += fmt.Sprintf(" AND actor_id ILIKE $%d", len(args))
	}

	countArgs := append([]any{}, args...)
	offset := (page - 1) * perPage
	limitPH, offsetPH := len(args)+1, len(args)+2
	args = append(args, perPage, offset)

	rows, err := h.db.Query(fmt.Sprintf(`
		SELECT id, actor_id, actor_role, action, entity, entity_id, changes, request_id, created_at
		FROM catalog_audit_log
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, limitPH, offsetPH), args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]auditRow, 0, perPage)
	for rows.Next() {
		var r auditRow
		var changes []byte
		var created time.Time
		if err := rows.Scan(&r.ID, &r.ActorID, &r.ActorRole, &r.Action, &r.Entity,
			&r.EntityID, &changes, &r.RequestID, &created); err != nil {
			DBError(c, err)
			return
		}
		r.Changes = json.RawMessage(changes)
		r.CreatedAt = created.Format(time.RFC3339)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var total int
	if err := h.db.QueryRow("SELECT count(*) FROM catalog_audit_log WHERE "+where, countArgs...).Scan(&total); err != nil {
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
