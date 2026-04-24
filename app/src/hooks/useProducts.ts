import { useQuery } from '@tanstack/react-query'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { getMockProducts } from '@/lib/mockProducts'
import type { ProductsParams, ProductsResponse } from '@/types/product'

function toQueryString(params: ProductsParams): string {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== '')
  return new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString()
}

export function useProducts(params: ProductsParams = {}) {
  return useQuery({
    queryKey: ['products', params],
    queryFn: () =>
      isCatalogEnabled
        ? catalogGet<ProductsResponse>(`/api/v1/products?${toQueryString(params)}`)
        : getMockProducts(params),
    staleTime: 1000 * 60 * 5,
  })
}
