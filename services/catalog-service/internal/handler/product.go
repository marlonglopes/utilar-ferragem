package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
)

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
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d OR COALESCE(p.description,'') ILIKE $%d OR s.name ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+params.Q+"%")
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

	orderBy := "p.created_at DESC"
	switch params.Sort {
	case "price_asc":
		orderBy = "p.price ASC"
	case "price_desc":
		orderBy = "p.price DESC"
	case "top_rated":
		orderBy = "p.rating DESC, p.review_count DESC"
	case "newest":
		orderBy = "p.created_at DESC"
	}

	whereSQL := strings.Join(where, " AND ")

	// Count
	var total int
	countSQL := "SELECT count(*) FROM products p JOIN sellers s ON s.id = p.seller_id WHERE " + whereSQL
	if err := h.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count error: " + err.Error()})
		return
	}

	// Page
	offset := (params.Page - 1) * params.PerPage
	args = append(args, params.PerPage, offset)

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query error: " + err.Error()})
		return
	}
	defer rows.Close()

	products := make([]model.Product, 0)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error: " + err.Error()})
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
func (h *ProductHandler) GetBySlug(c *gin.Context) {
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
		c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error: " + err.Error()})
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
		where = append(where, fmt.Sprintf("(p.name ILIKE $%d OR COALESCE(p.description,'') ILIKE $%d)", idx, idx))
		args = append(args, "%"+params.Q+"%")
		idx++
	}
	whereSQL := strings.Join(where, " AND ")

	// Brands with counts
	brandSQL := `
		SELECT p.brand, count(*) AS cnt
		FROM products p
		WHERE ` + whereSQL + ` AND p.brand IS NOT NULL
		GROUP BY p.brand ORDER BY cnt DESC, p.brand ASC
	`
	rows, err := h.db.Query(brandSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "facets error"})
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

// -- helpers -----------------------------------------------------------------

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

	q := productsQuery{
		Category: c.Query("category"),
		Q:        strings.TrimSpace(c.Query("q")),
		Brand:    c.Query("brand"),
		InStock:  c.Query("in_stock") == "true",
		Sort:     c.Query("sort"),
		Page:     page,
		PerPage:  perPage,
	}

	if v := c.Query("price_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			q.PriceMin = &f
		}
	}
	if v := c.Query("price_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			q.PriceMax = &f
		}
	}
	return q
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
