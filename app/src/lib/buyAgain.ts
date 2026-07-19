import { isApiError } from '@/lib/api'
import type { Product } from '@/types/product'
import type { OrderItem } from '@/lib/mockOrders'

/**
 * Recompra ("Comprar novamente").
 *
 * Cimento, argamassa, cabo e parafuso são recompra pura — o mesmo cliente leva
 * o mesmo item todo mês. Mas o pedido é uma FOTO DO PASSADO: entre a compra e o
 * clique em "comprar novamente" o produto pode ter mudado de preço, zerado o
 * estoque ou saído de linha.
 *
 * A regra desta tela: repor o que dá e DIZER o que não deu. Adicionar 3 de 5
 * itens em silêncio é pior do que falhar inteiro — o cliente fecha o pedido
 * achando que levou os 5 e descobre na obra que faltam 2.
 */

export type BuyAgainOutcome =
  /** Reposto na quantidade original. */
  | 'added'
  /** Reposto, mas em quantidade menor: o estoque não cobre o pedido original. */
  | 'reduced'
  /** Existe no catálogo, mas está com estoque zero. */
  | 'out_of_stock'
  /** Não existe mais no catálogo (404) — saiu de linha. */
  | 'discontinued'
  /** Não deu pra saber: o catálogo falhou. Diferente de "não tem". */
  | 'lookup_failed'

export interface BuyAgainLine {
  productId: string
  name: string
  requestedQty: number
  addedQty: number
  outcome: BuyAgainOutcome
  /** Preço pago no pedido original. */
  oldPrice: number
  /** Preço atual no catálogo. `undefined` quando não deu pra consultar. */
  newPrice?: number
  /** Produto resolvido, quando existe — quem chama usa pra montar o item do carrinho. */
  product?: Product
}

export interface BuyAgainResult {
  lines: BuyAgainLine[]
  /** Linhas distintas do pedido original. */
  totalLines: number
  /** Linhas que entraram no carrinho (inclui as reduzidas). */
  addedLines: number
  outOfStock: BuyAgainLine[]
  discontinued: BuyAgainLine[]
  reduced: BuyAgainLine[]
  failed: BuyAgainLine[]
  /** Linhas repostas cujo preço mudou desde o pedido original. */
  priceChanged: BuyAgainLine[]
  /** `true` quando absolutamente nada entrou no carrinho. */
  nothingAdded: boolean
  /** `true` quando entrou algo, mas não tudo — o caso que exige explicação. */
  partial: boolean
}

/**
 * Resolve um produto pelo id. `null` significa "não existe mais" (404);
 * qualquer throw significa "não deu pra saber".
 */
export type ProductLookup = (productId: string) => Promise<Product | null>

/**
 * 404 vem em dois formatos neste app e os dois significam a mesma coisa:
 * - `ApiError` com `code: 'not_found'` (envelope do backend, caminho normal);
 * - `Error('not_found')`, sentinela que `catalogGet` lança ANTES de tentar ler
 *   o envelope (lib/api.ts) — 404 do catálogo nem sempre tem corpo JSON.
 *
 * Checamos `code` primeiro, como manda a casa. A sentinela é comparada por
 * igualdade exata, não por substring: não é texto de mensagem, é um valor
 * fixo do nosso próprio código.
 */
export function isNotFoundError(err: unknown): boolean {
  if (isApiError(err)) return err.is('not_found') || err.status === 404
  return err instanceof Error && err.message === 'not_found'
}

function classify(item: OrderItem, product: Product | null): BuyAgainLine {
  const base = {
    productId: item.productId,
    name: item.name,
    requestedQty: item.quantity,
    oldPrice: item.unitPrice,
  }

  if (!product) {
    return { ...base, addedQty: 0, outcome: 'discontinued' }
  }

  if (product.stock <= 0) {
    return { ...base, addedQty: 0, outcome: 'out_of_stock', newPrice: product.price, product }
  }

  // O estoque manda. Pedir 10 sacos quando restam 4 repõe 4 e avisa — melhor
  // do que estourar no checkout depois de o cliente já ter escolhido o frete.
  const addedQty = Math.min(item.quantity, product.stock)
  return {
    ...base,
    addedQty,
    outcome: addedQty < item.quantity ? 'reduced' : 'added',
    newPrice: product.price,
    product,
  }
}

/**
 * Consulta o catálogo item a item e classifica cada linha do pedido.
 *
 * NÃO mexe no carrinho — é função pura de decisão, pra poder ser testada sem
 * React e sem store. Quem adiciona é `useBuyAgain`.
 *
 * As consultas vão em paralelo (`allSettled`) porque um pedido de ferragem
 * tem tipicamente 5-15 linhas e em série isso é uma barra de progresso.
 * `allSettled` e não `all`: um produto fora do ar não pode derrubar a reposição
 * dos outros — a falha de um item vira `lookup_failed` daquela linha, só.
 */
export async function resolveBuyAgain(
  items: OrderItem[],
  lookup: ProductLookup
): Promise<BuyAgainResult> {
  const settled = await Promise.allSettled(items.map((i) => lookup(i.productId)))

  const lines: BuyAgainLine[] = items.map((item, idx) => {
    const outcome = settled[idx]
    if (outcome.status === 'fulfilled') return classify(item, outcome.value)

    if (isNotFoundError(outcome.reason)) return classify(item, null)

    // Erro de rede/servidor: NÃO é "acabou". Dizer "esgotado" aqui seria
    // mentir sobre estoque por causa de um timeout.
    return {
      productId: item.productId,
      name: item.name,
      requestedQty: item.quantity,
      oldPrice: item.unitPrice,
      addedQty: 0,
      outcome: 'lookup_failed' as const,
    }
  })

  const addedLines = lines.filter((l) => l.addedQty > 0)

  return {
    lines,
    totalLines: lines.length,
    addedLines: addedLines.length,
    outOfStock: lines.filter((l) => l.outcome === 'out_of_stock'),
    discontinued: lines.filter((l) => l.outcome === 'discontinued'),
    reduced: lines.filter((l) => l.outcome === 'reduced'),
    failed: lines.filter((l) => l.outcome === 'lookup_failed'),
    // Comparação em centavos: 38.5 vs 38.50 é o mesmo preço, e float
    // desigual por 1e-13 não pode virar aviso de "preço mudou".
    priceChanged: addedLines.filter(
      (l) => l.newPrice !== undefined && Math.round(l.newPrice * 100) !== Math.round(l.oldPrice * 100)
    ),
    nothingAdded: addedLines.length === 0,
    partial: addedLines.length > 0 && addedLines.length < lines.length,
  }
}
