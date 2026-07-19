// Rota de CUSTO PARA O BALCÃO — /api/v1/store.
//
// PORQUÊ existe: o custo de aquisição só saía por
// `GET /api/v1/admin/products/by-id/:id`, atrás de RequireAdmin. O papel
// `store_operator` não alcançava, então o PDV rodava a barra de margem em
// custo ESTIMADO (`preço × 0,72`). Num caso medido o custo real dava 60% de
// margem e a estimativa dava 28% — 32 pontos de diferença, e é esse número que
// o vendedor usa pra decidir desconto.
//
// PORQUÊ em LOTE: o PDV monta um carrinho com vários itens. Uma chamada por
// item viraria N+1 no caminho mais quente do balcão — o cliente esperando na
// fila enquanto o tablet faz 12 requisições.
//
// ⚠️ A regra pública NÃO muda: `cost` continua estruturalmente ausente de
// `model.Product` e da projeção `productColumns`. Esta rota usa uma projeção e
// uma struct próprias (`model.ProductCost`), e vive atrás de RequireStore.
package handler

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/utilar/catalog-service/internal/model"
)

// maxCostBatch — teto de ids por chamada.
//
// 200 é muito acima de qualquer carrinho de balcão real (o maior pedido de
// obra que passou pelo caixa tinha 30 itens) e baixo o suficiente pra que uma
// chamada abusiva não vire varredura do catálogo inteiro item a item.
const maxCostBatch = 200

// uuidRe valida o formato do id ANTES de mandar pro Postgres. Sem isso um id
// torto vira `invalid input syntax for type uuid`, que sai como 500 (erro
// nosso) quando na verdade é 400 (erro de quem chamou).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type StoreCostHandler struct{ db *sql.DB }

func NewStoreCostHandler(db *sql.DB) *StoreCostHandler { return &StoreCostHandler{db: db} }

// costsRequest é o corpo do POST. Existe além do GET porque um carrinho grande
// estoura o tamanho prático de query string (200 UUIDs ≈ 7,4 KB), e proxy que
// trunca URL falharia de um jeito silencioso — devolvendo custo de PARTE do
// carrinho, que é pior que não devolver nenhum.
type costsRequest struct {
	IDs []string `json:"ids"`
}

// Costs GET /api/v1/store/products/costs?ids=uuid1,uuid2
func (h *StoreCostHandler) Costs(c *gin.Context) {
	raw := c.Query("ids")
	if strings.TrimSpace(raw) == "" {
		BadRequest(c, "ids is required (comma-separated product uuids)")
		return
	}
	h.respond(c, strings.Split(raw, ","))
}

// CostsBatch POST /api/v1/store/products/costs  {"ids":[...]}
func (h *StoreCostHandler) CostsBatch(c *gin.Context) {
	var in costsRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if len(in.IDs) == 0 {
		BadRequest(c, "ids is required")
		return
	}
	h.respond(c, in.IDs)
}

// respond é o caminho comum do GET e do POST: normaliza, valida, consulta uma
// única vez e monta a resposta com os ausentes explicitados.
func (h *StoreCostHandler) respond(c *gin.Context, rawIDs []string) {
	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !uuidRe.MatchString(id) {
			BadRequest(c, "invalid product id: "+id)
			return
		}
		// Deduplica: o mesmo produto pode estar em duas linhas do carrinho
		// (quantidades diferentes negociadas). Consultar duas vezes só
		// desperdiçaria banco e duplicaria a linha na resposta.
		lower := strings.ToLower(id)
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		BadRequest(c, "ids is required")
		return
	}
	if len(ids) > maxCostBatch {
		BadRequest(c, "too many ids (max 200)")
		return
	}

	// SEM filtro de `status='published'`: o operador tem o item na prateleira
	// mesmo quando ele está em rascunho na vitrine. O `status` volta no payload
	// pra que o PDV avise, em vez de o servidor decidir por ele.
	rows, err := h.db.Query(`
		SELECT p.id, p.sku, p.name, p.price, p.currency, p.cost, p.unit_of_measure, p.status
		FROM products p
		WHERE p.id = ANY($1)
	`, pq.Array(ids))
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.ProductCost, 0, len(ids))
	found := make(map[string]struct{}, len(ids))
	for rows.Next() {
		var pc model.ProductCost
		if err := rows.Scan(&pc.ID, &pc.SKU, &pc.Name, &pc.Price, &pc.Currency,
			&pc.Cost, &pc.UnitOfMeasure, &pc.Status); err != nil {
			DBError(c, err)
			return
		}
		pc.MarginPct = marginPct(pc.Price, pc.Cost)
		found[strings.ToLower(pc.ID)] = struct{}{}
		out = append(out, pc)
	}
	// rows.Err() distingue "esses ids não existem" de "a varredura morreu no
	// meio". Sem a checagem, uma leitura truncada chegaria ao PDV como se
	// fosse a resposta completa e o vendedor negociaria margem de meio carrinho.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	missing := make([]string, 0)
	for _, id := range ids {
		if _, ok := found[strings.ToLower(id)]; !ok {
			missing = append(missing, id)
		}
	}

	// Sem cache HTTP: custo é dado sensível e volátil. Um proxy intermediário
	// guardando esta resposta serviria custo a quem não pediu.
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.ProductCostsResponse{Data: out, Missing: missing})
}

// marginPct é a margem sobre o PREÇO DE VENDA ((preço − custo) / preço), em
// pontos percentuais.
//
// PORQUÊ é função compartilhada entre a rota admin e a de balcão: se cada rota
// calculasse a sua, um ajuste em uma faria o gerente e o vendedor olharem
// números diferentes do mesmo produto — e o desconto sairia do número errado.
//
// Preço zero devolve nil em vez de dividir por zero: item de brinde não tem
// margem definida, e um `+Inf` no JSON quebraria o parser do PDV.
func marginPct(price float64, cost *float64) *float64 {
	if cost == nil || price <= 0 {
		return nil
	}
	m := (price - *cost) / price * 100
	return &m
}
