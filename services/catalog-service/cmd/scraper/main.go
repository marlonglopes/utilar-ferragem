// Comando `scraper` — roda um adapter de fonte AUTORIZADA e grava um LOTE de
// produtos de ferragem para revisão humana (nunca escreve no catálogo direto).
//
// Uso:
//
//	scraper -list
//	scraper -adapter=NOME -out=lote.json [-max=50] [-delay-ms=2500]
//
// O lote resultante entra depois pelo fluxo de import (draft → revisão →
// publicação humana). Ver internal/scrape e docs/ingestao-de-produtos.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/utilar/catalog-service/internal/scrape"
)

func main() {
	adapterName := flag.String("adapter", "", "nome do adapter (fonte) a rodar")
	out := flag.String("out", "scrape-batch.json", "arquivo de saída (lote para revisão)")
	max := flag.Int("max", 0, "máximo de produtos (0 = sem limite)")
	list := flag.Bool("list", false, "lista os adapters registrados e sai")
	ua := flag.String("ua", scrape.DefaultUserAgent, "User-Agent identificável")
	delayMs := flag.Int("delay-ms", 2500, "intervalo mínimo entre requisições ao mesmo host (spec: 2000–3000)")
	flag.Parse()

	reg := scrape.NewRegistry()
	// Registre aqui os adapters de fontes AUTORIZADAS, quando existirem. Ex.:
	//   reg.Register(mercadolivre.New(appID, secret)) // API oficial (OAuth2 + backoff)
	//   reg.Register(fornecedorx.New())               // site com robots.txt liberado / permissão
	//   reg.Register(gs1cnp.New(token))               // provedor por EAN (GS1 CNP / Cosmos)

	if *list {
		names := reg.Names()
		if len(names) == 0 {
			fmt.Println("nenhum adapter registrado ainda — adicione em cmd/scraper/main.go")
			return
		}
		for _, n := range names {
			fmt.Println(n)
		}
		return
	}

	if *adapterName == "" {
		fmt.Fprintln(os.Stderr, "uso: scraper -adapter=NOME [-out=lote.json] [-max=N]   (ou -list)")
		os.Exit(2)
	}
	a, ok := reg.Get(*adapterName)
	if !ok {
		fmt.Fprintf(os.Stderr, "adapter %q não registrado. Rode com -list.\n", *adapterName)
		os.Exit(2)
	}

	f := scrape.NewFetcher(*ua, time.Duration(*delayMs)*time.Millisecond)
	batch, err := scrape.Run(context.Background(), a, f, scrape.Options{MaxProdutos: *max})
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "serializar lote: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "escrever %s: %v\n", *out, err)
		os.Exit(1)
	}

	r := batch.Report
	fmt.Printf("fonte=%s  coletados=%d  duplicatas=%d  sinalizados=%d  erros=%d  pausados=%v\n",
		r.Fonte, r.Coletados, r.Duplicatas, len(r.Sinalizados), len(r.Erros), r.DominiosPausados)
	fmt.Printf("lote em %s — REVISAR antes de importar (nunca publica sozinho)\n", *out)
}
