package scrape

import (
	"context"

	"github.com/PuerkitoBio/goquery"
)

// DocFetcher é o que o Run e os adapters usam para baixar+parsear uma página.
// É uma interface (não o *Fetcher concreto) para os testes injetarem fixtures
// locais e rodarem SEM rede. O *Fetcher de produção satisfaz esta interface.
type DocFetcher interface {
	FetchDoc(ctx context.Context, url string) (*goquery.Document, error)
}

// Adapter é UM por site — o HTML muda entre fontes, então NÃO existe parser
// universal (é armadilha da spec). Cada adapter sabe descobrir as URLs de
// produto do seu site e extrair um produto de uma página já baixada.
type Adapter interface {
	// Name identifica a fonte (vai em ScrapedProduct.Fonte e no relatório).
	Name() string

	// BaseHost é o host canônico (ex: "www.exemplo.com.br") — usado pelo
	// gate de robots.txt e pelo rate limiter por domínio.
	BaseHost() string

	// Discover enumera as URLs de página de produto a partir do catálogo.
	// Prefira sitemap.xml quando existir; senão, paginação.
	Discover(ctx context.Context, f DocFetcher) ([]string, error)

	// Extract transforma UMA página de produto (já baixada e parseada) no
	// schema único. Deve devolver erro se a estrutura mudou (falhar visível,
	// nunca gravar dado meia-boca como se fosse completo).
	Extract(ctx context.Context, url string, doc *goquery.Document) (*ScrapedProduct, error)
}

// Registry guarda os adapters disponíveis por nome. Um catálogo de fontes
// autorizadas — nada roda sem um adapter registrado aqui.
type Registry struct{ adapters map[string]Adapter }

func NewRegistry() *Registry { return &Registry{adapters: map[string]Adapter{}} }

func (r *Registry) Register(a Adapter) { r.adapters[a.Name()] = a }

func (r *Registry) Get(name string) (Adapter, bool) {
	a, ok := r.adapters[name]
	return a, ok
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.adapters))
	for n := range r.adapters {
		out = append(out, n)
	}
	return out
}
