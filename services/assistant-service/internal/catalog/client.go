// Package catalog é o cliente HTTP que a Alice usa como FONTE DE FATOS.
// Toda afirmação factual (produto existe, preço, estoque) vem daqui via tool use
// — a Alice nunca inventa. Espelha o princípio da Gi (gifthy).
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/utilar/pkg/httpclient"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: httpclient.New(8 * time.Second)}
}

// Product é a visão enxuta que a Alice usa/expõe (cards no chat).
type Product struct {
	ID       string   `json:"id"`
	Slug     string   `json:"slug"`
	Name     string   `json:"name"`
	Price    float64  `json:"price"`
	Stock    int      `json:"stock"`
	Brand    *string  `json:"brand,omitempty"`
	Category string   `json:"category"`
	Icon     string   `json:"icon,omitempty"`
	Rating   float64  `json:"rating"`
	Images   []Image  `json:"images,omitempty"`
	Specs    rawSpecs `json:"specs,omitempty"`
	Desc     *string  `json:"description,omitempty"`
}

type Image struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

type rawSpecs = json.RawMessage

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("catalog GET %s → %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Search busca produtos por termo/categoria (máx `limit`).
func (c *Client) Search(ctx context.Context, query, category string, limit int) ([]Product, error) {
	if limit <= 0 || limit > 12 {
		limit = 6
	}
	q := url.Values{}
	if query != "" {
		q.Set("q", query)
	}
	if category != "" {
		q.Set("category", category)
	}
	q.Set("per_page", fmt.Sprint(limit))
	var resp struct {
		Data []Product `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/products?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetBySlug retorna um produto detalhado.
func (c *Client) GetBySlug(ctx context.Context, slug string) (*Product, error) {
	var p Product
	if err := c.get(ctx, "/api/v1/products/"+url.PathEscape(slug), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Categories lista as categorias (slugs).
func (c *Client) Categories(ctx context.Context) ([]string, error) {
	var resp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/categories", &resp); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(resp.Data))
	for _, cat := range resp.Data {
		out = append(out, cat.ID)
	}
	return out, nil
}
