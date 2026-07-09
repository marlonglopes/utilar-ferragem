import { renderHook, waitFor } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useFacets } from '@/hooks/useFacets'

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
}

describe('useFacets — modo mock', () => {
  it('retorna marcas e faixa de preço', async () => {
    const { result } = renderHook(() => useFacets({ category: 'ferramentas' }), {
      wrapper: wrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(Array.isArray(result.current.data?.brands)).toBe(true)
    expect(typeof result.current.data?.priceMin).toBe('number')
    expect(typeof result.current.data?.priceMax).toBe('number')
    expect(result.current.data!.priceMin).toBeLessThanOrEqual(result.current.data!.priceMax)
  })
})
