import { useState, useEffect, useCallback } from 'react'
import { apiGet, apiPost, isApiEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import { MOCK_ORDERS, type Order, type OrderStatus } from '@/lib/mockOrders'

interface UseOrdersReturn {
  orders: Order[]
  loading: boolean
  error: string
  refresh: () => void
  cancelOrder: (id: string) => Promise<boolean>
}

export function useOrders(): UseOrdersReturn {
  const token = useAuthStore((s) => s.token())
  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const fetchOrders = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      if (!isApiEnabled) {
        await new Promise((r) => setTimeout(r, 300))
        setOrders(MOCK_ORDERS)
      } else {
        const data = await apiGet<Order[]>('/api/v1/orders?mine=true', token ?? undefined)
        setOrders(data)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Erro ao carregar pedidos')
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchOrders()
  }, [fetchOrders])

  const cancelOrder = useCallback(async (id: string): Promise<boolean> => {
    try {
      if (!isApiEnabled) {
        await new Promise((r) => setTimeout(r, 400))
        setOrders((prev) =>
          prev.map((o) =>
            o.id === id
              ? { ...o, status: 'cancelled' as OrderStatus, cancelledAt: new Date().toISOString(), updatedAt: new Date().toISOString() }
              : o
          )
        )
        return true
      }
      await apiPost(`/api/v1/orders/${id}/cancel`, {}, token ?? undefined)
      await fetchOrders()
      return true
    } catch {
      return false
    }
  }, [token, fetchOrders])

  return { orders, loading, error, refresh: fetchOrders, cancelOrder }
}

export function useOrder(id: string) {
  const token = useAuthStore((s) => s.token())
  const [order, setOrder] = useState<Order | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    async function fetch() {
      setLoading(true)
      setError('')
      try {
        if (!isApiEnabled) {
          await new Promise((r) => setTimeout(r, 200))
          const found = MOCK_ORDERS.find((o) => o.id === id) ?? null
          setOrder(found)
        } else {
          const data = await apiGet<Order>(`/api/v1/orders/${id}`, token ?? undefined)
          setOrder(data)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Erro ao carregar pedido')
      } finally {
        setLoading(false)
      }
    }
    fetch()
  }, [id, token])

  return { order, loading, error }
}
