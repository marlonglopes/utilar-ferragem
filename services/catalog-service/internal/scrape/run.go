package scrape

import (
	"context"
	"errors"
	"time"
)

// Options controla um run.
type Options struct {
	MaxProdutos int              // 0 = sem limite (útil p/ testar com poucos itens)
	Now         func() time.Time // injetável p/ teste determinístico; nil = time.Now
}

// Run executa o fluxo COMPLETO de um adapter e devolve um Batch para revisão:
// descobre URLs → extrai → normaliza categoria → valida (imagem obrigatória) →
// deduplica → monta relatório. NUNCA escreve no catálogo (a publicação é decisão
// humana — o Batch entra depois como draft pelo import).
//
// Robustez (regras da spec):
//   - Bloqueio (429/403/robots) => PARA aquele domínio e reporta, não insiste.
//   - 404 numa página => item saiu do ar, pula.
//   - Mudança de HTML (Extract falha) => registra erro visível, não grava meia-boca.
func Run(ctx context.Context, a Adapter, f DocFetcher, opts Options) (*Batch, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	rep := Report{Fonte: a.Name(), IniciadoEm: now()}
	pausados := map[string]bool{}

	urls, err := a.Discover(ctx, f)
	if err != nil {
		if errors.Is(err, ErrBlocked) {
			// Descoberta bloqueada: reporta o domínio pausado, não falha o processo.
			pausados[a.BaseHost()] = true
			rep.DominiosPausados = sortedKeys(pausados)
			rep.FinalizadoEm = now()
			return &Batch{Report: rep}, nil
		}
		return nil, err
	}
	rep.URLsDescobertas = len(urls)

	var coletados []ScrapedProduct
coleta:
	for _, u := range urls {
		if opts.MaxProdutos > 0 && len(coletados) >= opts.MaxProdutos {
			break
		}
		if ctx.Err() != nil {
			break
		}
		doc, err := f.FetchDoc(ctx, u)
		if err != nil {
			switch {
			case errors.Is(err, ErrBlocked):
				pausados[HostOf(u)] = true
				break coleta // um host por adapter: bloqueou, para o run inteiro
			case errors.Is(err, ErrNotFound):
				continue // item fora do ar
			default:
				rep.Erros = append(rep.Erros, RunError{URLOrigem: u, Erro: err.Error()})
				continue
			}
		}
		p, err := a.Extract(ctx, u, doc)
		if err != nil {
			rep.Erros = append(rep.Erros, RunError{URLOrigem: u, Erro: err.Error()})
			continue
		}
		// URL é do orquestrador (o adapter não a conhece); o resto (fonte, moeda,
		// categoria normalizada, validação, sinalização) é a lógica compartilhada.
		p.URLOrigem = u
		if processarItem(p, a.Name(), now(), &rep) {
			coletados = append(coletados, *p)
		}
	}

	unique, dups := Dedup(coletados)
	rep.Coletados = len(unique)
	rep.Duplicatas = dups
	rep.DominiosPausados = sortedKeys(pausados)
	rep.FinalizadoEm = now()
	return &Batch{Report: rep, Produtos: unique}, nil
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
