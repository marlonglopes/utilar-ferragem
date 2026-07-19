import { useState, useCallback } from 'react'
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { MOCK_PRODUCTS } from '@/lib/mockProducts'
import { resolveBuyAgain, type BuyAgainResult, type ProductLookup } from '@/lib/buyAgain'
import { useCartStore } from '@/store/cartStore'
import type { Product } from '@/types/product'
import type { OrderItem } from '@/lib/mockOrders'

/**
 * Busca o produto pelo ID (e não pelo slug).
 *
 * O item do pedido guarda `productId`; o slug pode ter mudado desde a compra
 * (renomear produto reescreve o slug no catálogo). Buscar por slug faria um
 * produto ativo aparecer como "fora de linha" só porque mudou de nome.
 * Endpoint: `GET /api/v1/products/by-id/:id` (catalog-service).
 */
const lookupProduct: ProductLookup = async (productId) => {
  if (!isCatalogEnabled) {
    // Mock: o catálogo local só tem os ids de MOCK_PRODUCTS. Id desconhecido
    // devolve null = "saiu de linha", que é o comportamento certo e ainda
    // exercita o caminho parcial em modo mock.
    await new Promise((r) => setTimeout(r, 60))
    return MOCK_PRODUCTS.find((p) => p.id === productId) ?? null
  }
  return catalogGet<Product>(`/api/v1/products/by-id/${encodeURIComponent(productId)}`)
}

interface UseBuyAgainReturn {
  run: (items: OrderItem[]) => Promise<BuyAgainResult>
  /** Resultado da última execução, pra UI mostrar o resumo. `null` = nada rodou ainda. */
  result: BuyAgainResult | null
  loading: boolean
  dismiss: () => void
}

export function useBuyAgain(): UseBuyAgainReturn {
  const addItem = useCartStore((s) => s.addItem)
  const [result, setResult] = useState<BuyAgainResult | null>(null)
  const [loading, setLoading] = useState(false)

  const run = useCallback(
    async (items: OrderItem[]) => {
      setLoading(true)
      setResult(null)
      try {
        const res = await resolveBuyAgain(items, lookupProduct)

        for (const line of res.lines) {
          if (line.addedQty <= 0 || !line.product) continue
          const p = line.product
          addItem({
            productId: p.id,
            sellerId: p.sellerId ?? p.seller,
            sellerName: p.seller,
            name: p.name,
            icon: p.icon,
            // Preço ATUAL do catálogo, nunca o do pedido antigo. O preço do
            // pedido é histórico; recolocar ele no carrinho mostraria um valor
            // que o servidor vai recusar no checkout (o order-service resolve
            // preço do lado dele) e o cliente veria o total "subir sozinho".
            priceSnapshot: p.price,
            quantity: line.addedQty,
            stock: p.stock,
          })
        }

        setResult(res)
        return res
      } finally {
        setLoading(false)
      }
    },
    [addItem]
  )

  const dismiss = useCallback(() => setResult(null), [])

  return { run, result, loading, dismiss }
}
