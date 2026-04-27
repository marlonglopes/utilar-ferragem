package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
)

// slugLookupMinElapsed é o tempo mínimo de resposta de GetBySlug e Related.
// Mitiga timing attack que distinguiria slug existente vs inexistente
// (CT1-H4). Valor pequeno o suficiente pra não impactar UX humana.
const slugLookupMinElapsed = 50 * time.Millisecond

// padToMinElapsed bloqueia até que pelo menos `min` tenha decorrido desde
// `start`. Usado pra normalizar tempo de respostas sensíveis a timing.
func padToMinElapsed(start time.Time, min time.Duration) {
	if elapsed := time.Since(start); elapsed < min {
		time.Sleep(min - elapsed)
	}
}

type ProductHandler struct{ db *sql.DB }

func NewProductHandler(db *sql.DB) *ProductHandler { return &ProductHandler{db: db} }

// List GET /api/v1/products
// Query params: category, q, brand, price_min, price_max, in_stock, sort, page, per_page
func (h *ProductHandler) List(c *gin.Context) {
	params := parseProductsQuery(c)

	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if params.Category != "" {
		where = append(where, fmt.Sprintf("p.category_id = $%d", idx))
		args = append(args, params.Category)
		idx++
	}
	if params.Q != "" {
		// SEGURANÇA (audit CT1-C1): escapar `%` `_` `\` no termo de busca antes
		// do ILIKE, com `ESCAPE '\'`. Sem isso, atacante envia `%_%_%_%_%_%_%_`
		// e força ReDoS no pg_trgm (consumo CPU 100%).
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.description,'') ILIKE $%d ESCAPE '\\' OR s.name ILIKE $%d ESCAPE '\\')", idx, idx, idx))
		args = append(args, "%"+escapeLikePattern(params.Q)+"%")
		idx++
	}
	if params.Brand != "" {
		where = append(where, fmt.Sprintf("p.brand = $%d", idx))
		args = append(args, params.Brand)
		idx++
	}
	if params.PriceMin != nil {
		where = append(where, fmt.Sprintf("p.price >= $%d", idx))
		args = append(args, *params.PriceMin)
		idx++
	}
	if params.PriceMax != nil {
		where = append(where, fmt.Sprintf("p.price <= $%d", idx))
		args = append(args, *params.PriceMax)
		idx++
	}
	if params.InStock {
		where = append(where, "p.stock > 0")
	}

	// orderBy é whitelist — qualquer valor não reconhecido cai no default (audit CT1-M3).
	orderBy := "p.created_at DESC"
	switch params.Sort {
	case "price_asc":
		orderBy = "p.price ASC"
	case "price_desc":
		orderBy = "p.price DESC"
	case "top_rated":
		orderBy = "p.rating DESC, p.review_count DESC"
	case "newest", "":
		orderBy = "p.created_at DESC"
	}

	whereSQL := strings.Join(where, " AND ")

	// Count
	var total int
	countSQL := "SELECT count(*) FROM products p JOIN sellers s ON s.id = p.seller_id WHERE " + whereSQL
	if err := h.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		DBError(c, err)
		return
	}

	// Page
	offset := (params.Page - 1) * params.PerPage
	args = append(args, params.PerPage, offset)

	// #nosec G202 — whereSQL/orderBy são construídos só de literais hardcoded
	// (`p.brand = $N`, `p.price ASC`, etc.) com placeholders posicionais.
	// orderBy passa por whitelist em parseProductsQuery (CT1-M3); valores
	// entram via `args`. Atacante não controla os fragmentos de SQL.
	querySQL := `
		SELECT
		  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
		  s.name, s.id, s.rating, s.review_count,
		  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
		  p.description, p.specs, p.created_at, p.updated_at
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE ` + whereSQL + `
		ORDER BY ` + orderBy + `
		LIMIT $` + strconv.Itoa(idx) + ` OFFSET $` + strconv.Itoa(idx+1)

	rows, err := h.db.Query(querySQL, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	products := make([]model.Product, 0)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			DBError(c, err)
			return
		}
		products = append(products, p)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	c.JSON(http.StatusOK, model.ProductsResponse{
		Data: products,
		Meta: model.Meta{Page: params.Page, PerPage: params.PerPage, Total: total, TotalPages: totalPages},
	})
}

// GetBySlug GET /api/v1/products/:slug
//
// L-CATALOG-2: gera ETag derivado do `updated_at` do produto. Se o cliente
// envia `If-None-Match` que bate, retorna 304 Not Modified — economiza
// bandwidth em browsers/CDN com produto não-modificado.
func (h *ProductHandler) GetBySlug(c *gin.Context) {
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	slug := c.Param("slug")

	row := h.db.QueryRow(`
		SELECT
		  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
		  s.name, s.id, s.rating, s.review_count,
		  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
		  p.description, p.specs, p.created_at, p.updated_at
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.slug = $1
	`, slug)
	p, err := scanProduct(row)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Load images
	imgRows, err := h.db.Query(`
		SELECT url, alt FROM product_images WHERE product_id=$1 ORDER BY sort_order ASC
	`, p.ID)
	if err == nil {
		defer imgRows.Close()
		for imgRows.Next() {
			var im model.ProductImage
			if err := imgRows.Scan(&im.URL, &im.Alt); err == nil {
				p.Images = append(p.Images, im)
			}
		}
	}

	if respondWithETag(c, p.UpdatedAt) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// GetByID GET /api/v1/products/by-id/:id
//
// Endpoint usado por outros serviços (order-service) pra resolver um produto
// pelo seu UUID — o frontend mantém ID em vez de slug nos itens do carrinho.
// Retorna o mesmo payload que GetBySlug. O timing pad também aplica aqui pra
// não vazar enumeração de IDs (CT1-H4 generalizado).
func (h *ProductHandler) GetByID(c *gin.Context) {
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	id := c.Param("id")

	row := h.db.QueryRow(`
		SELECT
		  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
		  s.name, s.id, s.rating, s.review_count,
		  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
		  p.description, p.specs, p.created_at, p.updated_at
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.id = $1
	`, id)
	p, err := scanProduct(row)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	imgRows, err := h.db.Query(`
		SELECT url, alt FROM product_images WHERE product_id=$1 ORDER BY sort_order ASC
	`, p.ID)
	if err == nil {
		defer imgRows.Close()
		for imgRows.Next() {
			var im model.ProductImage
			if err := imgRows.Scan(&im.URL, &im.Alt); err == nil {
				p.Images = append(p.Images, im)
			}
		}
	}

	if respondWithETag(c, p.UpdatedAt) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// Facets GET /api/v1/products/facets?category=...&q=...
func (h *ProductHandler) Facets(c *gin.Context) {
	params := parseProductsQuery(c)

	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if params.Category != "" {
		where = append(where, fmt.Sprintf("p.category_id = $%d", idx))
		args = append(args, params.Category)
		idx++
	}
	if params.Q != "" {
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.description,'') ILIKE $%d ESCAPE '\\')", idx, idx))
		args = append(args, "%"+escapeLikePattern(params.Q)+"%")
		idx++
	}
	whereSQL := strings.Join(where, " AND ")

	// Brands with counts.
	// #nosec G202 — whereSQL é construído só de literais hardcoded (`p.brand = $N`,
	// `p.category_id = $N`, etc) com placeholders posicionais; valores entram via `args`,
	// nunca em string concat. Atacante não controla os fragmentos de SQL.
	brandSQL := `
		SELECT p.brand, count(*) AS cnt
		FROM products p
		WHERE ` + whereSQL + ` AND p.brand IS NOT NULL
		GROUP BY p.brand ORDER BY cnt DESC, p.brand ASC
	`
	rows, err := h.db.Query(brandSQL, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	brands := make([]model.BrandFacet, 0)
	for rows.Next() {
		var b model.BrandFacet
		if err := rows.Scan(&b.Value, &b.Count); err == nil {
			brands = append(brands, b)
		}
	}

	var priceMin, priceMax sql.NullFloat64
	priceSQL := "SELECT MIN(p.price), MAX(p.price) FROM products p WHERE " + whereSQL
	h.db.QueryRow(priceSQL, args...).Scan(&priceMin, &priceMax)

	c.JSON(http.StatusOK, model.Facets{
		Brands:   brands,
		PriceMin: priceMin.Float64,
		PriceMax: priceMax.Float64,
	})
}

// Related GET /api/v1/products/:slug/related?limit=4
// Produtos da mesma categoria, excluindo o slug atual.
func (h *ProductHandler) Related(c *gin.Context) {
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	slug := c.Param("slug")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "4"))
	if limit < 1 || limit > 24 {
		limit = 4
	}

	rows, err := h.db.Query(`
		SELECT
		  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
		  s.name, s.id, s.rating, s.review_count,
		  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
		  p.description, p.specs, p.created_at, p.updated_at
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.category_id = (SELECT category_id FROM products WHERE slug = $1 LIMIT 1)
		  AND p.slug != $1
		ORDER BY p.rating DESC, p.review_count DESC
		LIMIT $2
	`, slug, limit)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Product, 0)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			DBError(c, err)
			return
		}
		out = append(out, p)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// -- helpers -----------------------------------------------------------------

// CT1-M1: caps por filtro pra prevenir DoS via query string absurda
// (ILIKE em string de 100KB queima CPU mesmo com escape).
const (
	maxFilterQ        = 100 // termo de busca (typed)
	maxFilterCategory = 64  // slug
	maxFilterBrand    = 64  // nome de marca
	maxFilterSort     = 32  // string de sort (já tem whitelist)
)

// truncateRunes corta a string mantendo os primeiros N runes (não bytes).
// Evita explodir caracteres UTF-8 multibyte no meio.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

type productsQuery struct {
	Category string
	Q        string
	Brand    string
	PriceMin *float64
	PriceMax *float64
	InStock  bool
	Sort     string
	Page     int
	PerPage  int
}

func parseProductsQuery(c *gin.Context) productsQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "24"))
	if perPage < 1 || perPage > 100 {
		perPage = 24
	}

	// CT1-M1: trunca cada filtro pra um cap sensato. Cap > limite humano (q=100
	// chars já é uma frase longa) mas << que payload abusivo.
	q := productsQuery{
		Category: truncateRunes(c.Query("category"), maxFilterCategory),
		Q:        truncateRunes(strings.TrimSpace(c.Query("q")), maxFilterQ),
		Brand:    truncateRunes(c.Query("brand"), maxFilterBrand),
		InStock:  c.Query("in_stock") == "true",
		Sort:     truncateRunes(c.Query("sort"), maxFilterSort),
		Page:     page,
		PerPage:  perPage,
	}

	// price_min/max só aceitos se não-negativos (audit CT1-H3) — preço negativo
	// causa lógica de negócio surpresa.
	if v := c.Query("price_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			q.PriceMin = &f
		}
	}
	if v := c.Query("price_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			q.PriceMax = &f
		}
	}
	return q
}

// escapeLikePattern escapa os metacaracteres do LIKE/ILIKE (`%`, `_`, `\`).
// Usado junto com `ESCAPE '\'` no SQL pra prevenir ReDoS via wildcard injection
// no pg_trgm (audit CT1-C1).
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (model.Product, error) {
	var p model.Product
	err := row.Scan(
		&p.ID, &p.Slug, &p.Name, &p.Category, &p.Price, &p.OriginalPrice, &p.Currency, &p.Icon, &p.Brand,
		&p.Seller, &p.SellerID, &p.SellerRating, &p.SellerReviewCt,
		&p.Stock, &p.Rating, &p.ReviewCount, &p.CashbackAmount, &p.Badge, &p.BadgeLabel, &p.Installments,
		&p.Description, &p.Specs, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}
