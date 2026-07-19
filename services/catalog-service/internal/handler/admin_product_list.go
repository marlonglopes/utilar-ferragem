package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/model"
)

// ListProducts — GET /api/v1/admin/products
//
// A listagem que a tela de gestão de produtos consome. Não existia: só havia
// busca por id (um produto por vez), e a listagem pública — que corretamente
// esconde `cost` e só mostra `published`. O painel abria com "Produto não
// encontrado" porque a rota devolvia 404.
//
// Diferenças deliberadas em relação à listagem pública:
//   - devolve `cost` e `marginPct` (é rota de admin, e é o número que evita
//     cadastrar produto no prejuízo);
//   - NÃO filtra por status: o admin precisa justamente enxergar rascunho e
//     arquivado — é neles que ele trabalha depois de importar uma planilha;
//   - a busca inclui SKU e marca, porque quem administra procura por código.
//
// Query: q, category, status, sort, dir, page, pageSize
func (h *CatalogAdminHandler) ListProducts(c *gin.Context) {
	// sanitizeText antes de qualquer coisa: byte nulo e caractere de controle
	// não são digitáveis numa caixa de busca — só aparecem em ataque — e o
	// Postgres RECUSA NUL em texto, transformando `?q=%00` num 500. A busca
	// pública tinha exatamente essa falha; não repetir aqui.
	q := truncateRunes(sanitizeText(c.Query("q")), 120)
	category := strings.TrimSpace(c.Query("category"))
	status := strings.TrimSpace(c.Query("status"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "25"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}

	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if q != "" {
		// ILIKE com escape, e não o tsvector da busca pública: aqui o operador
		// procura por fragmento de SKU ("CUR-FER") ou pedaço de nome que lembra,
		// e stemming atrapalharia. O volume da tela de admin é uma página, então
		// o custo é aceitável — e o escape é o mesmo do audit CT1-C1, sem o qual
		// `%_%_%_%_%` força CPU 100% no pg_trgm.
		esc := escapeLikePattern(q)
		where = append(where, fmt.Sprintf(
			"(p.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.sku,'') ILIKE $%d ESCAPE '\\' "+
				"OR COALESCE(p.brand,'') ILIKE $%d ESCAPE '\\' OR COALESCE(p.barcode,'') = $%d)",
			idx, idx, idx, idx+1))
		args = append(args, "%"+esc+"%", q)
		idx += 2
	}
	if category != "" {
		where = append(where, fmt.Sprintf("p.category_id = $%d", idx))
		args = append(args, category)
		idx++
	}
	if status != "" {
		// Whitelist: valor desconhecido vira "sem filtro" em vez de erro, pra um
		// link antigo não quebrar a tela.
		switch status {
		case "draft", "published", "archived":
			where = append(where, fmt.Sprintf("p.status = $%d", idx))
			args = append(args, status)
			idx++
		}
	}
	whereSQL := strings.Join(where, " AND ")

	// Whitelist de ordenação — o valor vem da URL e NUNCA pode entrar no SQL.
	dir := "DESC"
	if strings.EqualFold(c.Query("dir"), "asc") {
		dir = "ASC"
	}
	orderBy := "p.created_at " + dir
	switch c.Query("sort") {
	case "name":
		orderBy = "p.name " + dir
	case "price":
		orderBy = "p.price " + dir
	case "cost":
		orderBy = "p.cost " + dir + " NULLS LAST"
	case "stock":
		orderBy = "p.stock " + dir
	case "sku":
		orderBy = "p.sku " + dir + " NULLS LAST"
	case "status":
		orderBy = "p.status " + dir + ", p.created_at DESC"
	case "margin":
		// Margem não é coluna: calcula na ordenação. NULLS LAST porque produto
		// sem custo não tem margem conhecida e não deve encabeçar a lista.
		orderBy = "CASE WHEN p.cost IS NULL OR p.price = 0 THEN NULL " +
			"ELSE (p.price - p.cost) / p.price END " + dir + " NULLS LAST"
	}

	var total int
	if err := h.db.QueryRow(
		"SELECT count(*) FROM products p WHERE "+whereSQL, args...,
	).Scan(&total); err != nil {
		DBError(c, err)
		return
	}

	args = append(args, pageSize, (page-1)*pageSize)

	// #nosec G202 — whereSQL e orderBy são montados só de literais fixos e
	// placeholders posicionais; os valores entram por `args`.
	rows, err := h.db.Query(`
		SELECT `+productColumns+`,
		       p.cost, p.supplier_id, p.supplier_sku, p.ncm, p.cfop, p.cest, p.origem, p.status
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE `+whereSQL+`
		ORDER BY `+orderBy+`
		LIMIT $`+strconv.Itoa(idx)+` OFFSET $`+strconv.Itoa(idx+1), args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	produtos := make([]model.AdminProduct, 0, pageSize)
	ids := make([]string, 0, pageSize)
	for rows.Next() {
		var (
			p    model.AdminProduct
			cost *float64
		)
		if err := rows.Scan(adminProductScanTargets(&p, &cost)...); err != nil {
			DBError(c, err)
			return
		}
		p.Cost = cost
		// Mesma função da rota de balcão de propósito: gerente e vendedor
		// precisam ver o MESMO número de margem pro mesmo produto.
		p.MarginPct = marginPct(p.Price, cost)
		produtos = append(produtos, p)
		ids = append(ids, p.ID)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	h.attachCovers(c, produtos, ids)

	totalPages := (total + pageSize - 1) / pageSize
	c.JSON(http.StatusOK, gin.H{
		"data": produtos,
		"meta": gin.H{
			"page": page, "pageSize": pageSize,
			"total": total, "totalPages": totalPages,
		},
	})
}

// attachCovers preenche a capa de cada produto da página em UMA query.
//
// Mesmo motivo do `loadThumbnails` da vitrine: uma consulta por produto seria
// N+1, e aqui a tela ainda mostra até 100 linhas por página.
//
// Falha aqui é degradação, não erro: sem foto a linha mostra o ícone. A
// listagem inteira não pode falhar por causa da miniatura.
func (h *CatalogAdminHandler) attachCovers(c *gin.Context, produtos []model.AdminProduct, ids []string) {
	if len(ids) == 0 {
		return
	}
	rows, err := h.db.Query(`
		SELECT DISTINCT ON (product_id) product_id, url, alt
		FROM product_images
		WHERE product_id = ANY($1)
		ORDER BY product_id, sort_order ASC
	`, pq.Array(ids))
	if err != nil {
		logDetailEnrichFailure(c, "", "admin_list_covers", err)
		return
	}
	defer rows.Close()

	capas := make(map[string]model.ProductImage, len(ids))
	for rows.Next() {
		var pid string
		var im model.ProductImage
		if err := rows.Scan(&pid, &im.URL, &im.Alt); err != nil {
			logDetailEnrichFailure(c, pid, "admin_list_covers", err)
			return
		}
		capas[pid] = im
	}
	if err := rows.Err(); err != nil {
		logDetailEnrichFailure(c, "", "admin_list_covers", err)
		return
	}
	for i := range produtos {
		if im, ok := capas[produtos[i].ID]; ok {
			produtos[i].Images = []model.ProductImage{im}
		}
	}
}
