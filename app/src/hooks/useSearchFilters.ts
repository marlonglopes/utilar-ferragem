import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { ProductsParams } from '@/types/product'

export interface SearchFilters {
  q: string
  category: string
  brand: string
  priceMin: string
  priceMax: string
  inStock: boolean
  sort: NonNullable<ProductsParams['sort']>
  page: number
}

export function useSearchFilters() {
  const [params, setParams] = useSearchParams()

  const filters: SearchFilters = {
    q: params.get('q') ?? '',
    category: params.get('categoria') ?? '',
    brand: params.get('marca') ?? '',
    priceMin: params.get('preco_min') ?? '',
    priceMax: params.get('preco_max') ?? '',
    inStock: params.get('em_estoque') === 'true',
    sort: (params.get('ordem') as ProductsParams['sort']) ?? 'relevance',
    page: Number(params.get('pagina') ?? '1'),
  }

  const set = useCallback((updates: Partial<SearchFilters>) => {
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      const map: Record<keyof SearchFilters, string> = {
        q: 'q', category: 'categoria', brand: 'marca',
        priceMin: 'preco_min', priceMax: 'preco_max',
        inStock: 'em_estoque', sort: 'ordem', page: 'pagina',
      }
      for (const [k, v] of Object.entries(updates) as [keyof SearchFilters, unknown][]) {
        const key = map[k]
        if (v === '' || v === false || v === 'relevance' || v === 1) {
          next.delete(key)
        } else {
          next.set(key, String(v))
        }
      }
      next.delete('pagina')
      if ('page' in updates) next.set('pagina', String(updates.page))
      return next
    }, { replace: true })
  }, [setParams])

  const toProductsParams = (): ProductsParams => ({
    q: filters.q || undefined,
    category: filters.category || undefined,
    brand: filters.brand || undefined,
    price_min: filters.priceMin ? Number(filters.priceMin) : undefined,
    price_max: filters.priceMax ? Number(filters.priceMax) : undefined,
    in_stock: filters.inStock || undefined,
    sort: filters.sort,
    page: filters.page,
    per_page: 12,
  })

  const activeCount = [
    filters.brand,
    filters.priceMin,
    filters.priceMax,
    filters.inStock,
    filters.category,
  ].filter(Boolean).length

  const clearAll = useCallback(() => {
    setParams((prev) => {
      const next = new URLSearchParams()
      if (prev.get('q')) next.set('q', prev.get('q')!)
      return next
    }, { replace: true })
  }, [setParams])

  return { filters, set, toProductsParams, activeCount, clearAll }
}
