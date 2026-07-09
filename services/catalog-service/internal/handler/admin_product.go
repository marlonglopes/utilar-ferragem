// Admin (ingestão de produtos) — rotas de escrita protegidas por RequireAdmin.
//
// Cobre o "mecanismo de ingerir produtos e expor pra venda" (Fase A):
//   - CRUD de produto (preço, estoque, specs, descrição, categoria)
//   - imagens por URL (MVP — sem upload S3 ainda)
//   - workflow de publicação (draft → published → archived)
//   - importação em massa via CSV (upsert por SKU)
//
// Imagens: nesta fase são URLs (o lojista fornece o link, ou a planilha traz).
// O pipeline de upload S3/CloudFront vem na Fase B.
package handler

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxImportBody — teto do corpo do CSV (2 MB ~ dezenas de milhares de linhas).
const maxImportBody = 2 << 20

type AdminProductHandler struct {
	db *sql.DB
}

func NewAdminProductHandler(db *sql.DB) *AdminProductHandler {
	return &AdminProductHandler{db: db}
}

// productInput é o corpo aceito em create/update. Campos ponteiro = opcionais
// no PATCH (nil = não mexe).
type productInput struct {
	SKU           *string          `json:"sku"`
	Slug          *string          `json:"slug"`
	Name          *string          `json:"name"`
	CategoryID    *string          `json:"category"`
	SellerID      *string          `json:"sellerId"`
	Price         *float64         `json:"price"`
	OriginalPrice *float64         `json:"originalPrice"`
	Icon          *string          `json:"icon"`
	Brand         *string          `json:"brand"`
	Stock         *int             `json:"stock"`
	Description   *string          `json:"description"`
	Specs         *json.RawMessage `json:"specs"`
	Status        *string          `json:"status"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify gera um slug URL-safe a partir do nome. Exportado pra reuso em testes.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i",
		"ó", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
	).Replace(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func validStatus(s string) bool {
	return s == "draft" || s == "published" || s == "archived"
}

// Create — POST /api/v1/admin/products. Cria um produto novo (default status=draft).
func (h *AdminProductHandler) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBody)
	var in productInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if in.Name == nil || *in.Name == "" {
		BadRequest(c, "name is required")
		return
	}
	if in.CategoryID == nil || *in.CategoryID == "" {
		BadRequest(c, "category is required")
		return
	}
	if in.Price == nil || *in.Price < 0 {
		BadRequest(c, "price is required and must be >= 0")
		return
	}

	slug := ""
	if in.Slug != nil && *in.Slug != "" {
		slug = Slugify(*in.Slug)
	} else {
		slug = Slugify(*in.Name)
	}
	if slug == "" {
		BadRequest(c, "could not derive slug from name/slug")
		return
	}

	status := "draft"
	if in.Status != nil {
		if !validStatus(*in.Status) {
			BadRequest(c, "status must be draft|published|archived")
			return
		}
		status = *in.Status
	}

	// Valida FKs (categoria e vendedor) com mensagem clara em vez de erro genérico de FK.
	if !h.exists(c, "categories", *in.CategoryID) {
		BadRequest(c, fmt.Sprintf("category %q does not exist", *in.CategoryID))
		return
	}
	sellerID := ""
	if in.SellerID != nil && *in.SellerID != "" {
		if !h.exists(c, "sellers", *in.SellerID) {
			BadRequest(c, fmt.Sprintf("seller %q does not exist", *in.SellerID))
			return
		}
		sellerID = *in.SellerID
	} else {
		// Sem vendedor explícito → usa o primeiro cadastrado (loja própria Utilar).
		if err := h.db.QueryRow(`SELECT id FROM sellers ORDER BY created_at LIMIT 1`).Scan(&sellerID); err != nil {
			BadRequest(c, "no seller available; create a seller first or pass sellerId")
			return
		}
	}

	specs := json.RawMessage(`{}`)
	if in.Specs != nil {
		specs = *in.Specs
	}
	icon := "package"
	if in.Icon != nil && *in.Icon != "" {
		icon = *in.Icon
	}

	var id string
	err := h.db.QueryRow(`
		INSERT INTO products (sku, slug, name, category_id, seller_id, price, original_price,
		                      icon, brand, stock, description, specs, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE($10,0),$11,$12,$13)
		RETURNING id
	`, in.SKU, slug, *in.Name, *in.CategoryID, sellerID, *in.Price, in.OriginalPrice,
		icon, in.Brand, in.Stock, in.Description, []byte(specs), status).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(c, "product with this slug or sku already exists")
			return
		}
		DBError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": slug, "status": status})
}

// Patch — PATCH /api/v1/admin/products/by-id/:id. Atualização parcial.
// O caso de uso mais comum: ajustar preço / estoque / publicar.
func (h *AdminProductHandler) Patch(c *gin.Context) {
	id := c.Param("id")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBody)
	var in productInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}

	set := []string{}
	args := []any{}
	idx := 1
	add := func(col string, val any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, val)
		idx++
	}

	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.Price != nil {
		if *in.Price < 0 {
			BadRequest(c, "price must be >= 0")
			return
		}
		add("price", *in.Price)
	}
	if in.OriginalPrice != nil {
		add("original_price", *in.OriginalPrice)
	}
	if in.Stock != nil {
		if *in.Stock < 0 {
			BadRequest(c, "stock must be >= 0")
			return
		}
		add("stock", *in.Stock)
	}
	if in.Brand != nil {
		add("brand", *in.Brand)
	}
	if in.Description != nil {
		add("description", *in.Description)
	}
	if in.Icon != nil {
		add("icon", *in.Icon)
	}
	if in.Specs != nil {
		add("specs", []byte(*in.Specs))
	}
	if in.CategoryID != nil {
		if !h.exists(c, "categories", *in.CategoryID) {
			BadRequest(c, fmt.Sprintf("category %q does not exist", *in.CategoryID))
			return
		}
		add("category_id", *in.CategoryID)
	}
	if in.Status != nil {
		if !validStatus(*in.Status) {
			BadRequest(c, "status must be draft|published|archived")
			return
		}
		add("status", *in.Status)
	}
	if in.SKU != nil {
		add("sku", *in.SKU)
	}

	if len(set) == 0 {
		BadRequest(c, "no fields to update")
		return
	}
	set = append(set, "updated_at = now()")
	args = append(args, id)

	// #nosec G201 — `set` é montado só de nomes de coluna hardcoded (`price = $N`),
	// valores entram por placeholders posicionais. Atacante não controla SQL.
	q := fmt.Sprintf("UPDATE products SET %s WHERE id = $%d", strings.Join(set, ", "), idx)
	res, err := h.db.Exec(q, args...)
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(c, "sku already in use")
			return
		}
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "product not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "updated": true})
}

// Delete — DELETE /api/v1/admin/products/by-id/:id. Soft-delete: arquiva (some
// da vitrine mas preserva histórico de pedidos que referenciam o produto).
func (h *AdminProductHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	res, err := h.db.Exec(`UPDATE products SET status='archived', updated_at=now() WHERE id=$1`, id)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "product not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "archived"})
}

type imageInput struct {
	URL       string `json:"url"`
	Alt       string `json:"alt"`
	SortOrder int    `json:"sortOrder"`
}

// AddImage — POST /api/v1/admin/products/by-id/:id/images. Adiciona imagem por URL.
func (h *AdminProductHandler) AddImage(c *gin.Context) {
	id := c.Param("id")
	var in imageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		BadRequest(c, "url must be an absolute http(s) URL")
		return
	}
	if !h.exists(c, "products", id) {
		NotFound(c, "product not found")
		return
	}
	var imgID string
	err := h.db.QueryRow(`
		INSERT INTO product_images (product_id, url, alt, sort_order)
		VALUES ($1,$2,$3,$4) RETURNING id
	`, id, in.URL, in.Alt, in.SortOrder).Scan(&imgID)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": imgID})
}

// DeleteImage — DELETE /api/v1/admin/products/by-id/:id/images/:imageId.
func (h *AdminProductHandler) DeleteImage(c *gin.Context) {
	res, err := h.db.Exec(`DELETE FROM product_images WHERE id=$1 AND product_id=$2`,
		c.Param("imageId"), c.Param("id"))
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "image not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// --- Importação CSV --------------------------------------------------------

// ImportResult é o relatório devolvido ao lojista após o upload.
type ImportResult struct {
	Total   int              `json:"total"`
	Created int              `json:"created"`
	Updated int              `json:"updated"`
	Failed  int              `json:"failed"`
	Errors  []ImportRowError `json:"errors"`
}

type ImportRowError struct {
	Line    int    `json:"line"`
	SKU     string `json:"sku,omitempty"`
	Message string `json:"message"`
}

// Import — POST /api/v1/admin/products/import (text/csv ou multipart "file").
//
// Colunas esperadas (header, ordem livre): sku, name, category, price, stock,
// brand, description, image_url, status. `sku` é a chave de upsert.
//
// Cada linha é independente: erro numa linha não aborta as outras (o relatório
// lista o que falhou). Publica por padrão (status=published) salvo indicação.
func (h *AdminProductHandler) Import(c *gin.Context) {
	reader, err := h.importReader(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	cr := csv.NewReader(reader)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		BadRequest(c, "could not read CSV header")
		return
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	if _, ok := col["name"]; !ok {
		BadRequest(c, "CSV must have at least a 'name' column")
		return
	}

	// Vendedor default (loja própria) pra linhas sem seller.
	var defaultSeller string
	_ = h.db.QueryRow(`SELECT id FROM sellers ORDER BY created_at LIMIT 1`).Scan(&defaultSeller)

	result := ImportResult{Errors: []ImportRowError{}}
	line := 1 // header é linha 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportRowError{Line: line, Message: "malformed row: " + err.Error()})
			continue
		}
		result.Total++

		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}

		row := importRow{
			sku:         get("sku"),
			name:        get("name"),
			category:    get("category"),
			brand:       get("brand"),
			description: get("description"),
			imageURL:    get("image_url"),
			status:      get("status"),
			priceStr:    get("price"),
			stockStr:    get("stock"),
		}

		created, err := h.upsertRow(row, defaultSeller)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportRowError{Line: line, SKU: row.sku, Message: err.Error()})
			continue
		}
		if created {
			result.Created++
		} else {
			result.Updated++
		}
	}

	c.JSON(http.StatusOK, result)
}

type importRow struct {
	sku, name, category, brand, description, imageURL, status, priceStr, stockStr string
}

// upsertRow insere ou atualiza um produto pela chave SKU. Retorna created=true
// se foi um insert. Toda a linha roda numa transação (produto + imagem juntos).
func (h *AdminProductHandler) upsertRow(row importRow, defaultSeller string) (created bool, err error) {
	if row.name == "" {
		return false, fmt.Errorf("name is required")
	}
	if row.category == "" {
		return false, fmt.Errorf("category is required")
	}
	price, err := parseMoney(row.priceStr)
	if err != nil {
		return false, fmt.Errorf("invalid price %q", row.priceStr)
	}
	stock := 0
	if row.stockStr != "" {
		stock, err = strconv.Atoi(row.stockStr)
		if err != nil || stock < 0 {
			return false, fmt.Errorf("invalid stock %q", row.stockStr)
		}
	}
	status := "published"
	if row.status != "" {
		if !validStatus(row.status) {
			return false, fmt.Errorf("invalid status %q", row.status)
		}
		status = row.status
	}

	tx, err := h.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// Categoria precisa existir (FK). Mensagem clara em vez de erro de FK cru.
	var catOK bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1)`, row.category).Scan(&catOK); err != nil {
		return false, err
	}
	if !catOK {
		return false, fmt.Errorf("category %q does not exist", row.category)
	}

	slug := Slugify(row.name)
	var brand *string
	if row.brand != "" {
		brand = &row.brand
	}
	var desc *string
	if row.description != "" {
		desc = &row.description
	}

	// Upsert por SKU quando presente; senão por slug.
	var productID string
	if row.sku != "" {
		err = tx.QueryRow(`
			INSERT INTO products (sku, slug, name, category_id, seller_id, price, icon, brand, stock, description, status)
			VALUES ($1,$2,$3,$4,$5,$6,'package',$7,$8,$9,$10)
			ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE SET
				name=EXCLUDED.name, category_id=EXCLUDED.category_id, price=EXCLUDED.price,
				brand=EXCLUDED.brand, stock=EXCLUDED.stock, description=EXCLUDED.description,
				status=EXCLUDED.status, updated_at=now()
			RETURNING id, (xmax = 0) AS inserted
		`, row.sku, slug, row.name, row.category, defaultSeller, price, brand, stock, desc, status).Scan(&productID, &created)
	} else {
		err = tx.QueryRow(`
			INSERT INTO products (slug, name, category_id, seller_id, price, icon, brand, stock, description, status)
			VALUES ($1,$2,$3,$4,$5,'package',$6,$7,$8,$9)
			ON CONFLICT (slug) DO UPDATE SET
				name=EXCLUDED.name, category_id=EXCLUDED.category_id, price=EXCLUDED.price,
				brand=EXCLUDED.brand, stock=EXCLUDED.stock, description=EXCLUDED.description,
				status=EXCLUDED.status, updated_at=now()
			RETURNING id, (xmax = 0) AS inserted
		`, slug, row.name, row.category, defaultSeller, price, brand, stock, desc, status).Scan(&productID, &created)
	}
	if err != nil {
		return false, err
	}

	// Imagem por URL (opcional). Só adiciona se ainda não existir a mesma URL.
	if row.imageURL != "" && (strings.HasPrefix(row.imageURL, "http://") || strings.HasPrefix(row.imageURL, "https://")) {
		if _, err = tx.Exec(`
			INSERT INTO product_images (product_id, url, alt, sort_order)
			SELECT $1,$2,$3,0
			WHERE NOT EXISTS (SELECT 1 FROM product_images WHERE product_id=$1 AND url=$2)
		`, productID, row.imageURL, row.name); err != nil {
			return false, err
		}
	}

	return created, tx.Commit()
}

// --- helpers ---------------------------------------------------------------

func (h *AdminProductHandler) importReader(c *gin.Context) (io.Reader, error) {
	ct := c.ContentType()
	if strings.HasPrefix(ct, "multipart/form-data") {
		f, _, err := c.Request.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("multipart upload requires a 'file' field")
		}
		return io.LimitReader(f, maxImportBody), nil
	}
	// text/csv ou octet-stream — corpo cru.
	return io.LimitReader(c.Request.Body, maxImportBody), nil
}

func (h *AdminProductHandler) exists(c *gin.Context, table, id string) bool {
	// #nosec G201 — `table` vem só de literais internos ("categories","sellers","products").
	var ok bool
	q := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id=$1)`, table)
	if err := h.db.QueryRow(q, id).Scan(&ok); err != nil {
		return false
	}
	return ok
}

// parseMoney aceita "1234.56", "1234,56" e "R$ 1.234,56".
func parseMoney(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.TrimSpace(s)
	// Se tem vírgula, assume formato BR: remove pontos de milhar, vírgula vira ponto.
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative")
	}
	return v, nil
}

// isUniqueViolation detecta o SQLSTATE 23505 do Postgres sem depender do driver.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
