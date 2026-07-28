package scrape

import "time"

// processarItem normaliza + valida um item COLETADO (de HTML ou API) e registra
// as sinalizações no relatório. Devolve se o item deve entrar no lote. É o ponto
// único onde a regra "imagem obrigatória + categoria controlada" se aplica —
// reusado pelo Run (HTML) e pelos coletores de API (ML, EAN…), pra não divergir.
func processarItem(p *ScrapedProduct, fonte string, coletadoEm time.Time, rep *Report) bool {
	p.Fonte = fonte
	p.Moeda = "BRL"
	if p.ColetadoEm.IsZero() {
		p.ColetadoEm = coletadoEm
	}
	if p.CategoriaNormalizada == "" {
		p.CategoriaNormalizada = NormalizeCategoria(p.CategoriaBruta, p.Nome)
	}
	incluir, flag := Validate(p)
	if flag != nil {
		rep.Sinalizados = append(rep.Sinalizados, *flag)
	}
	return incluir
}

// Assemble monta o Batch final a partir de itens JÁ COLETADOS (de qualquer
// fonte). Normaliza, valida (imagem obrigatória), deduplica e fecha o relatório.
// É a via dos coletores de API — o Run (HTML) faz a mesma coisa item a item,
// via processarItem, porque também precisa lidar com fetch/robots/erros por URL.
func Assemble(fonte string, raw []ScrapedProduct, iniciado, finalizado time.Time) *Batch {
	rep := Report{Fonte: fonte, IniciadoEm: iniciado, FinalizadoEm: finalizado}
	var coletados []ScrapedProduct
	for i := range raw {
		if processarItem(&raw[i], fonte, finalizado, &rep) {
			coletados = append(coletados, raw[i])
		}
	}
	unique, dups := Dedup(coletados)
	rep.Coletados = len(unique)
	rep.Duplicatas = dups
	return &Batch{Report: rep, Produtos: unique}
}
