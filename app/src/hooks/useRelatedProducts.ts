import { useQuery } from '@tanstack/react-query'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { getMockProducts } from '@/lib/mockProducts'
import type { Product } from '@/types/product'

/**
 * De onde a lista de relacionados veio.
 *
 * Espelha `model.RelatedMeta.Strategy` do catalog-service:
 * - `copurchase`        — co-compra real, sustentada por N pedidos distintos.
 * - `complement`        — regra de complemento curada (cabo → disjuntor).
 * - `mixed`             — mistura de co-compra e complemento.
 * - `category_fallback` — preenchimento por categoria. NÃO é recomendação:
 *   é `mesma categoria ORDER BY rating`, e todo produto de uma categoria ampla
 *   devolve os mesmos itens.
 *
 * O front NUNCA infere co-compra: só aceita se o backend declarar. É isso que
 * impede o bloco de virar promessa que o dado não cumpre.
 */
export type RelatedStrategy = 'copurchase' | 'complement' | 'mixed' | 'category_fallback'

/** Motivo por item, vindo do backend (`model.RelatedReason`). */
export interface RelatedReason {
  kind: string
  /** Texto curto já em pt-BR, pronto para exibir. */
  label: string
  /** Pedidos distintos que sustentam a co-compra — a evidência do número. */
  orders?: number
  note?: string | null
}

export interface RelatedProduct extends Product {
  reason?: RelatedReason
}

interface RelatedMeta {
  strategy?: RelatedStrategy
  /** `true` se QUALQUER item veio do preenchimento por categoria. */
  fallback?: boolean
  minCopurchaseOrders?: number
}

interface RelatedResponse {
  data: RelatedProduct[]
  meta?: RelatedMeta
}

export interface RelatedResult {
  products: RelatedProduct[]
  strategy: RelatedStrategy
  /** `true` ⇒ a UI não pode chamar isto de recomendação. */
  fallback: boolean
}

const KNOWN: RelatedStrategy[] = ['copurchase', 'complement', 'mixed', 'category_fallback']

/**
 * Normaliza a estratégia com viés conservador.
 *
 * Valor desconhecido, ausente ou resposta de uma versão antiga do backend cai
 * em `category_fallback` — o rótulo mais modesto. Errar para o lado humilde
 * mostra "Outros produtos de Elétrica" quando era recomendação de verdade;
 * errar para o outro lado promete co-compra que não existe.
 */
function normalizeStrategy(raw: unknown): RelatedStrategy {
  return KNOWN.includes(raw as RelatedStrategy) ? (raw as RelatedStrategy) : 'category_fallback'
}

async function fetchRelated(
  slug: string,
  category: string | undefined,
  limit: number
): Promise<RelatedResult> {
  if (isCatalogEnabled) {
    const res = await catalogGet<RelatedResponse>(`/api/v1/products/${slug}/related?limit=${limit}`)
    const strategy = normalizeStrategy(res.meta?.strategy)
    return {
      products: res.data,
      strategy,
      // `fallback` explícito do backend manda; sem ele, deduz da estratégia.
      fallback: res.meta?.fallback ?? strategy === 'category_fallback',
    }
  }
  // Mock: mesma categoria tirando o próprio produto — exatamente o que o
  // fallback do backend faz, então a estratégia declarada é a mesma.
  const mock = await getMockProducts({ category, per_page: limit + 1 })
  return {
    products: mock.data.filter((p) => p.slug !== slug).slice(0, limit),
    strategy: 'category_fallback',
    fallback: true,
  }
}

export function useRelatedProducts(slug: string, category: string | undefined, limit = 4) {
  return useQuery({
    queryKey: ['related', slug, category, limit],
    queryFn: () => fetchRelated(slug, category, limit),
    staleTime: 1000 * 60 * 5,
    enabled: Boolean(slug),
  })
}
