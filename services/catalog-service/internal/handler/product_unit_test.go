package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Pure unit tests — sem DB. Validam parsing de query string.

func TestParseProductsQuery_defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/products", nil)

	q := parseProductsQuery(c)

	if q.Page != 1 {
		t.Errorf("Page default: got %d want 1", q.Page)
	}
	if q.PerPage != 24 {
		t.Errorf("PerPage default: got %d want 24", q.PerPage)
	}
	if q.Category != "" || q.Q != "" || q.Brand != "" {
		t.Errorf("string filters should be empty, got cat=%q q=%q brand=%q", q.Category, q.Q, q.Brand)
	}
	if q.PriceMin != nil || q.PriceMax != nil {
		t.Error("price bounds should be nil by default")
	}
	if q.InStock {
		t.Error("InStock should default to false")
	}
}

func TestParseProductsQuery_fullParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET",
		"/products?category=ferramentas&q=bosch&brand=Makita&price_min=50&price_max=2000&in_stock=true&sort=price_asc&page=3&per_page=12",
		nil)

	q := parseProductsQuery(c)

	if q.Category != "ferramentas" {
		t.Errorf("Category: got %q", q.Category)
	}
	if q.Q != "bosch" {
		t.Errorf("Q: got %q", q.Q)
	}
	if q.Brand != "Makita" {
		t.Errorf("Brand: got %q", q.Brand)
	}
	if q.PriceMin == nil || *q.PriceMin != 50 {
		t.Errorf("PriceMin: got %v want 50", q.PriceMin)
	}
	if q.PriceMax == nil || *q.PriceMax != 2000 {
		t.Errorf("PriceMax: got %v want 2000", q.PriceMax)
	}
	if !q.InStock {
		t.Error("InStock should be true")
	}
	if q.Sort != "price_asc" {
		t.Errorf("Sort: got %q", q.Sort)
	}
	if q.Page != 3 {
		t.Errorf("Page: got %d want 3", q.Page)
	}
	if q.PerPage != 12 {
		t.Errorf("PerPage: got %d want 12", q.PerPage)
	}
}

func TestParseProductsQuery_sanitizesBounds(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantPage   int
		wantPer    int
		wantInStock bool
	}{
		{"negative page → 1", "/products?page=-5", 1, 24, false},
		{"zero page → 1", "/products?page=0", 1, 24, false},
		{"per_page > 100 → 24", "/products?per_page=500", 1, 24, false},
		{"per_page = 0 → 24", "/products?per_page=0", 1, 24, false},
		{"in_stock=1 is not true", "/products?in_stock=1", 1, 24, false},
		{"in_stock=true is true", "/products?in_stock=true", 1, 24, true},
		{"price_min invalid → nil", "/products?price_min=abc", 1, 24, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", tt.url, nil)

			q := parseProductsQuery(c)

			if q.Page != tt.wantPage {
				t.Errorf("Page: got %d want %d", q.Page, tt.wantPage)
			}
			if q.PerPage != tt.wantPer {
				t.Errorf("PerPage: got %d want %d", q.PerPage, tt.wantPer)
			}
			if q.InStock != tt.wantInStock {
				t.Errorf("InStock: got %v want %v", q.InStock, tt.wantInStock)
			}
		})
	}
}

func TestParseProductsQuery_trimsWhitespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/products?q=%20%20bosch%20%20", nil)

	q := parseProductsQuery(c)
	if q.Q != "bosch" {
		t.Errorf("Q should be trimmed: got %q", q.Q)
	}
}
