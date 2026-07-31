package scrape

import "strings"

// Dedup remove duplicatas de um lote. Chave, em ordem de confiança:
//  1. codigo_fabricante (quando existe) — o identificador mais estável.
//  2. nome-normalizado + categoria — quando não há código.
//
// Mantém o PRIMEIRO item visto (a ordem de descoberta costuma ser a canônica).
func Dedup(items []ScrapedProduct) (unique []ScrapedProduct, removed int) {
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		k := dedupKey(it)
		if seen[k] {
			removed++
			continue
		}
		seen[k] = true
		unique = append(unique, it)
	}
	return unique, removed
}

func dedupKey(p ScrapedProduct) string {
	if p.CodigoFabricante != nil {
		if c := strings.TrimSpace(*p.CodigoFabricante); c != "" {
			return "cod:" + strings.ToLower(c)
		}
	}
	// Sem código: nome sem acento, espaços colapsados, + categoria normalizada.
	nome := unaccentLower(strings.Join(strings.Fields(p.Nome), " "))
	return "nome:" + nome + "|cat:" + p.CategoriaNormalizada
}
