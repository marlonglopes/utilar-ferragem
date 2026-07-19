import { describe, it, expect } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import {
  useShippingQuote,
  normalizeCep,
  formatCep,
  isValidCep,
} from '@/hooks/useShippingQuote'

/**
 * Regressão: o carrinho mostrava o texto fixo "Calculado no checkout". O
 * backend já servia POST /api/v1/shipping/quote (docs/shipping-api.md) e
 * ninguém chamava.
 *
 * Frete no carrinho é table stakes no Brasil: o cliente decide olhando o total
 * COM entrega. Descobrir o valor só no fim do funil é das maiores causas de
 * abandono.
 */
describe('CEP', () => {
  it('normaliza para 8 dígitos', () => {
    expect(normalizeCep('01310-100')).toBe('01310100')
    expect(normalizeCep('01310 100')).toBe('01310100')
    expect(normalizeCep('013101009999')).toBe('01310100') // trunca excesso
    expect(normalizeCep('abc')).toBe('')
  })

  it('formata progressivamente enquanto digita', () => {
    expect(formatCep('013')).toBe('013')
    expect(formatCep('01310')).toBe('01310')
    expect(formatCep('013101')).toBe('01310-1')
    expect(formatCep('01310100')).toBe('01310-100')
  })

  it('valida só com 8 dígitos', () => {
    expect(isValidCep('01310-100')).toBe(true)
    expect(isValidCep('0131010')).toBe(false)
    expect(isValidCep('')).toBe(false)
  })
})

describe('useShippingQuote — modo mock', () => {
  it('devolve opções ordenadas da mais barata para a mais cara', async () => {
    const { result } = renderHook(() => useShippingQuote())

    await act(async () => {
      await result.current.quote('01310-100', 150, 2)
    })

    await waitFor(() => expect(result.current.options).not.toBeNull())
    const opts = result.current.options!
    expect(opts.length).toBeGreaterThan(0)
    for (let i = 1; i < opts.length; i++) {
      expect(opts[i].cost).toBeGreaterThanOrEqual(opts[i - 1].cost)
    }
    expect(result.current.error).toBe('')
  })

  it('marca frete grátis acima do limiar em vez de custo zero', async () => {
    const { result } = renderHook(() => useShippingQuote())

    await act(async () => {
      await result.current.quote('01310-100', 350, 5)
    })

    await waitFor(() => expect(result.current.options).not.toBeNull())
    const padrao = result.current.options!.find((o) => o.serviceCode === 'standard')!
    // `free: true` existe para a UI dizer "Frete grátis", que o cliente lê como
    // benefício — e não "R$ 0,00", que parece erro de cálculo.
    expect(padrao.free).toBe(true)
    expect(padrao.cost).toBe(0)
  })

  it('rejeita CEP inválido sem chamar a API', async () => {
    const { result } = renderHook(() => useShippingQuote())

    await act(async () => {
      const r = await result.current.quote('123', 100, 1)
      expect(r).toBeNull()
    })

    expect(result.current.error).toBe('CEP inválido')
    expect(result.current.options).toBeNull()
  })

  it('descarta resposta de CEP antigo (race ao digitar)', async () => {
    const { result } = renderHook(() => useShippingQuote())

    // Duas cotações concorrentes: só a última pode valer. Sem o guarda de
    // request id, a resposta do CEP anterior chegaria depois e sobrescreveria a
    // do CEP atual — o cliente veria o frete de outra região.
    await act(async () => {
      const antiga = result.current.quote('20040-002', 100, 1) // interior/outros
      const nova = result.current.quote('01310-100', 100, 1) // capital
      const [rAntiga, rNova] = await Promise.all([antiga, nova])
      expect(rAntiga).toBeNull() // descartada
      expect(rNova).not.toBeNull()
    })

    await waitFor(() => expect(result.current.options).not.toBeNull())
    expect(result.current.options![0].zoneName).toContain('Capital')
  })

  it('reset limpa o estado ao trocar de CEP', async () => {
    const { result } = renderHook(() => useShippingQuote())

    await act(async () => {
      await result.current.quote('01310-100', 150, 2)
    })
    await waitFor(() => expect(result.current.options).not.toBeNull())

    act(() => result.current.reset())
    expect(result.current.options).toBeNull()
    expect(result.current.error).toBe('')
    expect(result.current.loading).toBe(false)
  })
})
