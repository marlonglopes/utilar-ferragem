import { renderHook, waitFor } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useProducts } from '@/hooks/useProducts'

// Wrapper com QueryClient isolado por teste (sem cache/retry).
function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
}

describe('useProducts — modo mock', () => {
  it('retorna produtos com meta de paginação', async () => {
    const { result } = renderHook(() => useProducts(), { wrapper: wrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(Array.isArray(result.current.data?.data)).toBe(true)
    expect(result.current.data!.data.length).toBeGreaterThan(0)
    expect(result.current.data!.meta.total).toBeGreaterThan(0)
  })

  it('filtra por categoria', async () => {
    const { result } = renderHook(() => useProducts({ category: 'ferramentas' }), {
      wrapper: wrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const items = result.current.data!.data
    expect(items.length).toBeGreaterThan(0)
    expect(items.every((p) => p.category === 'ferramentas')).toBe(true)
  })

  it('busca textual por nome (q) reduz o resultado', async () => {
    const { result } = renderHook(() => useProducts({ q: 'furadeira' }), {
      wrapper: wrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const items = result.current.data!.data
    expect(items.length).toBeGreaterThan(0)
    // o filtro q casa em nome/vendedor/descrição — todo item bate em algum deles
    expect(
      items.every((p) =>
        /furadeira/i.test(`${p.name} ${p.seller} ${p.description ?? ''}`),
      ),
    ).toBe(true)
    // e pelo menos um bate pelo nome
    expect(items.some((p) => /furadeira/i.test(p.name))).toBe(true)
  })
})
