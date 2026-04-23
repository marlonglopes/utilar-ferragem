import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { useOrders, useOrder } from '@/hooks/useOrders'
import { useAuthStore } from '@/store/authStore'
import { MOCK_ORDERS } from '@/lib/mockOrders'

beforeEach(() => {
  useAuthStore.setState({
    user: { id: 'u1', email: 'test@test.com', name: 'Test', role: 'customer', token: 'tok' },
  })
})

describe('useOrders — mock mode', () => {
  it('returns all mock orders after loading', async () => {
    const { result } = renderHook(() => useOrders())
    expect(result.current.loading).toBe(true)

    await act(async () => {
      await new Promise((r) => setTimeout(r, 400))
    })

    expect(result.current.loading).toBe(false)
    expect(result.current.orders).toHaveLength(MOCK_ORDERS.length)
    expect(result.current.error).toBe('')
  }, 5000)

  it('returns orders with correct fields', async () => {
    const { result } = renderHook(() => useOrders())
    await act(async () => { await new Promise((r) => setTimeout(r, 400)) })

    const first = result.current.orders[0]
    expect(first).toHaveProperty('id')
    expect(first).toHaveProperty('status')
    expect(first).toHaveProperty('items')
    expect(first.items.length).toBeGreaterThan(0)
  }, 5000)

  it('cancelOrder transitions status to cancelled', async () => {
    const { result } = renderHook(() => useOrders())
    await act(async () => { await new Promise((r) => setTimeout(r, 400)) })

    const pending = result.current.orders.find((o) => o.status === 'pending_payment')
    expect(pending).toBeDefined()

    let ok = false
    await act(async () => {
      ok = await result.current.cancelOrder(pending!.id)
    })

    expect(ok).toBe(true)
    const updated = result.current.orders.find((o) => o.id === pending!.id)
    expect(updated?.status).toBe('cancelled')
  }, 5000)

  it('cancelOrder on non-existent id still returns true in mock mode', async () => {
    const { result } = renderHook(() => useOrders())
    await act(async () => { await new Promise((r) => setTimeout(r, 400)) })

    let ok = false
    await act(async () => {
      ok = await result.current.cancelOrder('non-existent')
    })
    expect(ok).toBe(true)
  }, 5000)

  it('provides a refresh function that re-fetches', async () => {
    const { result } = renderHook(() => useOrders())
    await act(async () => { await new Promise((r) => setTimeout(r, 400)) })

    expect(typeof result.current.refresh).toBe('function')
    await act(async () => {
      result.current.refresh()
      await new Promise((r) => setTimeout(r, 400))
    })
    expect(result.current.orders).toHaveLength(MOCK_ORDERS.length)
  }, 10000)
})

describe('useOrder — mock mode', () => {
  it('returns a single order by id', async () => {
    const target = MOCK_ORDERS[0]
    const { result } = renderHook(() => useOrder(target.id))

    await act(async () => { await new Promise((r) => setTimeout(r, 300)) })

    expect(result.current.order?.id).toBe(target.id)
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe('')
  }, 5000)

  it('returns null for unknown id', async () => {
    const { result } = renderHook(() => useOrder('does-not-exist'))
    await act(async () => { await new Promise((r) => setTimeout(r, 300)) })

    expect(result.current.order).toBeNull()
    expect(result.current.loading).toBe(false)
  }, 5000)
})

describe('mockOrders data integrity', () => {
  it('all mock orders have required fields', () => {
    for (const order of MOCK_ORDERS) {
      expect(order.id).toBeTruthy()
      expect(order.number).toBeTruthy()
      expect(order.status).toBeTruthy()
      expect(order.items.length).toBeGreaterThan(0)
      expect(order.total).toBeGreaterThan(0)
    }
  })

  it('has at least one order in each status category', () => {
    const statuses = MOCK_ORDERS.map((o) => o.status)
    expect(statuses).toContain('pending_payment')
    expect(statuses).toContain('delivered')
    expect(statuses).toContain('shipped')
    expect(statuses).toContain('paid')
  })
})
