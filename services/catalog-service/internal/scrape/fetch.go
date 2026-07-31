package scrape

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// UA padrão — identificável, NUNCA disfarçado de navegador para burlar bloqueio
// (regra da spec). Coloque um contato real de produção.
const DefaultUserAgent = "UtilarCatalogBot/1.0 (+https://utilar.com.br/bot; contato@utilar.com.br)"

// ErrBlocked sinaliza 429/403/robots — o run PARA aquele domínio e reporta,
// não insiste (regra da spec: bloqueio => parar imediatamente).
var ErrBlocked = errors.New("scrape: domínio bloqueado (429/403/robots)")

// ErrNotFound é 404 numa página de produto (item saiu do ar).
var ErrNotFound = errors.New("scrape: 404")

// Fetcher faz GET respeitando: robots.txt (por host), rate limit (por host),
// UA identificável e retry com backoff (máx. 3) só para falhas transientes.
type Fetcher struct {
	client   *http.Client
	ua       string
	minDelay time.Duration // intervalo mínimo entre requisições ao MESMO host

	mu      sync.Mutex
	lastHit map[string]time.Time
	robots  map[string]*Robots
}

// NewFetcher — minDelay padrão 2.5s (a spec pede 2–3s). O Crawl-delay do
// robots.txt, se maior, prevalece.
func NewFetcher(ua string, minDelay time.Duration) *Fetcher {
	if ua == "" {
		ua = DefaultUserAgent
	}
	if minDelay <= 0 {
		minDelay = 2500 * time.Millisecond
	}
	return &Fetcher{
		client:   &http.Client{Timeout: 30 * time.Second},
		ua:       ua,
		minDelay: minDelay,
		lastHit:  map[string]time.Time{},
		robots:   map[string]*Robots{},
	}
}

// robotsFor busca e cacheia o robots.txt do host. 404 => permissivo; erro de
// rede => Robots "unknown" (o Allowed devolve false por segurança).
func (f *Fetcher) robotsFor(ctx context.Context, host string) *Robots {
	f.mu.Lock()
	if r, ok := f.robots[host]; ok {
		f.mu.Unlock()
		return r
	}
	f.mu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/robots.txt", nil)
	req.Header.Set("User-Agent", f.ua)
	var r *Robots
	resp, err := f.client.Do(req)
	switch {
	case err != nil:
		r = RobotsUnknown()
	case resp.StatusCode == http.StatusNotFound:
		r = ParseRobots("", f.ua) // sem robots.txt => permitido
	case resp.StatusCode >= 400:
		r = RobotsUnknown()
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		r = ParseRobots(string(body), f.ua)
	}
	if resp != nil {
		resp.Body.Close()
	}
	f.mu.Lock()
	f.robots[host] = r
	f.mu.Unlock()
	return r
}

// throttle espera o intervalo mínimo (ou o Crawl-delay) para o host.
func (f *Fetcher) throttle(host string, delay time.Duration) {
	f.mu.Lock()
	last := f.lastHit[host]
	wait := time.Duration(0)
	if !last.IsZero() {
		if elapsed := time.Since(last); elapsed < delay {
			wait = delay - elapsed
		}
	}
	// Reserva o slot já (marca o próximo horário) antes de soltar o lock.
	f.lastHit[host] = time.Now().Add(wait)
	f.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

// Get baixa uma URL respeitando robots + rate limit + retry. Devolve o corpo
// (o caller fecha). Erros: ErrBlocked (429/403/robots), ErrNotFound (404).
func (f *Fetcher) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url inválida %q: %w", rawURL, err)
	}
	host := u.Host

	robots := f.robotsFor(ctx, host)
	if !robots.Allowed(u.Path) {
		return nil, fmt.Errorf("%w: robots.txt proíbe %s", ErrBlocked, u.Path)
	}
	delay := f.minDelay
	if robots.CrawlDelay > delay {
		delay = robots.CrawlDelay
	}

	var lastErr error
	backoff := 1 * time.Second
	for attempt := 0; attempt < 3; attempt++ {
		f.throttle(host, delay)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		req.Header.Set("User-Agent", f.ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode == http.StatusForbidden:
			resp.Body.Close()
			return nil, fmt.Errorf("%w: HTTP %d em %s", ErrBlocked, resp.StatusCode, rawURL)
		case resp.StatusCode == http.StatusNotFound:
			resp.Body.Close()
			return nil, ErrNotFound
		case resp.StatusCode >= 500:
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			time.Sleep(backoff)
			backoff *= 2
			continue
		case resp.StatusCode >= 400:
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d em %s", resp.StatusCode, rawURL)
		default:
			return resp, nil
		}
	}
	return nil, fmt.Errorf("falhou após 3 tentativas: %w", lastErr)
}

// FetchDoc baixa e parseia a página em um goquery.Document.
func (f *Fetcher) FetchDoc(ctx context.Context, rawURL string) (*goquery.Document, error) {
	resp, err := f.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("parse HTML %s: %w", rawURL, err)
	}
	return doc, nil
}

// HostOf extrai o host de uma URL (helper para adapters).
func HostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}
