import { useEffect, useState } from 'react'
import { checkCartAvailability, type CartAvailabilityIssue } from '@/lib/api'
import { useCartStore } from '@/store/cartStore'

/**
 * Valida o carrinho atual contra o catálogo: item arquivado/despublicado ou sem
 * saldo suficiente. Re-valida quando o carrinho muda. É o que evita o cliente
 * chegar no pagamento e tomar "product not found" cru.
 *
 * useEffect/useState (não react-query) DE PROPÓSITO: o fluxo de carrinho/checkout
 * não monta QueryClient nos testes, e isto é uma checagem simples e advisory.
 */
export function useCartAvailability(): { data: CartAvailabilityIssue[]; loading: boolean } {
  const items = useCartStore((s) => s.items)
  const key = items.map((i) => `${i.productId}:${i.quantity}`).join(',')
  const [data, setData] = useState<CartAvailabilityIssue[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (items.length === 0) {
      setData([])
      return
    }
    let cancelled = false
    setLoading(true)
    checkCartAvailability(items.map((i) => ({ productId: i.productId, quantity: i.quantity })))
      .then((r) => {
        if (!cancelled) setData(r)
      })
      .catch(() => {
        if (!cancelled) setData([]) // erro transitório não bloqueia
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // key resume a identidade do carrinho; `items` mudaria a cada render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  return { data, loading }
}
