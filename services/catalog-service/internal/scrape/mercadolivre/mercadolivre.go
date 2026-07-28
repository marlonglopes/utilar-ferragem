// Package mercadolivre coleta produtos de ferragem pela **API oficial** do
// Mercado Livre (site MLB/Brasil). É a via LEGÍTIMA — OAuth2 + token + backoff —
// ao contrário de raspar o site anônimo, que dispara o bloqueio de 24h.
//
// ESQUELETO pronto para usar: crie o app em developers.mercadolivre.com.br,
// pegue App ID + Secret, e rode com os termos de busca de ferragem. O mapeamento
// para o schema único (scrape.ScrapedProduct) já está feito; o que pode precisar
// de ajuste fino está marcado com TODO.
//
// Uso (quando tiver credenciais):
//
//	ml := mercadolivre.New(clientID, clientSecret,
//	    []string{"dobradiça", "fechadura", "parafuso", "cadeado", "puxador"}, 30)
//	batch, err := ml.Run(ctx)   // batch → revisar → importar (nunca publica só)
//
// ⚠️ Termos do ML regem o uso da API; e a FOTO do anúncio é do vendedor/fabricante
// — re-hospedar publicamente é decisão humana (a mesma ressalva de sempre).
package mercadolivre

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/utilar/catalog-service/internal/scrape"
)

const (
	tokenURL = "https://api.mercadolibre.com/oauth/token"
	apiBase  = "https://api.mercadolibre.com"
	siteID   = "MLB" // Brasil
)

// Collector busca ferragem via API do ML e devolve um Batch para revisão.
type Collector struct {
	clientID     string
	clientSecret string
	termos       []string // termos de busca (ex.: "dobradiça", "fechadura")
	porTermo     int      // itens por termo (limit da busca)
	client       *http.Client
	pausa        time.Duration // backoff educado entre termos
}

// New monta o coletor. clientID/clientSecret vêm do app do ML (ambiente, nunca
// hardcoded). Sem termos, não há o que buscar.
func New(clientID, clientSecret string, termos []string, porTermo int) *Collector {
	if porTermo <= 0 {
		porTermo = 20
	}
	return &Collector{
		clientID:     clientID,
		clientSecret: clientSecret,
		termos:       termos,
		porTermo:     porTermo,
		client:       &http.Client{Timeout: 30 * time.Second},
		pausa:        500 * time.Millisecond,
	}
}

func (c *Collector) Name() string { return "mercadolivre" }

// token faz o OAuth2 client_credentials → access_token. É este passo que evita o
// bloqueio anônimo: você chama a API autenticado, dentro do seu limite.
func (c *Collector) token(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ml oauth: HTTP %d (confira client_id/secret)", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("ml oauth: access_token vazio")
	}
	return out.AccessToken, nil
}

// Run: autentica, coleta cada termo e monta o Batch (via scrape.Assemble, que
// normaliza + exige imagem + deduplica). Nunca escreve no catálogo.
func (c *Collector) Run(ctx context.Context) (*scrape.Batch, error) {
	inicio := time.Now()
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	var raw []scrape.ScrapedProduct
	var erros []scrape.RunError
	for _, termo := range c.termos {
		items, err := c.search(ctx, tok, termo)
		if err != nil {
			erros = append(erros, scrape.RunError{URLOrigem: "busca:" + termo, Erro: err.Error()})
			// 429/erro de rede: para de martelar. Termos já coletados ficam.
			break
		}
		raw = append(raw, items...)
		time.Sleep(c.pausa) // backoff educado entre termos
	}
	batch := scrape.Assemble(c.Name(), raw, inicio, time.Now())
	batch.Report.Erros = append(batch.Report.Erros, erros...)
	return batch, nil
}

// mlSearchResp espelha o subconjunto usado de GET /sites/MLB/search.
type mlSearchResp struct {
	Results []mlItem `json:"results"`
}

type mlItem struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Price      float64 `json:"price"`
	Thumbnail  string  `json:"thumbnail"`
	Permalink  string  `json:"permalink"`
	CategoryID string  `json:"category_id"`
	Attributes []struct {
		ID        string `json:"id"`
		ValueName string `json:"value_name"`
	} `json:"attributes"`
}

func (c *Collector) search(ctx context.Context, token, termo string) ([]scrape.ScrapedProduct, error) {
	u := fmt.Sprintf("%s/sites/%s/search?q=%s&limit=%d", apiBase, siteID, url.QueryEscape(termo), c.porTermo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("ml 429 em %q — respeite o backoff", termo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml search %q: HTTP %d", termo, resp.StatusCode)
	}
	var r mlSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return mapResults(r), nil
}

// mapResults transforma a resposta do ML no schema único. UMA imagem só (a
// thumbnail); a foto grande viria de GET /items/{id}.pictures[0] — TODO se quiser
// resolução melhor. CategoriaBruta = category_id do ML; a normalização casa pelo
// NOME (o dicionário de ferragem), não pela taxonomia do ML.
func mapResults(r mlSearchResp) []scrape.ScrapedProduct {
	out := make([]scrape.ScrapedProduct, 0, len(r.Results))
	for _, it := range r.Results {
		p := scrape.ScrapedProduct{
			Nome:           strings.TrimSpace(it.Title),
			CategoriaBruta: it.CategoryID,
			URLOrigem:      it.Permalink,
		}
		if it.Price > 0 {
			pr := it.Price
			p.Preco = &pr
		}
		if img := strings.TrimSpace(it.Thumbnail); img != "" {
			p.ImagemURLOriginal = &img
		}
		// Código do fabricante: GTIN/EAN ou SKU do vendedor, quando o anúncio traz.
		for _, at := range it.Attributes {
			if (at.ID == "GTIN" || at.ID == "SELLER_SKU") && strings.TrimSpace(at.ValueName) != "" {
				v := strings.TrimSpace(at.ValueName)
				p.CodigoFabricante = &v
				break
			}
		}
		out = append(out, p)
	}
	return out
}
