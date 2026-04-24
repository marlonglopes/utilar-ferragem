import { useQuery } from '@tanstack/react-query'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { getMockProduct } from '@/lib/mockProducts'
import type { Product } from '@/types/product'

async function fetchProduct(slug: string): Promise<Product | null> {
  if (!isCatalogEnabled) return getMockProduct(slug)
  try {
    return await catalogGet<Product>(`/api/v1/products/${slug}`)
  } catch (err) {
    if (err instanceof Error && err.message === 'not_found') return null
    throw err
  }
}

export function useProduct(slug: string) {
  return useQuery({
    queryKey: ['product', slug],
    queryFn: () => fetchProduct(slug),
    staleTime: 1000 * 60 * 5,
    enabled: Boolean(slug),
  })
}
