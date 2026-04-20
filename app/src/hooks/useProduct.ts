import { useQuery } from '@tanstack/react-query'
import { getMockProduct } from '@/lib/mockProducts'
import type { Product } from '@/types/product'

const isApiEnabled = Boolean(import.meta.env.VITE_API_URL)

async function fetchProduct(slug: string): Promise<Product | null> {
  if (!isApiEnabled) return getMockProduct(slug)
  const res = await fetch(`${import.meta.env.VITE_API_URL}/api/v1/products/${slug}`)
  if (res.status === 404) return null
  if (!res.ok) throw new Error('Failed to fetch product')
  return res.json() as Promise<Product>
}

export function useProduct(slug: string) {
  return useQuery({
    queryKey: ['product', slug],
    queryFn: () => fetchProduct(slug),
    staleTime: 1000 * 60 * 5,
    enabled: Boolean(slug),
  })
}
