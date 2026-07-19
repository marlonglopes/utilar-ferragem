package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Auditoria de escrita administrativa.
//
// PORQUÊ: sem trilha, "o preço do cimento caiu 60% ontem" é indistinguível de
// operação normal e não tem como reverter. Toda rota que escreve catálogo
// registra ator + ação + valor antigo → novo.
//
// DECISÃO — auditoria NUNCA derruba a operação: se o INSERT no log falhar,
// logamos em ERROR e seguimos. Uma tabela de auditoria com problema não pode
// impedir o lojista de corrigir um preço errado na vitrine. O inverso (perder
// a escrita pra preservar o log) troca um problema de compliance por um
// problema de negócio.
//
// Se `pkg/` ganhar um pacote de auditoria compartilhado, este arquivo vira o
// adaptador dele — mas o registro não depende disso existir.

// FieldChange é o par antigo→novo de um campo.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// AuditChanges mapeia nome do campo → mudança.
type AuditChanges map[string]FieldChange

// execer aceita tanto *sql.DB quanto *sql.Tx — as escritas de import rodam
// dentro de transação por linha e o log deve entrar junto.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// auditActor extrai quem está agindo. Em produção vem do `sub` do JWT; em
// DEV_MODE, do header X-User-Id (pode ser vazio — registramos assim mesmo,
// porque "ator desconhecido em tal request_id" ainda é melhor que nada).
func auditActor(c *gin.Context) (id, role string) {
	return c.GetString("user_id"), c.GetString("user_role")
}

// audit grava um evento. `changes` pode ser nil (ações sem diff, ex: arquivar).
func audit(db execer, c *gin.Context, action, entity, entityID string, changes AuditChanges) {
	actorID, actorRole := auditActor(c)

	payload := []byte("{}")
	if len(changes) > 0 {
		if b, err := json.Marshal(changes); err == nil {
			payload = b
		}
	}

	_, err := db.Exec(`
		INSERT INTO catalog_audit_log (actor_id, actor_role, action, entity, entity_id, changes, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, nullIfEmpty(actorID), nullIfEmpty(actorRole), action, entity, nullIfEmpty(entityID),
		payload, nullIfEmpty(c.GetString("request_id")))

	if err != nil {
		// ERROR e não WARN: perder trilha de auditoria é incidente, mesmo que
		// a operação de negócio tenha dado certo.
		slog.Error("audit.write_failed",
			"action", action, "entity", entity, "entity_id", entityID,
			"actor_id", actorID, "request_id", c.GetString("request_id"),
			"error", err.Error())
	}
}

// recordPriceChange grava a linha de histórico de preço/custo.
//
// Só é chamada quando preço OU custo realmente mudou — gravar um "mudou de
// 42,90 para 42,90" a cada PATCH de descrição enche a tabela de ruído e faz o
// alerta de queda percentual perder o sinal.
func recordPriceChange(db execer, c *gin.Context, productID string, oldPrice, newPrice float64, oldCost, newCost *float64, source string) {
	actorID, _ := auditActor(c)
	_, err := db.Exec(`
		INSERT INTO product_price_history (product_id, price, cost, old_price, old_cost, source, changed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, productID, newPrice, newCost, oldPrice, oldCost, source, nullIfEmpty(actorID))
	if err != nil {
		slog.Error("price_history.write_failed",
			"product_id", productID, "source", source,
			"request_id", c.GetString("request_id"), "error", err.Error())
	}
}

// nullIfEmpty evita gravar string vazia onde NULL é a verdade ("não sei quem
// foi" ≠ "o ator se chama ”"). Facilita `WHERE actor_id IS NULL` na análise.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// changed registra um campo no diff só se o valor de fato mudou.
func (ch AuditChanges) changed(field string, old, new any) {
	if !sameValue(old, new) {
		ch[field] = FieldChange{Old: old, New: new}
	}
}

// sameValue compara via JSON: os valores são de tipos heterogêneos (float64,
// string, *string, nil) e reflect.DeepEqual trataria `*string("a")` e
// `string("a")` como diferentes, poluindo o diff com falsos positivos.
func sameValue(a, b any) bool {
	ja, ea := json.Marshal(a)
	jb, eb := json.Marshal(b)
	if ea != nil || eb != nil {
		return false
	}
	return string(ja) == string(jb)
}

// defaultIfEmpty devolve `fallback` quando `s` é vazio.
//
// Existe para colunas NOT NULL ... DEFAULT: no Postgres o DEFAULT só vale quando
// a coluna é OMITIDA do INSERT. Passar NULL explícito (que é o que nullIfEmpty
// faz) viola a constraint. Para campo opcional no JSON que mapeia em coluna
// NOT NULL, o default tem que ser aplicado aqui, não no banco.
func defaultIfEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
