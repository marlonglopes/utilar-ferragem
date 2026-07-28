package scrape

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ── Fixtures offline (sem rede) ─────────────────────────────────────────────

// fixtureFetcher devolve HTML local por URL; satisfaz DocFetcher. Permite marcar
// URLs como bloqueadas (429/robots) ou 404 para exercitar esses caminhos.
type fixtureFetcher struct {
	pages    map[string]string
	blocked  map[string]bool
	notfound map[string]bool
}

func (f *fixtureFetcher) FetchDoc(_ context.Context, url string) (*goquery.Document, error) {
	if f.blocked[url] {
		return nil, ErrBlocked
	}
	if f.notfound[url] {
		return nil, ErrNotFound
	}
	html, ok := f.pages[url]
	if !ok {
		return nil, ErrNotFound
	}
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

// fixtureAdapter: um adapter de brinquedo que lê a estrutura das páginas-fixture.
type fixtureAdapter struct{ urls []string }

func (fixtureAdapter) Name() string     { return "fixture" }
func (fixtureAdapter) BaseHost() string { return "fix" }
func (a fixtureAdapter) Discover(_ context.Context, _ DocFetcher) ([]string, error) {
	return a.urls, nil
}
func (fixtureAdapter) Extract(_ context.Context, _ string, doc *goquery.Document) (*ScrapedProduct, error) {
	art := doc.Find("article").First()
	nome := strings.TrimSpace(doc.Find("h1.nome").First().Text())
	if nome == "" {
		return nil, errors.New("estrutura mudou: sem h1.nome")
	}
	p := &ScrapedProduct{
		Nome:           nome,
		CategoriaBruta: strings.TrimSpace(doc.Find("span.cat").First().Text()),
		Preco:          ParsePreco(doc.Find("span.preco").First().Text()),
	}
	if c := strings.TrimSpace(art.AttrOr("data-codigo", "")); c != "" {
		p.CodigoFabricante = &c
	}
	if src := strings.TrimSpace(doc.Find("img.foto").First().AttrOr("src", "")); src != "" {
		p.ImagemURLOriginal = &src
	}
	return p, nil
}

func page(cod, nome, cat, preco, img string) string {
	imgTag := ""
	if img != "" {
		imgTag = `<img class="foto" src="` + img + `">`
	}
	return `<html><body><article data-codigo="` + cod + `">` +
		`<h1 class="nome">` + nome + `</h1>` +
		`<span class="cat">` + cat + `</span>` +
		`<span class="preco">` + preco + `</span>` + imgTag +
		`</article></body></html>`
}

// ── Testes ──────────────────────────────────────────────────────────────────

// Pipeline completo: normaliza categoria, EXIGE imagem (produto sem foto sai do
// lote mas é sinalizado), deduplica por código, e conta tudo no relatório.
func TestRun_PipelineCompleto(t *testing.T) {
	pages := map[string]string{
		"https://fix/dobradica":     page("D1", `Dobradiça 3" Zincada`, "Dobradiças", "R$ 12,90", "https://cdn/d.jpg"),
		"https://fix/furadeira":     page("F1", "Furadeira de Impacto 750W", "Ferramentas Elétricas", "R$ 199,90", "https://cdn/f.jpg"),
		"https://fix/dobradica-dup": page("D1", "Dobradiça 3 pol Zincada", "Dobradiças", "R$ 13,00", "https://cdn/d2.jpg"),
		"https://fix/sem-foto":      page("P1", "Parafuso Chipboard 4x40", "Fixação", "R$ 0,20", ""), // SEM imagem
		"https://fix/misterioso":    page("M1", "Item Misterioso XYZ", "Diversos", "R$ 5,00", "https://cdn/m.jpg"),
	}
	adapter := fixtureAdapter{urls: []string{
		"https://fix/dobradica", "https://fix/furadeira", "https://fix/dobradica-dup",
		"https://fix/sem-foto", "https://fix/misterioso",
	}}
	fixedNow := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

	batch, err := Run(context.Background(), adapter, &fixtureFetcher{pages: pages},
		Options{Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatalf("Run erro: %v", err)
	}

	if batch.Report.URLsDescobertas != 5 {
		t.Errorf("URLsDescobertas = %d, quero 5", batch.Report.URLsDescobertas)
	}
	// Entram: dobradica, furadeira, misterioso. Fora: dobradica-dup (dup), sem-foto (sem imagem).
	if len(batch.Produtos) != 3 {
		t.Fatalf("Produtos = %d, quero 3 (%+v)", len(batch.Produtos), nomesDe(batch.Produtos))
	}
	if batch.Report.Duplicatas != 1 {
		t.Errorf("Duplicatas = %d, quero 1", batch.Report.Duplicatas)
	}
	// Sinalizados: sem-foto (sem imagem) + misterioso (categoria não reconhecida).
	if len(batch.Report.Sinalizados) != 2 {
		t.Errorf("Sinalizados = %d, quero 2 (%+v)", len(batch.Report.Sinalizados), batch.Report.Sinalizados)
	}

	byNome := map[string]ScrapedProduct{}
	for _, p := range batch.Produtos {
		byNome[p.Nome] = p
		if p.Moeda != "BRL" {
			t.Errorf("%s: moeda %q, quero BRL", p.Nome, p.Moeda)
		}
		if !p.ColetadoEm.Equal(fixedNow) {
			t.Errorf("%s: coletadoEm %v, quero %v", p.Nome, p.ColetadoEm, fixedNow)
		}
	}
	if c := byNome[`Dobradiça 3" Zincada`].CategoriaNormalizada; c != "fixacao" {
		t.Errorf("dobradiça normalizou para %q, quero fixacao", c)
	}
	if c := byNome["Furadeira de Impacto 750W"].CategoriaNormalizada; c != "ferramentas" {
		t.Errorf("furadeira normalizou para %q, quero ferramentas", c)
	}
	if c := byNome["Item Misterioso XYZ"].CategoriaNormalizada; c != CategoriaDesconhecida {
		t.Errorf("misterioso normalizou para %q, quero desconhecida (vazia)", c)
	}
}

// Bloqueio (429/robots) numa página PARA o domínio e reporta — não insiste, não
// falha o processo (regra da spec).
func TestRun_BloqueioParaDominioEReporta(t *testing.T) {
	adapter := fixtureAdapter{urls: []string{"https://fix/a", "https://fix/b", "https://fix/c"}}
	f := &fixtureFetcher{
		pages:   map[string]string{"https://fix/a": page("A1", "Alicate Universal 8", "Ferramentas", "R$ 30", "https://cdn/a.jpg")},
		blocked: map[string]bool{"https://fix/b": true}, // trava no 2º
	}
	batch, err := Run(context.Background(), adapter, f, Options{})
	if err != nil {
		t.Fatalf("Run não deveria falhar num bloqueio: %v", err)
	}
	if len(batch.Report.DominiosPausados) == 0 {
		t.Error("esperava domínio pausado reportado")
	}
	// Coletou o 'a' antes de bloquear; não tentou 'c'.
	if len(batch.Produtos) != 1 {
		t.Errorf("Produtos = %d, quero 1 (o coletado antes do bloqueio)", len(batch.Produtos))
	}
}

func nomesDe(ps []ScrapedProduct) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Nome
	}
	return out
}
