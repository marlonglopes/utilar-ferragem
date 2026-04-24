import { useState, useEffect, useCallback } from 'react'
import { orderGet, orderPatch, isOrderEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import { MOCK_ORDERS, type Order, type OrderStatus } from '@/lib/mockOrders'

interface UseOrdersReturn {
  orders: Order[]
  loading: boolean
  error: string
  refresh: () => void
  cancelOrder: (id: string) => Promise<boolean>
}

interface OrdersListResponse {
  data: Order[]
  meta: { page: number; per_page: number; total: number; total_pages: number }
}

export function useOrders(): UseOrdersReturn {
  const user = useAuthStore((s) => s.user)
  const userId = user?.id ?? ''
  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const fetchOrders = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      if (!isOrderEnabled) {
        await new Promise((r) => setTimeout(r, 300))
        setOrders(MOCK_ORDERS)
      } else if (!userId) {
        setOrders([])
      } else {
        const res = await orderGet<OrdersListResponse>('/api/v1/orders?per_page=50', userId)
        setOrders(res.data)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Erro ao carregar pedidos')
    } finally {
      setLoading(false)
    }
  }, [userId])

  useEffect(() => {
    fetchOrders()
  }, [fetchOrders])

  const cancelOrder = useCallback(async (id: string): Promise<boolean> => {
    try {
      if (!isOrderEnabled) {
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
      if (!userId) return false
      await orderPatch(`/api/v1/orders/${id}/cancel`, userId)
      await fetchOrders()
      return true
    } catch {
      return false
    }
  }, [userId, fetchOrders])

  return { orders, loading, error, refresh: fetchOrders, cancelOrder }
}

export function useOrder(id: string) {
  const user = useAuthStore((s) => s.user)
  const userId = user?.id ?? ''
  const [order, setOrder] = useState<Order | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    async function fetch() {
      setLoading(true)
      setError('')
      try {
        if (!isOrderEnabled) {
          await new Promise((r) => setTimeout(r, 200))
          const found = MOCK_ORDERS.find((o) => o.id === id) ?? null
          setOrder(found)
          return
        }
        if (!userId) {
          setOrder(null)
          return
        }
        const data = await orderGet<Order>(`/api/v1/orders/${id}`, userId)
        setOrder(data)
      } catch (err) {
        if (err instanceof Error && err.message === 'not_found') {
          setOrder(null)
        } else {
          setError(err instanceof Error ? err.message : 'Erro ao carregar pedido')
        }
      } finally {
        setLoading(false)
      }
    }
    fetch()
  }, [id, userId])

  return { order, loading, error }
}
