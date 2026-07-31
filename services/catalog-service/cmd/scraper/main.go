// Comando `scraper` — coleta produtos de ferragem de uma fonte AUTORIZADA e grava
// um LOTE para revisão humana (nunca escreve no catálogo direto).
//
// Uso:
//
//	scraper -list
//	scraper -adapter=mercadolivre -out=lote.json [-max=30]     # API do ML (env: ML_CLIENT_ID/SECRET)
//	scraper -adapter=NOME -out=lote.json [-max=50]             # fonte de HTML registrada
//
// O lote entra depois pelo fluxo de import (draft → revisão → publicação humana).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/utilar/catalog-service/internal/scrape"
	"github.com/utilar/catalog-service/internal/scrape/mercadolivre"
)

// Termos de busca padrão de ferragem. Sobrescreve com ML_TERMOS="a,b,c".
var termosPadrao = []string{
	"dobradiça", "fechadura", "parafuso", "cadeado", "puxador", "bucha",
	"corrediça", "trilho", "fecho", "maçaneta", "ferrolho", "cantoneira",
}

func main() {
	adapterName := flag.String("adapter", "", "fonte a rodar (ex.: mercadolivre)")
	out := flag.String("out", "scrape-batch.json", "arquivo de saída (lote para revisão)")
	max := flag.Int("max", 0, "máx. produtos (HTML) / itens por termo (API); 0 = padrão")
	list := flag.Bool("list", false, "lista os adapters de HTML registrados e sai")
	ua := flag.String("ua", scrape.DefaultUserAgent, "User-Agent identificável")
	delayMs := flag.Int("delay-ms", 2500, "intervalo mínimo entre requisições ao mesmo host")
	flag.Parse()

	reg := scrape.NewRegistry()
	// Adapters de HTML (scrape.Adapter) entram aqui quando existirem, ex.:
	//   reg.Register(fornecedorx.New())  // site com robots.txt liberado / permissão

	if *list {
		names := reg.Names()
		if len(names) == 0 {
			fmt.Println("nenhum adapter de HTML registrado. Fonte de API pronta: mercadolivre")
			return
		}
		for _, n := range names {
			fmt.Println(n)
		}
		return
	}

	switch *adapterName {
	case "":
		fmt.Fprintln(os.Stderr, "uso: scraper -adapter=mercadolivre [-out=lote.json] [-max=N]   (ou -list)")
		os.Exit(2)

	case "mercadolivre":
		// Credenciais do app do devcenter do Mercado Livre — CONTA DO UTILAR.
		// Nunca hardcoded: vêm do ambiente (e o Secret vive no 1Password/.env gitignored).
		clientID := os.Getenv("ML_CLIENT_ID")
		secret := os.Getenv("ML_CLIENT_SECRET")
		if clientID == "" || secret == "" {
			fmt.Fprintln(os.Stderr, "defina ML_CLIENT_ID e ML_CLIENT_SECRET (app do Mercado Livre da conta do Utilar)")
			os.Exit(2)
		}
		termos := termosPadrao
		if t := strings.TrimSpace(os.Getenv("ML_TERMOS")); t != "" {
			termos = strings.Split(t, ",")
		}
		porTermo := *max
		if porTermo <= 0 {
			porTermo = 30
		}
		ml := mercadolivre.New(clientID, secret, termos, porTermo)
		batch, err := ml.Run(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "mercadolivre: %v\n", err)
			os.Exit(1)
		}
		gravar(batch, *out)

	default:
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
		gravar(batch, *out)
	}
}

func gravar(batch *scrape.Batch, out string) {
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "serializar lote: %v\n", err)
		os.Exit(1)
	}
	// filepath.Clean normaliza o caminho (colapsa `..`/`.`) — endurecimento
	// barato, mesmo o valor não sendo hostil.
	out = filepath.Clean(out)
	// #nosec G703 — `out` é a flag `-out` que o OPERADOR passa na linha de
	// comando (default "scrape-batch.json"), não entrada de rede nem de dado
	// coletado. Quem roda este binário de dev já tem autoridade de escrita no
	// processo; "path traversal" aqui é escrever onde o próprio operador mandou.
	// O lote é sempre um arquivo de REVISÃO — nunca publica nada sozinho.
	if err := os.WriteFile(out, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "escrever %s: %v\n", out, err)
		os.Exit(1)
	}
	r := batch.Report
	fmt.Printf("fonte=%s  coletados=%d  duplicatas=%d  sinalizados=%d  erros=%d\n",
		r.Fonte, r.Coletados, r.Duplicatas, len(r.Sinalizados), len(r.Erros))
	fmt.Printf("lote em %s — REVISAR antes de importar (nunca publica sozinho)\n", out)
}
