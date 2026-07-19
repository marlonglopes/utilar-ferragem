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
	"errors"
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
	SKU           *string  `json:"sku"`
	Slug          *string  `json:"slug"`
	Name          *string  `json:"name"`
	CategoryID    *string  `json:"category"`
	SellerID      *string  `json:"sellerId"`
	Price         *float64 `json:"price"`
	OriginalPrice *float64 `json:"originalPrice"`
	Icon          *string  `json:"icon"`
	Brand         *string  `json:"brand"`
	// Stock é float64 desde a migration 005 — a loja tem 1,5 m³ de areia.
	Stock       *float64         `json:"stock"`
	Description *string          `json:"description"`
	Specs       *json.RawMessage `json:"specs"`
	Status      *string          `json:"status"`

	// --- domínio de ferragem ------------------------------------------------
	// Cost NUNCA é devolvido em rota pública. Aqui é ENTRADA de rota admin.
	Cost          *float64 `json:"cost"`
	UnitOfMeasure *string  `json:"unitOfMeasure"`
	QtyStep       *float64 `json:"qtyStep"`
	Barcode       *string  `json:"barcode"`
	WeightKg      *float64 `json:"weightKg"`
	LengthCm      *float64 `json:"lengthCm"`
	WidthCm       *float64 `json:"widthCm"`
	HeightCm      *float64 `json:"heightCm"`
	SupplierID    *string  `json:"supplierId"`
	SupplierSKU   *string  `json:"supplierSku"`
	NCM           *string  `json:"ncm"`
	CFOP          *string  `json:"cfop"`
	CEST          *string  `json:"cest"`
	Origem        *int     `json:"origem"`
}

// validate roda as regras que valem tanto no create quanto no patch, e
// NORMALIZA os campos que têm forma canônica (unidade minúscula, barcode sem
// pontuação). Chamar antes de montar o SQL.
//
// Retorna a primeira violação: o lojista corrige uma por vez de qualquer jeito,
// e acumular erros aqui obrigaria a um formato de resposta que o
// {error,code,requestId} da casa não tem.
func (in *productInput) validate() error {
	if in.Price != nil {
		if err := validateMoney("price", *in.Price); err != nil {
			return err
		}
	}
	if in.OriginalPrice != nil {
		if err := validateMoney("originalPrice", *in.OriginalPrice); err != nil {
			return err
		}
	}
	if in.Cost != nil {
		if err := validateMoney("cost", *in.Cost); err != nil {
			return err
		}
	}
	if in.Stock != nil && *in.Stock < 0 {
		return fmt.Errorf("stock must be >= 0")
	}
	if in.QtyStep != nil {
		if err := validateQtyStep(*in.QtyStep); err != nil {
			return err
		}
	}
	if in.UnitOfMeasure != nil {
		u, err := normalizeUnit(*in.UnitOfMeasure)
		if err != nil {
			return err
		}
		in.UnitOfMeasure = &u
	}
	if in.Barcode != nil {
		b, err := validateBarcode(*in.Barcode)
		if err != nil {
			return err
		}
		in.Barcode = &b
	}
	for _, d := range []struct {
		name string
		v    *float64
	}{
		{"weightKg", in.WeightKg}, {"lengthCm", in.LengthCm},
		{"widthCm", in.WidthCm}, {"heightCm", in.HeightCm},
	} {
		if d.v != nil {
			if err := validateDimension(d.name, *d.v); err != nil {
				return err
			}
		}
	}
	return validateFiscal(in.NCM, in.CFOP, in.CEST, in.Origem)
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
	if in.Price == nil {
		BadRequest(c, "price is required")
		return
	}
	if err := in.validate(); err != nil {
		BadRequest(c, err.Error())
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
		                      icon, brand, stock, description, specs, status,
		                      cost, unit_of_measure, qty_step, barcode,
		                      weight_kg, length_cm, width_cm, height_cm,
		                      supplier_id, supplier_sku, ncm, cfop, cest, origem)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE($10,0),$11,$12,$13,
		        $14, COALESCE($15,'un'), COALESCE($16,1), $17,
		        $18,$19,$20,$21,
		        $22,$23,$24,$25,$26,$27)
		RETURNING id
	`, in.SKU, slug, *in.Name, *in.CategoryID, sellerID, *in.Price, in.OriginalPrice,
		icon, in.Brand, in.Stock, in.Description, []byte(specs), status,
		in.Cost, in.UnitOfMeasure, in.QtyStep, nilIfEmptyStr(in.Barcode),
		in.WeightKg, in.LengthCm, in.WidthCm, in.HeightCm,
		in.SupplierID, in.SupplierSKU, in.NCM, in.CFOP, in.CEST, in.Origem).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			Conflict(c, "product with this slug, sku or barcode already exists")
			return
		}
		DBError(c, err)
		return
	}

	// Trilha: quem criou, com que preço/custo. O preço inicial entra no
	// histórico porque "sempre foi R$ 42,90" precisa ter começo registrado.
	audit(h.db, c, "product.create", "product", id, AuditChanges{
		"name":   {Old: nil, New: *in.Name},
		"price":  {Old: nil, New: *in.Price},
		"status": {Old: nil, New: status},
	})
	recordPriceChange(h.db, c, id, 0, *in.Price, nil, in.Cost, "admin")

	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": slug, "status": status})
}

// nilIfEmptyStr: barcode "" precisa virar NULL, não string vazia — o índice
// único parcial (`WHERE barcode IS NOT NULL`) trataria dois produtos com ""
// como colisão e o segundo cadastro falharia sem motivo aparente.
func nilIfEmptyStr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
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

	if err := in.validate(); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Lê o estado anterior ANTES de escrever: é o "de" do "de → para" da
	// auditoria e do histórico de preço. Sem isso a trilha registra só o valor
	// novo, que é exatamente a metade inútil da informação quando se investiga
	// um preço errado.
	before, err := h.loadForAudit(id)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	set := []string{}
	args := []any{}
	idx := 1
	changes := AuditChanges{}
	add := func(col string, val any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, val)
		idx++
	}

	if in.Name != nil {
		changes.changed("name", before.Name, *in.Name)
		add("name", *in.Name)
	}
	if in.Price != nil {
		changes.changed("price", before.Price, *in.Price)
		add("price", *in.Price)
	}
	if in.Cost != nil {
		changes.changed("cost", before.Cost, *in.Cost)
		add("cost", *in.Cost)
	}
	if in.OriginalPrice != nil {
		add("original_price", *in.OriginalPrice)
	}
	if in.Stock != nil {
		changes.changed("stock", before.Stock, *in.Stock)
		add("stock", *in.Stock)
	}
	if in.UnitOfMeasure != nil {
		changes.changed("unit_of_measure", before.UnitOfMeasure, *in.UnitOfMeasure)
		add("unit_of_measure", *in.UnitOfMeasure)
	}
	if in.QtyStep != nil {
		add("qty_step", *in.QtyStep)
	}
	if in.Barcode != nil {
		changes.changed("barcode", before.Barcode, *in.Barcode)
		add("barcode", nilIfEmptyStr(in.Barcode))
	}
	if in.WeightKg != nil {
		add("weight_kg", *in.WeightKg)
	}
	if in.LengthCm != nil {
		add("length_cm", *in.LengthCm)
	}
	if in.WidthCm != nil {
		add("width_cm", *in.WidthCm)
	}
	if in.HeightCm != nil {
		add("height_cm", *in.HeightCm)
	}
	if in.SupplierID != nil {
		add("supplier_id", *in.SupplierID)
	}
	if in.SupplierSKU != nil {
		add("supplier_sku", *in.SupplierSKU)
	}
	if in.NCM != nil {
		add("ncm", nilIfEmptyStr(in.NCM))
	}
	if in.CFOP != nil {
		add("cfop", nilIfEmptyStr(in.CFOP))
	}
	if in.CEST != nil {
		add("cest", nilIfEmptyStr(in.CEST))
	}
	if in.Origem != nil {
		add("origem", *in.Origem)
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
		changes.changed("status", before.Status, *in.Status)
		add("status", *in.Status)
	}
	if in.SKU != nil {
		changes.changed("sku", before.SKU, *in.SKU)
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
			Conflict(c, "sku or barcode already in use")
			return
		}
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "product not found")
		return
	}

	audit(h.db, c, "product.update", "product", id, changes)

	// Histórico só quando preço OU custo mudou de verdade. Um PATCH que só
	// ajusta a descrição não pode poluir a série que alimenta o alerta de
	// queda percentual (o detector do erro de vírgula na importação).
	newPrice, newCost := before.Price, before.Cost
	if in.Price != nil {
		newPrice = *in.Price
	}
	if in.Cost != nil {
		newCost = in.Cost
	}
	if newPrice != before.Price || !sameValue(newCost, before.Cost) {
		recordPriceChange(h.db, c, id, before.Price, newPrice, before.Cost, newCost, "admin")
	}

	c.JSON(http.StatusOK, gin.H{"id": id, "updated": true})
}

// productSnapshot é o "antes" usado pela auditoria. Deliberadamente pequeno:
// só os campos cuja mudança alguém vai querer investigar depois.
type productSnapshot struct {
	Name          string
	Price         float64
	Cost          *float64
	Stock         float64
	Status        string
	SKU           *string
	Barcode       *string
	UnitOfMeasure string
}

func (h *AdminProductHandler) loadForAudit(id string) (productSnapshot, error) {
	var s productSnapshot
	err := h.db.QueryRow(`
		SELECT name, price, cost, stock, status, sku, barcode, unit_of_measure
		FROM products WHERE id = $1
	`, id).Scan(&s.Name, &s.Price, &s.Cost, &s.Stock, &s.Status, &s.SKU, &s.Barcode, &s.UnitOfMeasure)
	return s, err
}

// Delete — DELETE /api/v1/admin/products/by-id/:id. Soft-delete: arquiva (some
// da vitrine mas preserva histórico de pedidos que referenciam o produto).
func (h *AdminProductHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	// RETURNING o status anterior: "quem arquivou" vale pouco sem "arquivou o
	// que estava publicado" (tirar da vitrine) vs "arquivou um rascunho".
	// A CTE `old` é avaliada sobre o snapshot da consulta, então enxerga o
	// status ANTES do UPDATE — ler numa segunda query abriria janela pra outro
	// admin arquivar no meio e a trilha registrar o valor errado.
	var prevStatus string
	err := h.db.QueryRow(`
		WITH old AS (SELECT status FROM products WHERE id = $1)
		UPDATE products SET status='archived', updated_at=now()
		WHERE id = $1
		RETURNING (SELECT status FROM old)`, id).Scan(&prevStatus)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	audit(h.db, c, "product.archive", "product", id, AuditChanges{
		"status": {Old: prevStatus, New: "archived"},
	})
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
			// Colunas de ferragem. Todas opcionais: planilha de fornecedor que
			// não traz custo continua importando, só sem trava de margem.
			costStr:     get("cost"),
			unit:        get("unit"),
			barcode:     get("barcode"),
			weightStr:   get("weight_kg"),
			supplierID:  get("supplier_id"),
			supplierSKU: get("supplier_sku"),
			ncm:         get("ncm"),
			cfop:        get("cfop"),
		}

		created, err := h.upsertRow(c, row, defaultSeller)
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

	// Uma linha de auditoria pro LOTE, não por produto: as linhas individuais
	// já viram histórico de preço, e 4.000 eventos "product.update" afogariam
	// a trilha justamente quando ela mais importa (achar a importação errada).
	audit(h.db, c, "product.import", "product", "", AuditChanges{
		"total":   {Old: nil, New: result.Total},
		"created": {Old: nil, New: result.Created},
		"updated": {Old: nil, New: result.Updated},
		"failed":  {Old: nil, New: result.Failed},
	})

	c.JSON(http.StatusOK, result)
}

type importRow struct {
	sku, name, category, brand, description, imageURL, status, priceStr, stockStr string
	costStr, unit, barcode, weightStr, supplierID, supplierSKU, ncm, cfop         string
}

// upsertRow insere ou atualiza um produto pela chave SKU. Retorna created=true
// se foi um insert. Toda a linha roda numa transação (produto + imagem juntos).
func (h *AdminProductHandler) upsertRow(c *gin.Context, row importRow, defaultSeller string) (created bool, err error) {
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
	if err := validateMoney("price", price); err != nil {
		return false, err
	}

	// Custo é opcional na planilha, mas quando vem tem que ser válido — custo
	// errado é pior que custo ausente, porque a trava de margem passa a mentir
	// com aparência de precisão.
	var cost *float64
	if row.costStr != "" {
		v, err := parseMoney(row.costStr)
		if err != nil {
			return false, fmt.Errorf("invalid cost %q", row.costStr)
		}
		if err := validateMoney("cost", v); err != nil {
			return false, err
		}
		cost = &v
	}

	// Estoque agora é fracionário (2,5 m de cabo). parseMoney já lida com o
	// decimal brasileiro ("2,5"), que é como a planilha vem.
	stock := 0.0
	if row.stockStr != "" {
		stock, err = parseMoney(row.stockStr)
		if err != nil || stock < 0 {
			return false, fmt.Errorf("invalid stock %q", row.stockStr)
		}
	}

	unit := "un"
	if row.unit != "" {
		if unit, err = normalizeUnit(row.unit); err != nil {
			return false, err
		}
	}

	barcode, err := validateBarcode(row.barcode)
	if err != nil {
		return false, err
	}

	var weight *float64
	if row.weightStr != "" {
		v, err := parseMoney(row.weightStr)
		if err != nil {
			return false, fmt.Errorf("invalid weight_kg %q", row.weightStr)
		}
		weight = &v
	}

	if err := validateFiscal(strPtrOrNil(row.ncm), strPtrOrNil(row.cfop), nil, nil); err != nil {
		return false, err
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

	// Preço/custo anteriores, pra que o histórico registre a variação da
	// importação. É esta série que denuncia o erro de vírgula ("de R$ 1.234,56
	// para R$ 1,23") — o modo de falha mais caro do catálogo.
	var oldPrice float64
	var oldCost *float64
	var hadRow bool
	if row.sku != "" {
		err = tx.QueryRow(`SELECT price, cost FROM products WHERE sku = $1`, row.sku).Scan(&oldPrice, &oldCost)
	} else {
		err = tx.QueryRow(`SELECT price, cost FROM products WHERE slug = $1`, slug).Scan(&oldPrice, &oldCost)
	}
	switch {
	case err == nil:
		hadRow = true
	case errors.Is(err, sql.ErrNoRows):
		// produto novo — sem "antes"
	default:
		return false, err
	}

	// COALESCE nas colunas opcionais do UPDATE: planilha que não traz a coluna
	// `cost` não pode ZERAR o custo que já estava cadastrado. Ausência na
	// planilha significa "não sei", nunca "apague" — a mesma regra do "nunca
	// apagar por ausência" documentada em docs/ingestao-de-produtos.md.
	const upsertTail = `
		ON CONFLICT (%s) %s DO UPDATE SET
			name=EXCLUDED.name, category_id=EXCLUDED.category_id, price=EXCLUDED.price,
			brand=EXCLUDED.brand, stock=EXCLUDED.stock, description=EXCLUDED.description,
			status=EXCLUDED.status,
			cost=COALESCE(EXCLUDED.cost, products.cost),
			unit_of_measure=EXCLUDED.unit_of_measure,
			barcode=COALESCE(EXCLUDED.barcode, products.barcode),
			weight_kg=COALESCE(EXCLUDED.weight_kg, products.weight_kg),
			supplier_id=COALESCE(EXCLUDED.supplier_id, products.supplier_id),
			supplier_sku=COALESCE(EXCLUDED.supplier_sku, products.supplier_sku),
			ncm=COALESCE(EXCLUDED.ncm, products.ncm),
			cfop=COALESCE(EXCLUDED.cfop, products.cfop),
			updated_at=now()
		RETURNING id, (xmax = 0) AS inserted`

	// Upsert por SKU quando presente; senão por slug.
	var productID string
	if row.sku != "" {
		// #nosec G201 — o único fragmento interpolado é o alvo do ON CONFLICT,
		// literal hardcoded acima.
		err = tx.QueryRow(`
			INSERT INTO products (sku, slug, name, category_id, seller_id, price, icon, brand, stock, description, status,
			                      cost, unit_of_measure, barcode, weight_kg, supplier_id, supplier_sku, ncm, cfop)
			VALUES ($1,$2,$3,$4,$5,$6,'package',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		`+fmt.Sprintf(upsertTail, "sku", "WHERE sku IS NOT NULL"),
			row.sku, slug, row.name, row.category, defaultSeller, price, brand, stock, desc, status,
			cost, unit, nilIfEmptyStr(&barcode), weight,
			nilIfEmptyStr(&row.supplierID), nilIfEmptyStr(&row.supplierSKU),
			nilIfEmptyStr(&row.ncm), nilIfEmptyStr(&row.cfop)).Scan(&productID, &created)
	} else {
		// #nosec G201 — idem.
		err = tx.QueryRow(`
			INSERT INTO products (slug, name, category_id, seller_id, price, icon, brand, stock, description, status,
			                      cost, unit_of_measure, barcode, weight_kg, supplier_id, supplier_sku, ncm, cfop)
			VALUES ($1,$2,$3,$4,$5,'package',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		`+fmt.Sprintf(upsertTail, "slug", ""),
			slug, row.name, row.category, defaultSeller, price, brand, stock, desc, status,
			cost, unit, nilIfEmptyStr(&barcode), weight,
			nilIfEmptyStr(&row.supplierID), nilIfEmptyStr(&row.supplierSKU),
			nilIfEmptyStr(&row.ncm), nilIfEmptyStr(&row.cfop)).Scan(&productID, &created)
	}
	if err != nil {
		return false, err
	}

	// Histórico dentro da MESMA transação do upsert: se a linha do produto
	// entrar, a linha de histórico entra junto. Trilha que pode divergir do
	// dado auditado não serve pra auditar.
	newCost := cost
	if newCost == nil {
		newCost = oldCost // COALESCE acima preservou o custo anterior
	}
	if !hadRow || price != oldPrice || !sameValue(newCost, oldCost) {
		recordPriceChange(tx, c, productID, oldPrice, price, oldCost, newCost, "import")
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

// strPtrOrNil converte string de CSV vazia em ausência de valor. Célula vazia
// na planilha significa "não informado", não "vazio".
func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
