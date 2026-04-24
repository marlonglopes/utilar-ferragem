import { useQuery } from '@tanstack/react-query'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { getMockProducts } from '@/lib/mockProducts'
import type { Product } from '@/types/product'

interface RelatedResponse {
  data: Product[]
}

async function fetchRelated(slug: string, category: string | undefined, limit: number): Promise<Product[]> {
  if (isCatalogEnabled) {
    const res = await catalogGet<RelatedResponse>(`/api/v1/products/${slug}/related?limit=${limit}`)
    return res.data
  }
  // Fallback mock: category query + filter self out
  const mock = await getMockProducts({ category, per_page: limit + 1 })
  return mock.data.filter((p) => p.slug !== slug).slice(0, limit)
}

export function useRelatedProducts(slug: string, category: string | undefined, limit = 4) {
  return useQuery({
    queryKey: ['related', slug, category, limit],
    queryFn: () => fetchRelated(slug, category, limit),
    staleTime: 1000 * 60 * 5,
    enabled: Boolean(slug),
  })
}
