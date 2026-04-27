// Testa hardening do catalog-service:
// - escapeLikePattern (audit CT1-C1): escapa wildcards % _ \
// - parseProductsQuery rejeita price negativos (audit CT1-H3)
package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEscapeLikePattern(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"normal text", "normal text"},
		{"50%off", `50\%off`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
		{`%_\\`, `\%\_\\\\`},
		{"", ""},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := escapeLikePattern(c.in)
			if got != c.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseProductsQuery_RejectsNegativePriceRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name      string
		query     string
		wantPrice bool // true = aceito
	}{
		{"valid positive", "?price_min=10.5&price_max=100", true},
		{"zero accepted", "?price_min=0&price_max=0", true},
		{"min negative rejected", "?price_min=-1&price_max=100", false},
		{"max negative rejected", "?price_min=10&price_max=-1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest("GET", "/products"+tc.query, nil)
			c.Request = req

			q := parseProductsQuery(c)
			minOK := q.PriceMin != nil
			maxOK := q.PriceMax != nil

			if tc.wantPrice {
				if !minOK || !maxOK {
					t.Errorf("expected both prices set, got min=%v max=%v", q.PriceMin, q.PriceMax)
				}
			} else {
				// Pelo menos um dos dois deve ter sido rejeitado (nil)
				if minOK && maxOK {
					t.Errorf("expected at least one price to be rejected (nil), got min=%v max=%v", *q.PriceMin, *q.PriceMax)
				}
			}
		})
	}
}
