package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/model"
)

// ============================================================================
// RECOMENDAÇÃO — três fontes, em cascata, e a última se declara
// ============================================================================
//
// O QUE ISTO SUBSTITUI: `mesma categoria ORDER BY rating DESC LIMIT 4`. Numa
// categoria com dezenas de itens, isso devolvia OS MESMOS 4 PRODUTOS em toda
// página de produto da categoria — e ordenados por um `rating` que o seed
// inventou. Lista fixa com nome de recomendação.
//
// A cascata, nesta ordem:
//
//	1. CO-COMPRA (product_copurchase, migration 016) — o sinal mais forte,
//	   porque é comportamento observado e não opinião de quem cadastrou. Só
//	   entra acima de MinCopurchaseOrders pedidos distintos.
//
//	2. COMPLEMENTAR POR REGRA TÉCNICA (product_complement_rules) — funciona no
//	   dia 1, sem histórico nenhum, e é onde mora o conhecimento do balcão
//	   ("porcelanato pede AC-III"). Preenche o que a co-compra não cobriu.
//
//	3. FALLBACK POR CATEGORIA — o que sobrar. **Marcado como fallback no
//	   payload**, item a item e no meta. É a diferença entre "recomendamos" e
//	   "ainda não sabemos, veja outros da categoria": o frontend recebe o
//	   suficiente para não mentir no cabeçalho da seção.
//
// A ORDEM É A FEATURE (mesma postura da cascata de busca em product.go): fundir
// as três fontes numa pontuação única faria a regra técnica competir com a
// estatística em unidades incomparáveis, e o item de fallback — que não é
// recomendação nenhuma — poderia ganhar de uma co-compra real.
// ============================================================================

// MinCopurchaseOrders — mínimo de PEDIDOS DISTINTOS contendo o par para que ele
// possa ser sugerido.
//
// Com o limiar em 1 ou 2, duas pessoas que por acaso levaram cimento e uma
// tomada no mesmo pedido produzem uma "recomendação" — e nas primeiras semanas
// da loja, quando há poucos pedidos, TODA a lista seria feita desse tipo de
// coincidência. 5 é o ponto onde a coincidência fica improvável sem exigir
// volume que só uma loja grande tem; abaixo disso a cascata prefere a regra
// técnica, que é conhecimento de verdade, à estatística de mentira.
const MinCopurchaseOrders = 5

// maxRelatedLimit — teto do `?limit=`.
const maxRelatedLimit = 24

// RecommendationHandler serve GET /products/:slug/related.
type RecommendationHandler struct {
	db *sql.DB
}

func NewRecommendationHandler(db *sql.DB) *RecommendationHandler {
	return &RecommendationHandler{db: db}
}

// Related GET /api/v1/products/:slug/related?limit=8
func (h *RecommendationHandler) Related(c *gin.Context) {
	// Mesmo padding de GetBySlug/Related antigo: normaliza o tempo de resposta
	// para slug existente vs inexistente (CT1-H4).
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	slug := c.Param("slug")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "8"))
	if limit < 1 || limit > maxRelatedLimit {
		limit = 8
	}

	var sourceID, categoryID string
	err := h.db.QueryRow(
		`SELECT id, category_id FROM products WHERE slug = $1 AND status = 'published'`, slug,
	).Scan(&sourceID, &categoryID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// `visto` impede o mesmo produto de aparecer duas vezes quando é ao mesmo
	// tempo co-compra e complemento. Sem isso a lista de 8 pode ter 3 itens.
	visto := map[string]bool{sourceID: true}
	out := make([]model.RelatedProduct, 0, limit)
	var meta model.RelatedMeta
	meta.MinCopurchaseOrders = MinCopurchaseOrders

	// 1. co-compra
	if len(out) < limit {
		items, err := h.copurchase(sourceID, limit-len(out), visto)
		if err != nil {
			DBError(c, err)
			return
		}
		meta.Counts.Copurchase = len(items)
		out = append(out, items...)
	}

	// 2. regra técnica
	if len(out) < limit {
		items, err := h.complements(sourceID, categoryID, limit-len(out), visto)
		if err != nil {
			DBError(c, err)
			return
		}
		meta.Counts.Complement = len(items)
		out = append(out, items...)
	}

	// 3. fallback por categoria
	if len(out) < limit {
		items, err := h.categoryFallback(sourceID, categoryID, limit-len(out), visto)
		if err != nil {
			DBError(c, err)
			return
		}
		meta.Counts.Fallback = len(items)
		out = append(out, items...)
	}

	meta.Fallback = meta.Counts.Fallback > 0
	meta.Strategy = strategyOf(meta)

	c.JSON(http.StatusOK, model.RelatedResponse{Data: out, Meta: meta})
}

// strategyOf resume a composição num rótulo só, para a UI escolher o título da
// seção sem reimplementar a lógica de contagem.
func strategyOf(m model.RelatedMeta) string {
	fontes := 0
	unica := model.ReasonCategoryFallback
	if m.Counts.Copurchase > 0 {
		fontes++
		unica = model.ReasonCopurchase
	}
	if m.Counts.Complement > 0 {
		fontes++
		unica = model.ReasonComplement
	}
	if m.Counts.Fallback > 0 {
		fontes++
	}
	if fontes > 1 {
		return "mixed"
	}
	return unica
}

// copurchase lê o agregado. É um index scan em idx_copurchase_lookup — o
// trabalho pesado foi feito pelo job de internal/reco.
func (h *RecommendationHandler) copurchase(sourceID string, limit int, visto map[string]bool) ([]model.RelatedProduct, error) {
	// ⚠️ O `candidatos` SEPARADO NÃO É ENFEITE — é o que limita o trabalho.
	//
	// A versão de uma consulta só (JOIN + ORDER BY cp.order_count, p.rating_bayes)
	// medida no perf_lab: o desempate por `rating_bayes` é coluna de `products`,
	// então o planejador PRECISA ler a linha de produto de TODOS os pares antes
	// de aplicar o LIMIT. Num campeão de vendas com 16.666 pares, isso deu
	// 10,9 ms e 10.195 buffers para devolver 8 cards.
	//
	// Aqui o corte é feito primeiro, DENTRO do índice (product_id, order_count
	// DESC), com uma ordem que só usa colunas de `product_copurchase`. O JOIN
	// passa a tocar no máximo `limit*4` linhas, e o custo deixa de depender de
	// quantos pares o produto acumulou. Ver docs/reviews-e-recomendacao.md.
	rows, err := h.db.Query(`
		WITH candidatos AS (
			SELECT related_product_id, order_count
			  FROM product_copurchase
			 WHERE product_id = $1 AND order_count >= $2
			 ORDER BY order_count DESC, related_product_id ASC
			 LIMIT $4
		)
		SELECT `+productColumns+`, cp.order_count
		  FROM candidatos cp
		  JOIN products p ON p.id = cp.related_product_id
		  JOIN sellers  s ON s.id = p.seller_id
		 WHERE p.status = 'published'
		 ORDER BY cp.order_count DESC, p.rating_bayes DESC, p.id ASC
		 LIMIT $3
	`, sourceID, MinCopurchaseOrders, limit, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.RelatedProduct, 0, limit)
	for rows.Next() {
		var p model.Product
		var n int
		if err := scanProductWith(rows, &p, &n); err != nil {
			return nil, err
		}
		if visto[p.ID] {
			continue
		}
		visto[p.ID] = true
		out = append(out, model.RelatedProduct{
			Product: p,
			Reason: model.RelatedReason{
				Kind:   model.ReasonCopurchase,
				Label:  "Quem levou este produto também levou",
				Orders: n,
			},
		})
	}
	return out, rows.Err()
}

// complements resolve as regras técnicas que casam com o produto de origem.
//
// UMA CONSULTA SÓ, não uma por regra: as regras aplicáveis e os produtos-alvo
// saem juntos num LATERAL. O laço em Go faria N+1 numa rota que está no caminho
// da página de produto — o mesmo erro que o item 2 de docs/performance-banco.md
// documenta no order-service.
//
// O casamento de origem usa `search_vector` (migration 014) do produto ATUAL:
// é uma linha só, então o custo é a avaliação da tsquery, não uma varredura.
func (h *RecommendationHandler) complements(sourceID, categoryID string, limit int, visto map[string]bool) ([]model.RelatedProduct, error) {
	rows, err := h.db.Query(`
		WITH origem AS (
			SELECT id, category_id, search_vector FROM products WHERE id = $1
		),
		regras AS (
			SELECT r.id, r.note, r.priority, r.target_category_id, r.target_query
			  FROM product_complement_rules r, origem o
			 WHERE r.active
			   AND (r.source_category_id IS NULL OR r.source_category_id = o.category_id)
			   AND (r.source_query IS NULL
			        OR o.search_vector @@ websearch_to_tsquery('utilar_pt', r.source_query))
			 ORDER BY r.priority ASC, r.id ASC
		)
		SELECT `+productColumns+`, g.note
		  FROM regras g
		  JOIN LATERAL (
			SELECT p.*
			  FROM products p
			 WHERE p.status = 'published'
			   AND p.id <> $1
			   AND (g.target_category_id IS NULL OR p.category_id = g.target_category_id)
			   AND (g.target_query IS NULL
			        OR p.search_vector @@ websearch_to_tsquery('utilar_pt', g.target_query))
			 ORDER BY p.rating_bayes DESC, p.review_count DESC, p.id ASC
			 -- 2 por regra: a ideia é COBRIR as necessidades técnicas (argamassa
			 -- E rejunte E espaçador), não empilhar oito argamassas. Uma regra
			 -- gulosa afogaria as outras e devolveria a mesma prateleira de novo.
			 LIMIT 2
		  ) p ON true
		  JOIN sellers s ON s.id = p.seller_id
		 ORDER BY g.priority ASC, p.rating_bayes DESC, p.id ASC
		 LIMIT $2
	`, sourceID, limit*3) // *3: sobra para descartar os já vistos sem segunda ida ao banco
	if err != nil {
		// Regra com termo inválido não pode derrubar a página de produto.
		// websearch_to_tsquery não levanta erro de sintaxe (é por isso que ela
		// foi escolhida na 016), mas um erro aqui ainda assim degrada para as
		// outras fontes em vez de virar 500.
		slog.Error("related.complements_failed", "product", sourceID, "error", err.Error())
		return nil, nil
	}
	defer rows.Close()

	out := make([]model.RelatedProduct, 0, limit)
	for rows.Next() {
		var p model.Product
		var note string
		if err := scanProductWith(rows, &p, &note); err != nil {
			return nil, err
		}
		if visto[p.ID] || len(out) >= limit {
			continue
		}
		visto[p.ID] = true
		n := note
		out = append(out, model.RelatedProduct{
			Product: p,
			Reason: model.RelatedReason{
				Kind:  model.ReasonComplement,
				Label: "Costuma ser necessário junto",
				Note:  &n,
			},
		})
	}
	return out, rows.Err()
}

// categoryFallback é o preenchimento honesto: outros produtos da MESMA
// categoria, quando não há dado suficiente para recomendar de verdade.
//
// ⚠️ O `md5(id || $1)` no ORDER BY é o que conserta o defeito original. Ordenar
// só por qualidade devolve os mesmos N itens para TODOS os produtos da
// categoria — que era exatamente a queixa. Misturando o id do produto de ORIGEM
// no critério de desempate, cada página recebe um recorte diferente, de forma
// determinística (a mesma página devolve sempre a mesma lista, então cache e
// paginação continuam coerentes) e sem `random()`, que quebraria as duas coisas.
//
// A qualidade continua mandando primeiro (`rating_bayes`); a rotação só decide
// entre empatados — e hoje, com a loja começando sem avaliações, todo mundo
// empata em 0, então a rotação faz todo o trabalho. É o comportamento certo:
// sem sinal, variedade é melhor que uma lista fixa.
func (h *RecommendationHandler) categoryFallback(sourceID, categoryID string, limit int, visto map[string]bool) ([]model.RelatedProduct, error) {
	rows, err := h.db.Query(`
		SELECT `+productColumns+`
		  FROM products p
		  JOIN sellers s ON s.id = p.seller_id
		 WHERE p.category_id = $2
		   AND p.id <> $1
		   AND p.status = 'published'
		 ORDER BY p.rating_bayes DESC, md5(p.id::text || $1) ASC
		 LIMIT $3
	`, sourceID, categoryID, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.RelatedProduct, 0, limit)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		if visto[p.ID] || len(out) >= limit {
			continue
		}
		visto[p.ID] = true
		out = append(out, model.RelatedProduct{
			Product: p,
			Reason: model.RelatedReason{
				Kind: model.ReasonCategoryFallback,
				// O rótulo NÃO diz "recomendado". É a mesma honestidade do
				// meta.fallback, na camada que o usuário lê.
				Label: "Outros produtos desta categoria",
			},
		})
	}
	return out, rows.Err()
}

// scanProductWith lê a projeção pública mais colunas EXTRAS no fim.
//
// Existe porque as consultas de recomendação carregam a evidência do motivo
// (número de pedidos, nota da regra) na mesma linha do produto — buscá-la
// depois seria N+1 por card.
func scanProductWith(rows *sql.Rows, p *model.Product, extras ...any) error {
	dest := append(productScanTargets(p), extras...)
	return rows.Scan(dest...)
}
