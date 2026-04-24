package model

import (
	"encoding/json"
	"time"
)

type Category struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Icon      string  `json:"icon"`
	ParentID  *string `json:"parent_id,omitempty"`
	SortOrder int     `json:"sort_order"`
}

type Seller struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"review_count"`
	Verified    bool    `json:"verified"`
}

type ProductImage struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

// Product JSON tags seguem o shape do frontend (camelCase).
// Payloads de listagem/facets também herdam esse formato.
type Product struct {
	ID             string          `json:"id"`
	Slug           string          `json:"slug"`
	Name           string          `json:"name"`
	Category       string          `json:"category"`
	Price          float64         `json:"price"`
	OriginalPrice  *float64        `json:"originalPrice,omitempty"`
	Currency       string          `json:"currency"`
	Icon           string          `json:"icon"`
	Brand          *string         `json:"brand,omitempty"`
	Seller         string          `json:"seller"`
	SellerID       string          `json:"sellerId"`
	SellerRating   float64         `json:"sellerRating"`
	SellerReviewCt int             `json:"sellerReviewCount"`
	Stock          int             `json:"stock"`
	Rating         float64         `json:"rating"`
	ReviewCount    int             `json:"reviewCount"`
	CashbackAmount *float64        `json:"cashbackAmount,omitempty"`
	Badge          *string         `json:"badge,omitempty"`
	BadgeLabel     *string         `json:"badgeLabel,omitempty"`
	Installments   *int            `json:"installments,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Specs          json.RawMessage `json:"specs"`
	Images         []ProductImage  `json:"images,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type ProductsResponse struct {
	Data []Product `json:"data"`
	Meta Meta      `json:"meta"`
}

type Meta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type Facets struct {
	Brands   []BrandFacet `json:"brands"`
	PriceMin float64      `json:"price_min"`
	PriceMax float64      `json:"price_max"`
}

type BrandFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}
