import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { usePayment } from '@/hooks/usePayment'

describe('usePayment — mock mode', () => {
  it('initial state is null result and empty error', () => {
    const { result } = renderHook(() => usePayment())
    expect(result.current.result).toBeNull()
    expect(result.current.error).toBe('')
  })

  it('createPayment(pix) returns pending mock result', async () => {
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment('order-1', 'pix', 100)
    })

    expect(result.current.result?.method).toBe('pix')
    expect(result.current.result?.status).toBe('pending')
    expect(result.current.result?.qrCodeBase64).toBeDefined()
    expect(result.current.result?.copyPaste).toBeDefined()
    expect(result.current.result?.expiresAt).toBeInstanceOf(Date)
  }, 5000)

  it('createPayment(boleto) returns pending mock with barCode', async () => {
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment('order-2', 'boleto', 100)
    })

    expect(result.current.result?.method).toBe('boleto')
    expect(result.current.result?.status).toBe('pending')
    expect(result.current.result?.barCode).toBeDefined()
    expect(result.current.result?.boletoExpiresAt).toBeInstanceOf(Date)
  }, 5000)

  it('createPayment(card) returns pending mock with initPoint', async () => {
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment('order-3', 'card', 100)
    })

    expect(result.current.result?.method).toBe('card')
    expect(result.current.result?.status).toBe('pending')
    expect(result.current.result?.initPoint).toBeDefined()
  }, 5000)

  it('pix polling auto-confirms after 6s (2 ticks × 3s)', async () => {
    vi.useFakeTimers()

    const { result } = renderHook(() => usePayment())

    // Start creation; advance past the 600ms simulated latency
    let createPromise: ReturnType<typeof result.current.createPayment>
    act(() => {
      createPromise = result.current.createPayment('order-4', 'pix', 100)
    })

    await act(async () => {
      await vi.advanceTimersByTimeAsync(700)
    })

    expect(result.current.result?.status).toBe('pending')

    // First poll tick (3s) — still pending (count = 1 < 2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(result.current.result?.status).toBe('pending')

    // Second poll tick (6s) — auto-confirms (count = 2 >= 2)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(result.current.result?.status).toBe('confirmed')

    await act(async () => { await createPromise })
    vi.useRealTimers()
  }, 10000)

  it('simulateConfirm sets status to confirmed immediately', async () => {
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment('order-5', 'boleto', 100)
    })

    act(() => {
      result.current.simulateConfirm()
    })

    expect(result.current.result?.status).toBe('confirmed')
  }, 5000)

  it('stopPolling stops the interval', async () => {
    vi.useFakeTimers()

    const { result } = renderHook(() => usePayment())

    let createPromise: ReturnType<typeof result.current.createPayment>
    act(() => {
      createPromise = result.current.createPayment('order-6', 'pix', 100)
    })

    await act(async () => {
      await vi.advanceTimersByTimeAsync(700)
    })

    act(() => {
      result.current.stopPolling()
    })

    // Advance well past auto-confirm point — status should stay pending
    await act(async () => {
      await vi.advanceTimersByTimeAsync(9000)
    })
    expect(result.current.result?.status).toBe('pending')

    await act(async () => { await createPromise })
    vi.useRealTimers()
  }, 10000)

  it('status transitions to creating while payment is in flight', async () => {
    vi.useFakeTimers()

    const { result } = renderHook(() => usePayment())

    let createPromise: ReturnType<typeof result.current.createPayment>
    act(() => {
      createPromise = result.current.createPayment('order-7', 'pix', 100)
    })

    // Status should be 'creating' before the 600ms resolves
    expect(result.current.result?.status).toBe('creating')

    await act(async () => {
      await vi.advanceTimersByTimeAsync(700)
      await createPromise
    })

    expect(result.current.result?.status).toBe('pending')
    vi.useRealTimers()
  }, 10000)
})
