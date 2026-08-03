import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mocka a camada de API pra inspecionar o CORPO enviado ao POST /payments.
// (Os outros testes de usePayment rodam em mock mode, sem apiPost.)
const apiPost = vi.fn()
vi.mock('@/lib/api', () => ({
  isApiEnabled: true,
  apiPost: (...args: unknown[]) => apiPost(...args),
  apiGet: vi.fn().mockResolvedValue({ status: 'pending' }),
}))

import { usePayment } from '@/hooks/usePayment'
import { useAuthStore } from '@/store/authStore'

beforeEach(() => {
  apiPost.mockReset()
  apiPost.mockResolvedValue({
    id: 'pay-x',
    psp_id: '900',
    provider: 'appmax-v1',
    method: 'card',
    status: 'pending',
    psp_payload: { provider: 'appmax-v1', installments: 6 },
  })
  useAuthStore.setState({ user: { token: 'jwt-123' } as never })
})

describe('usePayment.createPayment — corpo enviado ao PSP', () => {
  it('cartão inclui card_token e installments no corpo (parcelamento ponta a ponta)', async () => {
    const { result } = renderHook(() => usePayment())
    await act(async () => {
      await result.current.createPayment('order-9', 'card', 500, {
        card_token: 'tok_browser_abc',
        installments: 6,
        payer_cpf: '12345678909',
      })
    })

    expect(apiPost).toHaveBeenCalledTimes(1)
    const [path, body, jwt] = apiPost.mock.calls[0] as [string, Record<string, unknown>, string]
    expect(path).toBe('/api/v1/payments')
    expect(body).toMatchObject({
      order_id: 'order-9',
      method: 'card',
      amount: 500,
      card_token: 'tok_browser_abc',
      installments: 6,
      payer_cpf: '12345678909',
    })
    expect(jwt).toBe('jwt-123')
  })

  it('sem extras de cartão, o corpo não carrega card_token/installments', async () => {
    apiPost.mockResolvedValue({
      id: 'pay-y',
      provider: 'appmax-v1',
      method: 'card',
      status: 'pending',
      psp_payload: { provider: 'appmax-v1', installments: 1 },
    })
    const { result } = renderHook(() => usePayment())
    await act(async () => {
      await result.current.createPayment('order-10', 'card', 100)
    })
    const [, body] = apiPost.mock.calls[0] as [string, Record<string, unknown>]
    expect(body).not.toHaveProperty('card_token')
    expect(body).not.toHaveProperty('installments')
  })
})
