import { AlertTriangle } from 'lucide-react'
import { useCartStore } from '@/store/cartStore'
import type { CartAvailabilityIssue } from '@/lib/api'

/**
 * Aviso gracioso de itens do carrinho que travariam a compra — produto
 * arquivado/despublicado ("indisponível") ou sem saldo ("sem estoque"). Deixa
 * remover (ou ajustar a quantidade) em vez de estourar "product not found" no
 * pagamento. Usado no carrinho e no checkout.
 */
export function CartIssues({ issues }: { issues: CartAvailabilityIssue[] }) {
  const items = useCartStore((s) => s.items)
  const removeItem = useCartStore((s) => s.removeItem)
  const updateQuantity = useCartStore((s) => s.updateQuantity)

  if (issues.length === 0) return null

  return (
    <div
      role="alert"
      className="space-y-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800"
    >
      <p className="flex items-center gap-1.5 font-semibold">
        <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
        Ajuste estes itens para continuar
      </p>
      <ul className="space-y-2">
        {issues.map((iss) => {
          const item = items.find((i) => i.productId === iss.productId)
          const name = item?.name ?? 'Item'
          return (
            <li key={iss.productId} className="flex flex-wrap items-center gap-2">
              <span className="min-w-0 flex-1">
                <strong>{name}</strong>{' '}
                {iss.reason === 'indisponivel'
                  ? '— não está mais disponível.'
                  : `— só ${iss.available} em estoque.`}
              </span>
              {iss.reason === 'sem_estoque' && iss.available > 0 && (
                <button
                  type="button"
                  onClick={() => updateQuantity(iss.productId, iss.available)}
                  className="rounded-md border border-red-300 bg-white px-2 py-1 text-xs font-semibold text-red-700 hover:bg-red-100"
                >
                  Ajustar para {iss.available}
                </button>
              )}
              <button
                type="button"
                onClick={() => removeItem(iss.productId)}
                className="rounded-md bg-red-600 px-2 py-1 text-xs font-semibold text-white hover:bg-red-700"
              >
                Remover
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
