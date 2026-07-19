// Funções puras da Alice. Ficam fora dos arquivos de componente para não
// quebrar o fast refresh (um módulo de componente só deve exportar componentes)
// e para poderem ser testadas sem renderizar nada.
import type { MaterialItem } from '@/lib/alice'

export type Gravidade = 'alta' | 'media'

/**
 * Infere a gravidade pelo conteúdo do aviso.
 *
 * Os avisos vêm do package `safety` do assistant-service, com texto fixo. Os que
 * falam de estrutura, gás ou risco de vida são os que não podem em hipótese
 * alguma passar como recado de rodapé.
 */
export function inferirGravidade(aviso: string): Gravidade {
  const t = aviso.toLowerCase()
  return /(estrutural|engenheiro|arquiteto|g[áa]s|vidas|desabar|comprometer a constru)/.test(t)
    ? 'alta'
    : 'media'
}

/** Subtotal da linha, ou null quando não há produto casado (logo, não há preço). */
export function subtotalItem(item: MaterialItem): number | null {
  if (!item.produto) return null
  return item.embalagens * item.produto.preco
}

/** Total da lista. Só soma o que tem preço real — nunca estima o que falta. */
export function totalLista(itens: MaterialItem[]): number {
  return itens.reduce((acc, i) => acc + (subtotalItem(i) ?? 0), 0)
}

/** Itens que têm produto casado no catálogo — os únicos que podem ir ao carrinho. */
export function itensComProduto(itens: MaterialItem[]): MaterialItem[] {
  return itens.filter((i) => i.produto !== undefined && i.embalagens > 0)
}
