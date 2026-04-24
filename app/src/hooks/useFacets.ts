import { useQuery } from '@tanstack/react-query'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { getMockFacets } from '@/lib/mockProducts'

interface BrandFacet {
  value: string
  count: number
}

interface FacetsResponse {
  brands: BrandFacet[]
  price_min: number
  price_max: number
}

interface FacetsParams {
  category?: string
  q?: string
}

function toQueryString(params: FacetsParams): string {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== '')
  return new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString()
}

function normalize(res: FacetsResponse) {
  return { brands: res.brands, priceMin: res.price_min, priceMax: res.price_max }
}

export function useFacets(params: FacetsParams = {}) {
  return useQuery({
    queryKey: ['facets', params],
    queryFn: async () => {
      if (!isCatalogEnabled) return getMockFacets(params)
      const res = await catalogGet<FacetsResponse>(`/api/v1/products/facets?${toQueryString(params)}`)
      return normalize(res)
    },
    staleTime: 1000 * 60 * 5,
  })
}
