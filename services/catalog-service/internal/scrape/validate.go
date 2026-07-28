package scrape

import "strings"

// Validate decide o destino de um item coletado. Regras (spec + diretriz do dono):
//   - SEM imagem  => NÃO entra no lote (o dono pediu: só produto com foto).
//   - SEM nome    => NÃO entra (não dá pra revisar o que não tem nome).
//   - SEM categoria reconhecida => ENTRA, mas SINALIZADO (humano classifica).
//
// Nunca descarta em SILÊNCIO: mesmo o item recusado volta como Flag, para o
// relatório — quem revisa decide se valeu a pena buscar a imagem manualmente.
//
// Devolve (incluir, flag). flag != nil sempre que há algo a revisar.
func Validate(p *ScrapedProduct) (incluir bool, flag *Flag) {
	nome := strings.TrimSpace(p.Nome)
	temImg := p.ImagemURLOriginal != nil && strings.TrimSpace(*p.ImagemURLOriginal) != ""

	var motivos []string
	if nome == "" {
		motivos = append(motivos, "sem nome")
	}
	if !temImg {
		motivos = append(motivos, "sem imagem")
	}
	if len(motivos) > 0 {
		// Requisito duro ausente: fora do lote, mas reportado.
		return false, &Flag{URLOrigem: p.URLOrigem, Nome: nome, Motivos: motivos}
	}
	if p.CategoriaNormalizada == CategoriaDesconhecida {
		// Entra, mas pede classificação humana.
		return true, &Flag{URLOrigem: p.URLOrigem, Nome: nome, Motivos: []string{"categoria não reconhecida"}}
	}
	return true, nil
}
