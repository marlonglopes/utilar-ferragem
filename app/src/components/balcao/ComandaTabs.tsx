import { Plus, X } from 'lucide-react'
import { cn } from '@/lib/cn'
import type { Comanda } from '@/store/balcaoStore'

export interface ComandaTabsProps {
  comandas: Comanda[]
  activeId: string
  onSelect: (id: string) => void
  onOpen: () => void
  onClose: (id: string) => void
}

/**
 * Abas de comandas — múltiplos pedidos em aberto ao mesmo tempo.
 *
 * Cenário real do balcão: o cliente A pede para "segurar" enquanto vai buscar o
 * cartão no carro e o cliente B já quer ser atendido. Sem isso o vendedor perde
 * o pedido ou anota no papel.
 */
export function ComandaTabs({
  comandas,
  activeId,
  onSelect,
  onOpen,
  onClose,
}: ComandaTabsProps) {
  return (
    <div className="flex items-center gap-1 overflow-x-auto border-b border-gray-200 bg-gray-50 px-2 py-1">
      {comandas.map((c) => {
        const active = c.id === activeId
        const count = c.items.reduce((sum, i) => sum + i.quantity, 0)
        return (
          <div
            key={c.id}
            className={cn(
              'flex shrink-0 items-center rounded-lg',
              active ? 'bg-white shadow-sm ring-1 ring-brand-orange' : 'hover:bg-gray-100'
            )}
          >
            <button
              type="button"
              onClick={() => onSelect(c.id)}
              aria-current={active ? 'true' : undefined}
              className={cn(
                'flex h-12 items-center gap-2 rounded-l-lg px-3 text-sm font-semibold',
                active ? 'text-brand-orange' : 'text-gray-600'
              )}
            >
              {c.label}
              {count > 0 && (
                <span className="rounded-full bg-gray-200 px-1.5 text-[11px] font-bold text-gray-700">
                  {count}
                </span>
              )}
            </button>
            {comandas.length > 1 && (
              <button
                type="button"
                onClick={() => onClose(c.id)}
                aria-label={`Fechar ${c.label}`}
                className="flex h-12 w-10 items-center justify-center rounded-r-lg text-gray-400 hover:text-red-600"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            )}
          </div>
        )
      })}

      <button
        type="button"
        onClick={onOpen}
        aria-label="Nova comanda"
        className="ml-1 flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border border-dashed border-gray-300 text-gray-500 hover:border-brand-orange hover:text-brand-orange"
      >
        <Plus className="h-5 w-5" aria-hidden="true" />
      </button>
    </div>
  )
}
