package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/utilar/catalog-service/internal/model"
	"github.com/utilar/catalog-service/internal/storage"
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

type ProductHandler struct {
	db *sql.DB
	// media traduz a CHAVE lógica gravada em product_images.variants na URL
	// pública. É o único ponto de leitura que sabe onde a mídia mora — por isso
	// migrar disco→S3/CDN é trocar o resolver, não reescrever a tabela.
	media storage.URLResolver
}

func NewProductHandler(db *sql.DB) *ProductHandler {
	// Default "/media": é onde o driver local publica. Mantém a assinatura de
	// antes funcionando (vários testes chamam assim) sem precisar de storage.
	return &ProductHandler{db: db, media: storage.PrefixResolver("/media")}
}

// WithMedia troca o resolver de URL de mídia. O main injeta o do storage ativo.
func (h *ProductHandler) WithMedia(r storage.URLResolver) *ProductHandler {
	if r != nil {
		h.media = r
	}
	return h
}

// imageVariants converte o JSONB de variantes (chaves lógicas) no bloco de URLs
// públicas do payload.
//
// Devolve nil para imagem EXTERNA (as 288 fotos CC0 do Wikimedia, sem
// variantes) — e é essa ausência que o frontend usa pra distinguir os dois
// tipos: com `variants`, escolhe o tamanho pelo contexto; sem, usa `url`.
func (h *ProductHandler) imageVariants(raw []byte) *model.ImageVariants {
	if len(raw) == 0 {
		return nil
	}
	var keys map[string]string
	if err := json.Unmarshal(raw, &keys); err != nil || len(keys) == 0 {
		return nil
	}
	return &model.ImageVariants{
		Thumb:  h.media.URL(keys["thumb"]),
		Medium: h.media.URL(keys["medium"]),
		Large:  h.media.URL(keys["large"]),
	}
}

// productColumns é a projeção PÚBLICA de produto — a única usada pelas rotas
// abertas.
//
// ⚠️ `p.cost` NÃO ESTÁ AQUI E NÃO PODE ENTRAR. Custo é informação sensível de
// negócio (quem vê o custo sabe até onde a loja pode baixar o preço) e só sai
// pelas rotas de admin, que têm projeção própria em admin_product.go.
// O teste TestPublicAPI_NuncaVazaCusto cobre isso.
//
// Constante única em vez de a mesma lista repetida em quatro queries: era o que
// já acontecia, e uma coluna nova exigia lembrar de editar os quatro lugares
// (com `scanProduct` quebrando silenciosamente na ordem errada).
const productColumns = `
	  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
	  s.name, s.id, s.rating, s.review_count,
	  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
	  p.description, p.specs, p.created_at, p.updated_at,
	  p.sku, p.barcode, p.unit_of_measure, p.qty_step,
	  p.weight_kg, p.length_cm, p.width_cm, p.height_cm`

// List GET /api/v1/products
// Query params: category, q, brand, price_min, price_max, in_stock, sort, page, per_page
func (h *ProductHandler) List(c *gin.Context) {
	params := parseProductsQuery(c)

	where := []string{"p.status = 'published'"}
	args := []any{}
	idx := 1

	if params.Category != "" {
		where = append(where, fmt.Sprintf("p.category_id = $%d", idx))
		args = append(args, params.Category)
		idx++
	}
	// Lookup exato de scanner/balcão. Vem ANTES do `q` porque é a leitura mais
	// quente do PDV e precisa cair direto no índice único, sem ILIKE.
	if params.SKU != "" {
		where = append(where, fmt.Sprintf("p.sku = $%d", idx))
		args = append(args, params.SKU)
		idx++
	}
	if params.Barcode != "" {
		where = append(where, fmt.Sprintf("p.barcode = $%d", idx))
		args = append(args, params.Barcode)
		idx++
	}
	if params.Q != "" {
		// SEGURANÇA (audit CT1-C1): escapar `%` `_` `\` no termo de busca antes
		// do ILIKE, com `ESCAPE '\'`. Sem isso, atacante envia `%_%_%_%_%_%_%_`
		// e força ReDoS no pg_trgm (consumo CPU 100%).
		//
		// SKU entra na busca geral por PREFIXO (`ABC%`), não por `%ABC%`: código
		// de produto é lido/digitado da esquerda pra direita, e o infixo daria
		// resultado lixo ("12" casando com metade do catálogo) além de ser mais
		// caro. Antes desta mudança o SKU não era buscável de jeito nenhum — o
		// vendedor digitava o código no balcão e recebia zero resultados.
		where = append(where, fmt.Sprintf(
			"(p.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.description,'') ILIKE $%d ESCAPE '\\' OR s.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.sku,'') ILIKE $%d ESCAPE '\\' OR p.barcode = $%d)",
			idx, idx, idx, idx+1, idx+2))
		esc := escapeLikePattern(params.Q)
		args = append(args, "%"+esc+"%", esc+"%", params.Q)
		idx += 3
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

	// Facetas técnicas: cada filtro de atributo vira um EXISTS independente,
	// que é o que dá semântica de AND entre grandezas diferentes ("bitola 2,5
	// E cor azul"). Um JOIN único não conseguiria — uma linha de
	// product_attributes tem uma chave só.
	where, args, idx = appendAttrFilters(where, args, idx, params)

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
		SELECT ` + productColumns + `
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

	// Capa de cada produto, em uma query só (ver loadThumbnails).
	h.loadThumbnails(c, products)

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
		SELECT `+productColumns+`
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.slug = $1 AND p.status = 'published'
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

	h.loadImages(c, &p)
	h.enrichDetail(c, &p)

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
		SELECT `+productColumns+`
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.id = $1 AND p.status = 'published'
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

	h.loadImages(c, &p)
	h.enrichDetail(c, &p)

	if respondWithETag(c, p.UpdatedAt) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// Facets GET /api/v1/products/facets?category=...&q=...
func (h *ProductHandler) Facets(c *gin.Context) {
	params := parseProductsQuery(c)

	where := []string{"p.status = 'published'"}
	args := []any{}
	idx := 1

	if params.Category != "" {
		where = append(where, fmt.Sprintf("p.category_id = $%d", idx))
		args = append(args, params.Category)
		idx++
	}
	if params.Q != "" {
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d ESCAPE '\\' OR COALESCE(p.description,'') ILIKE $%d ESCAPE '\\' OR COALESCE(p.sku,'') ILIKE $%d ESCAPE '\\')", idx, idx, idx+1))
		esc := escapeLikePattern(params.Q)
		args = append(args, "%"+esc+"%", esc+"%")
		idx += 2
	}
	where, args, idx = appendAttrFilters(where, args, idx, params)
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
		if err := rows.Scan(&b.Value, &b.Count); err != nil {
			DBError(c, err)
			return
		}
		brands = append(brands, b)
	}
	// rows.Err() distingue "não há marcas" de "a varredura morreu no meio" —
	// sem isso a vitrine mostraria uma lista de facetas truncada como se fosse
	// completa, e o usuário filtraria por um universo que não existe.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var priceMin, priceMax sql.NullFloat64
	priceSQL := "SELECT MIN(p.price), MAX(p.price) FROM products p WHERE " + whereSQL
	if err := h.db.QueryRow(priceSQL, args...).Scan(&priceMin, &priceMax); err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.Facets{
		Brands:     brands,
		PriceMin:   priceMin.Float64,
		PriceMax:   priceMax.Float64,
		Attributes: h.attributeFacets(c, params, whereSQL, args),
	})
}

// attributeFacets monta as facetas técnicas (bitola, tensão, potência) do
// resultado atual.
//
// SÓ COM CATEGORIA: atributo é definido por categoria no registry. Facetar
// "potência" sobre o catálogo inteiro juntaria furadeira com saco de cimento e
// devolveria um slider sem sentido — além de varrer a tabela toda.
//
// Numéricos viram min/max (slider); textuais viram valores com contagem
// (checkbox). Cada um é uma query só, agrupada — não uma query por atributo.
func (h *ProductHandler) attributeFacets(c *gin.Context, params productsQuery, whereSQL string, args []any) []model.AttributeFacet {
	out := make([]model.AttributeFacet, 0)
	if params.Category == "" {
		return out
	}

	// `p` continua na cláusula porque whereSQL referencia p.*; o JOIN com
	// sellers é necessário porque whereSQL pode citar s.name (busca por `q`).
	// #nosec G202 — whereSQL vem do mesmo builder das outras queries: literais
	// hardcoded com placeholders posicionais.
	base := `
		FROM product_attributes pa
		JOIN products p ON p.id = pa.product_id
		JOIN sellers s ON s.id = p.seller_id
		JOIN category_attributes ca ON ca.key = pa.key AND ca.category_id = p.category_id
		WHERE ` + whereSQL + ` AND ca.filterable = true`

	rows, err := h.db.Query(`
		SELECT ca.key, ca.label, ca.data_type, ca.unit,
		       pa.value_text,
		       MIN(pa.value_num), MAX(pa.value_num),
		       count(*), MIN(ca.sort_order)
		`+base+`
		GROUP BY ca.key, ca.label, ca.data_type, ca.unit, pa.value_text
		ORDER BY MIN(ca.sort_order) ASC, ca.key ASC, count(*) DESC`, args...)
	if err != nil {
		// Faceta é enriquecimento: sem ela a busca ainda funciona. Não vale um
		// 500 na vitrine inteira.
		slog.Error("facets.attributes_failed",
			"category", params.Category,
			"request_id", c.GetString("request_id"), "error", err.Error())
		return out
	}
	defer rows.Close()

	byKey := map[string]*model.AttributeFacet{}
	order := []string{}

	for rows.Next() {
		var (
			key, label, dataType string
			unit, valueText      *string
			min, max             *float64
			count, sortOrder     int
		)
		if err := rows.Scan(&key, &label, &dataType, &unit, &valueText, &min, &max, &count, &sortOrder); err != nil {
			slog.Error("facets.attributes_scan_failed", "request_id", c.GetString("request_id"), "error", err.Error())
			return out
		}

		f, ok := byKey[key]
		if !ok {
			f = &model.AttributeFacet{Key: key, Label: label, DataType: dataType, Unit: unit}
			byKey[key] = f
			order = append(order, key)
		}

		if dataType == "number" {
			if min != nil && (f.Min == nil || *min < *f.Min) {
				f.Min = min
			}
			if max != nil && (f.Max == nil || *max > *f.Max) {
				f.Max = max
			}
			continue
		}
		if valueText != nil {
			f.Values = append(f.Values, model.AttributeValueFacet{Value: *valueText, Count: count})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("facets.attributes_iter_failed", "request_id", c.GetString("request_id"), "error", err.Error())
		return out
	}

	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
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
		SELECT `+productColumns+`
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.category_id = (SELECT category_id FROM products WHERE slug = $1 LIMIT 1)
		  AND p.slug != $1
		  AND p.status = 'published'
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
	maxFilterCode     = 64  // SKU / código de barras (EAN tem 14; SKU de fornecedor é maior)
	maxFilterAttr     = 128 // par `chave:valor` de faceta técnica
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
	SKU      string
	Barcode  string
	PriceMin *float64
	PriceMax *float64
	InStock  bool
	Sort     string
	Page     int
	PerPage  int
	Attrs    []attrFilter
}

// attrFilter é um filtro por atributo tipado (`?attr=potencia_w:650`).
// Num != nil quando o valor parseou como número — aí a comparação vai contra
// `value_num` e não contra a representação textual (2.5 ≠ "2.500000").
type attrFilter struct {
	Key string
	Str string
	Num *float64
	Op  string // "eq" | "gte" | "lte"
}

// attrKeyRe: chave de atributo é identificador do nosso registry, não texto
// livre do usuário. Restringir aqui evita que qualquer coisa vire argumento de
// query e limita o custo de uma query maliciosa.
var attrKeyRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// maxAttrFilters: cada filtro vira um EXISTS. Sem teto, `?attr=` repetido 500
// vezes monta um plano absurdo — mesmo problema de DoS que CT1-M1 tratou.
const maxAttrFilters = 8

// parseAttrFilters lê os parâmetros `attr`, `attr_min` e `attr_max`, todos no
// formato `chave:valor` e todos repetíveis.
//
//	?attr=cor:Azul&attr_min=potencia_w:700
//
// Formato inválido é IGNORADO, não vira 400: filtro de faceta chega de link
// compartilhado e de botão de UI antiga, e derrubar a busca inteira por um
// parâmetro obsoleto é pior que ignorá-lo.
func parseAttrFilters(c *gin.Context) []attrFilter {
	out := make([]attrFilter, 0, 4)

	collect := func(param, op string) {
		for _, raw := range c.QueryArray(param) {
			if len(out) >= maxAttrFilters {
				return
			}
			key, val, ok := strings.Cut(truncateRunes(raw, maxFilterAttr), ":")
			if !ok || !attrKeyRe.MatchString(key) || val == "" {
				continue
			}
			f := attrFilter{Key: key, Str: val, Op: op}
			if n, err := strconv.ParseFloat(val, 64); err == nil {
				f.Num = &n
			}
			// Faixa numérica só faz sentido com número. `attr_min=cor:Azul` é
			// um erro de quem chamou, não um filtro textual disfarçado.
			if op != "eq" && f.Num == nil {
				continue
			}
			out = append(out, f)
		}
	}

	collect("attr", "eq")
	collect("attr_min", "gte")
	collect("attr_max", "lte")
	return out
}

// appendAttrFilters traduz os filtros de atributo em EXISTS correlacionados.
// Devolve where/args/idx atualizados pra encaixar no builder de List/Facets.
func appendAttrFilters(where []string, args []any, idx int, params productsQuery) ([]string, []any, int) {
	for _, f := range params.Attrs {
		var cond string
		switch f.Op {
		case "gte":
			cond = fmt.Sprintf("pa.value_num >= $%d", idx+1)
			args = append(args, f.Key, *f.Num)
		case "lte":
			cond = fmt.Sprintf("pa.value_num <= $%d", idx+1)
			args = append(args, f.Key, *f.Num)
		default:
			if f.Num != nil {
				// Casa pelos dois lados: "2.5" tanto como número quanto como
				// texto, porque o mesmo atributo pode estar cadastrado como
				// number numa categoria e text noutra.
				cond = fmt.Sprintf("(pa.value_num = $%d OR pa.value_text = $%d::text)", idx+1, idx+2)
				args = append(args, f.Key, *f.Num, f.Str)
			} else {
				cond = fmt.Sprintf("pa.value_text = $%d", idx+1)
				args = append(args, f.Key, f.Str)
			}
		}

		// #nosec G202 — `cond` é literal hardcoded com placeholders posicionais;
		// chave e valor entram por `args`.
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM product_attributes pa WHERE pa.product_id = p.id AND pa.key = $%d AND %s)",
			idx, cond))

		idx += 2
		if f.Op == "eq" && f.Num != nil {
			idx++ // o caso numérico consome um placeholder extra (num + texto)
		}
	}
	return where, args, idx
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
		// SKU e barcode são lookup EXATO (`=`), nunca ILIKE: é leitura de
		// scanner e digitação de código no balcão — precisa bater o índice
		// único e voltar em microssegundos, não varrer trigrama.
		SKU:     truncateRunes(strings.TrimSpace(c.Query("sku")), maxFilterCode),
		Barcode: truncateRunes(strings.TrimSpace(c.Query("barcode")), maxFilterCode),
		Attrs:   parseAttrFilters(c),
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

// scanProduct lê a projeção `productColumns` — a ordem dos destinos TEM que
// bater com a ordem das colunas lá.
func scanProduct(row scanner) (model.Product, error) {
	var p model.Product
	err := row.Scan(
		&p.ID, &p.Slug, &p.Name, &p.Category, &p.Price, &p.OriginalPrice, &p.Currency, &p.Icon, &p.Brand,
		&p.Seller, &p.SellerID, &p.SellerRating, &p.SellerReviewCt,
		&p.Stock, &p.Rating, &p.ReviewCount, &p.CashbackAmount, &p.Badge, &p.BadgeLabel, &p.Installments,
		&p.Description, &p.Specs, &p.CreatedAt, &p.UpdatedAt,
		&p.SKU, &p.Barcode, &p.UnitOfMeasure, &p.QtyStep,
		&p.WeightKg, &p.LengthCm, &p.WidthCm, &p.HeightCm,
	)
	return p, err
}

// loadTiers carrega as faixas de atacado de um produto.
//
// Só usada no DETALHE, nunca na listagem: 24 cards × 1 query cada seria N+1 por
// uma informação que o card não mostra. Quando a vitrine quiser exibir "a
// partir de R$ X" no card, o caminho é um LEFT JOIN LATERAL na query de
// listagem, não este helper em loop.
func loadTiers(db *sql.DB, productID string) ([]model.PriceTier, error) {
	rows, err := db.Query(`
		SELECT min_qty, price FROM product_price_tiers
		WHERE product_id = $1 ORDER BY min_qty ASC
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.PriceTier
	for rows.Next() {
		var t model.PriceTier
		if err := rows.Scan(&t.MinQty, &t.Price); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadAttributes junta os valores tipados do produto com o registry da
// categoria (label/unidade). Atributo gravado que não está mais no registry
// fica de fora — o registry é a fonte da verdade do que a UI sabe exibir.
func loadAttributes(db *sql.DB, productID, categoryID string) ([]model.ProductAttribute, error) {
	rows, err := db.Query(`
		SELECT ca.key, ca.label, ca.data_type, ca.unit, pa.value_num, pa.value_text, pa.value_bool
		FROM product_attributes pa
		JOIN category_attributes ca ON ca.key = pa.key AND ca.category_id = $2
		WHERE pa.product_id = $1
		ORDER BY ca.sort_order ASC, ca.key ASC
	`, productID, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ProductAttribute
	for rows.Next() {
		var a model.ProductAttribute
		if err := rows.Scan(&a.Key, &a.Label, &a.DataType, &a.Unit, &a.ValueNum, &a.ValueText, &a.ValueBool); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// enrichDetail anexa faixas de preço e atributos ao produto do detalhe.
//
// Falha aqui NÃO derruba a resposta: o produto ainda é vendável sem faixa de
// atacado e sem ficha técnica estruturada. Logamos e seguimos — devolver 500
// numa página de produto por causa de um atributo seria trocar uma degradação
// invisível por uma venda perdida.
func (h *ProductHandler) enrichDetail(c *gin.Context, p *model.Product) {
	if tiers, err := loadTiers(h.db, p.ID); err != nil {
		logDetailEnrichFailure(c, p.ID, "price_tiers", err)
	} else {
		p.PriceTiers = tiers
	}
	if attrs, err := loadAttributes(h.db, p.ID, p.Category); err != nil {
		logDetailEnrichFailure(c, p.ID, "attributes", err)
	} else {
		p.Attributes = attrs
	}
}

// loadImages substitui o padrão anterior (erro do Query ignorado e rows.Err()
// nunca checado), que podia devolver uma galeria truncada como se fosse a
// galeria completa.
func (h *ProductHandler) loadImages(c *gin.Context, p *model.Product) {
	rows, err := h.db.Query(`
		SELECT id, url, alt, variants FROM product_images WHERE product_id=$1 ORDER BY sort_order ASC
	`, p.ID)
	if err != nil {
		logDetailEnrichFailure(c, p.ID, "images", err)
		return
	}
	defer rows.Close()

	var imgs []model.ProductImage
	for rows.Next() {
		var im model.ProductImage
		var rawVariants []byte
		if err := rows.Scan(&im.ID, &im.URL, &im.Alt, &rawVariants); err != nil {
			logDetailEnrichFailure(c, p.ID, "images", err)
			return
		}
		// Imagem própria: `url` do banco guarda a CHAVE, não uma URL. Resolver
		// aqui é o que mantém o banco livre de URL absoluta. Imagem externa já
		// vem com http(s) e o resolver a devolve intacta.
		im.Variants = h.imageVariants(rawVariants)
		if im.Variants != nil {
			// O detalhe usa a média no carrossel; o zoom pede a grande via
			// `variants.large`. Mandar a grande em `url` faria o slide baixar
			// 1600px que a tela não usa.
			im.URL = im.Variants.Medium
		}
		imgs = append(imgs, im)
	}
	if err := rows.Err(); err != nil {
		logDetailEnrichFailure(c, p.ID, "images", err)
		return
	}
	p.Images = imgs
}

// loadThumbnails preenche a imagem de capa dos produtos de UMA página de
// listagem, em uma única query.
//
// PORQUÊ existe: a vitrine (List) não devolvia imagem nenhuma — só o detalhe
// devolvia. O card do produto caía no emoji da categoria, e ninguém compra uma
// furadeira de R$ 429 olhando "⚒". Foto na vitrine é o que vende.
//
// PORQUÊ em lote: chamar loadImages por produto seria N+1 (100 itens = 100
// queries). Aqui é uma query com `= ANY($1)`, e o resultado é distribuído em
// memória.
//
// Só a capa (menor sort_order) entra: a galeria completa continua exclusiva do
// detalhe. Mandar 5 imagens de 100 produtos inflaria a resposta da listagem sem
// que o card use nenhuma delas.
//
// Falha aqui é degradação, não erro: sem foto o card cai no ícone, que é o
// comportamento de antes. Listagem inteira não pode falhar por causa da capa.
func (h *ProductHandler) loadThumbnails(c *gin.Context, products []model.Product) {
	if len(products) == 0 {
		return
	}

	ids := make([]string, 0, len(products))
	for i := range products {
		ids = append(ids, products[i].ID)
	}

	rows, err := h.db.Query(`
		SELECT DISTINCT ON (product_id) product_id, url, alt, variants
		FROM product_images
		WHERE product_id = ANY($1)
		ORDER BY product_id, sort_order ASC
	`, pq.Array(ids))
	if err != nil {
		slog.Error("product.list_thumbnails_failed",
			"request_id", c.GetString("request_id"), "error", err.Error())
		return
	}
	defer rows.Close()

	capas := make(map[string]model.ProductImage, len(products))
	for rows.Next() {
		var pid string
		var im model.ProductImage
		var rawVariants []byte
		if err := rows.Scan(&pid, &im.URL, &im.Alt, &rawVariants); err != nil {
			slog.Error("product.list_thumbnails_failed",
				"request_id", c.GetString("request_id"), "error", err.Error())
			return
		}
		// A VITRINE SERVE A MINIATURA. Este `im.URL = Thumb` é a linha que
		// decide se a listagem no celular carrega em 1s ou em 20s: são ~20
		// cards por página, e servir a imagem de zoom em cada um são megabytes
		// para desenhar 150px de tela.
		im.Variants = h.imageVariants(rawVariants)
		if im.Variants != nil {
			im.URL = im.Variants.Thumb
		}
		capas[pid] = im
	}
	if err := rows.Err(); err != nil {
		slog.Error("product.list_thumbnails_failed",
			"request_id", c.GetString("request_id"), "error", err.Error())
		return
	}

	for i := range products {
		if im, ok := capas[products[i].ID]; ok {
			products[i].Images = []model.ProductImage{im}
		}
	}
}

func logDetailEnrichFailure(c *gin.Context, productID, part string, err error) {
	slog.Error("product.detail_enrich_failed",
		"product_id", productID, "part", part,
		"request_id", c.GetString("request_id"), "error", err.Error())
}
