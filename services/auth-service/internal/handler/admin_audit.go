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

// AuditHandler expõe a LEITURA da trilha de administração de staff
// (store_audit_events): criação/edição de operador, teto de desconto e MUDANÇA
// DE PAPEL (quem promoveu quem a contador/vendas/almoxarife/admin). A gravação
// já existia (logStoreEvent); faltava ler — as linhas ficavam invisíveis fora
// do banco.
//
// Devolve o MESMO shape do catalog (`/admin/audit`) para a tela unificada de
// atividade poder juntar as fontes sem tradutor por serviço. Ver
// docs/backoffice-personas.md e a trilha unificada no painel.
type AuditHandler struct{ db *sql.DB }

func NewAuditHandler(db *sql.DB) *AuditHandler { return &AuditHandler{db: db} }

type auditRow struct {
	ID        string          `json:"id"`
	ActorID   *string         `json:"actorId"`
	ActorRole *string         `json:"actorRole"` // auth não grava o papel do ator aqui → sempre null
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *string         `json:"entityId"`
	Changes   json.RawMessage `json:"changes"`
	RequestID *string         `json:"requestId"`
	CreatedAt string          `json:"createdAt"`
}

// AuditList GET /api/v1/admin/audit?action=&entity=&entityId=&actor=&page=&per_page=
//
// Só admin (grupo de rota). É a trilha de QUEM MEXEU EM STAFF — inclui a
// atribuição de papel, que decide quem vê custo e quem comanda a loja. Não
// vaza custo de produto: esta tabela é sobre operadores/papéis, não catálogo.
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
	// `entity` é derivado do prefixo da ação (operator.created → operator;
	// user.role.update → user). Filtrar por entidade = filtrar o prefixo.
	if e := strings.TrimSpace(c.Query("entity")); e != "" {
		args = append(args, e+".%")
		where += fmt.Sprintf(" AND action LIKE $%d", len(args))
	}
	if eid := strings.TrimSpace(c.Query("entityId")); eid != "" {
		args = append(args, eid)
		where += fmt.Sprintf(" AND target_id::text = $%d", len(args))
	}
	if ac := strings.TrimSpace(c.Query("actor")); ac != "" {
		args = append(args, "%"+ac+"%")
		where += fmt.Sprintf(" AND actor_id::text ILIKE $%d", len(args))
	}

	countArgs := append([]any{}, args...)
	offset := (page - 1) * perPage
	limitPH, offsetPH := len(args)+1, len(args)+2
	args = append(args, perPage, offset)

	rows, err := h.db.Query(fmt.Sprintf(`
		SELECT id, actor_id, action, target_id, old_value, new_value, request_id, created_at
		FROM store_audit_events
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
		var oldRaw, newRaw []byte
		var created time.Time
		if err := rows.Scan(&r.ID, &r.ActorID, &r.Action, &r.EntityID, &oldRaw, &newRaw, &r.RequestID, &created); err != nil {
			DBError(c, err)
			return
		}
		r.Entity = entityFromAction(r.Action)
		r.Changes = diffChanges(oldRaw, newRaw)
		r.CreatedAt = created.Format(time.RFC3339)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var total int
	if err := h.db.QueryRow("SELECT count(*) FROM store_audit_events WHERE "+where, countArgs...).Scan(&total); err != nil {
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

// entityFromAction extrai a entidade do prefixo da ação: "user.role.update" →
// "user", "operator.created" → "operator". Sem ponto, a ação inteira é a entidade.
func entityFromAction(action string) string {
	if i := strings.IndexByte(action, '.'); i > 0 {
		return action[:i]
	}
	return action
}

// diffChanges transforma old_value/new_value (objetos JSON inteiros) no MESMO
// formato campo→{old,new} do catalog, para a tela unificada renderizar igual.
// Objetos ausentes/ilegíveis viram um diff vazio, não um erro (auditoria não
// derruba leitura).
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
