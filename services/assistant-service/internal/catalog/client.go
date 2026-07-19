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

	// comCusto liga o pedido dos campos internos (custo, estoque por loja).
	// Desligado por padrão: o caminho público NUNCA pede custo, então mesmo um
	// bug adiante não tem o dado para vazar. Ligar exige token de serviço.
	comCusto     bool
	serviceToken string
}

// New cria o cliente PÚBLICO — sem custo, para o modo cliente.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: httpclient.New(8 * time.Second)}
}

// NewInterno cria o cliente do BALCÃO, que enriquece produtos com custo e
// margem vindos do catalog-service.
//
// COMO A INTEGRAÇÃO REALMENTE FICOU: o catalog-service expõe custo em UMA única
// rota, `GET /api/v1/admin/products/by-id/:id`, sob RequireRole(admin|service).
// Não existe busca com custo, e a rota é por ID, não por slug — decisão correta
// do lado deles (o payload público não tem por onde vazar custo), mas significa
// que aqui o custo é obtido por ENRIQUECIMENTO: busca-se normalmente na rota
// pública e depois pede-se o custo de alguns produtos, um a um.
//
// Consequência prática, e o motivo de Enriquecer ter um teto: cada produto
// custa uma chamada HTTP extra. Enriquecer a busca inteira multiplicaria a
// carga sobre o catálogo, então o balcão enriquece só os poucos produtos que
// realmente entram na conversa.
//
// Sem token de serviço o modo custo nem liga: um endpoint que exige credencial
// ou responde 401 (ruído) ou, pior, responde.
func NewInterno(baseURL, serviceToken string) *Client {
	c := &Client{baseURL: baseURL, http: httpclient.New(8 * time.Second)}
	if serviceToken != "" {
		c.comCusto = true
		c.serviceToken = serviceToken
	}
	return c
}

// ComCusto informa se este cliente está autorizado a trazer custo.
func (c *Client) ComCusto() bool { return c.comCusto }

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

	// ---- Campos INTERNOS: só existem no modo vendedor ----
	//
	// Cost é o custo de aquisição. É o dado mais sensível do negócio: vazar
	// para o site público entrega a margem da UtiLar a qualquer concorrente.
	// Ponteiro de propósito — nil significa "não carregado/não autorizado",
	// que é distinto de "custo zero". Só é preenchido quando o Client foi
	// construído com WithCusto (ver New/NewInterno).
	Cost *float64 `json:"cost,omitempty"`
	// Margem em % sobre o preço de venda, derivada de Cost. Mesmo tratamento.
	Margem *float64 `json:"margem,omitempty"`
	// Estoques por loja/CD. Também interno: o cliente vê disponibilidade,
	// o operador vê onde está.
	Estoques []EstoqueLoja `json:"estoques,omitempty"`
}

// EstoqueLoja é a posição de estoque numa unidade específica.
type EstoqueLoja struct {
	LojaID   string `json:"loja_id"`
	LojaNome string `json:"loja_nome"`
	Qtd      int    `json:"qtd"`
}

// CalcularMargem preenche Margem a partir de Cost e Price. Sem custo não há
// margem — e margem estimada seria pior que margem ausente.
func (p *Product) CalcularMargem() {
	if p.Cost == nil || p.Price <= 0 {
		p.Margem = nil
		return
	}
	m := (p.Price - *p.Cost) / p.Price * 100
	p.Margem = &m
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
	if c.serviceToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.serviceToken)
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

// Enriquecer preenche Cost e Margem consultando a rota de admin do catálogo,
// um produto por vez, até o teto `max`.
//
// Devolve também quantas chamadas HTTP gastou, para o chamador debitar do seu
// orçamento de requisição — enriquecimento é a operação mais cara da Alice e
// não pode ficar fora da contabilidade.
//
// Falha de um produto NÃO derruba os demais nem vira erro: o produto fica sem
// custo e a Alice diz que não tem o custo dele. Custo ausente é um fato que ela
// sabe comunicar; custo inventado, não.
func (c *Client) Enriquecer(ctx context.Context, prods []Product, max int) ([]Product, int) {
	if !c.comCusto || len(prods) == 0 || max <= 0 {
		return prods, 0
	}
	out := make([]Product, len(prods))
	copy(out, prods)

	gastas := 0
	for i := range out {
		if gastas >= max {
			break
		}
		if out[i].ID == "" || out[i].Cost != nil {
			continue
		}
		gastas++
		var ap struct {
			Cost      *float64 `json:"cost"`
			MarginPct *float64 `json:"marginPct"`
		}
		if err := c.get(ctx, "/api/v1/admin/products/by-id/"+url.PathEscape(out[i].ID), &ap); err != nil {
			continue // sem custo para este item; segue a vida
		}
		out[i].Cost = ap.Cost
		if ap.MarginPct != nil {
			// Prefere a margem calculada no servidor: o PDV e a Alice têm que
			// mostrar o mesmo número, e reimplementar a conta é como se criam
			// duas verdades.
			out[i].Margem = ap.MarginPct
		} else {
			out[i].CalcularMargem()
		}
	}
	return out, gastas
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
