import { cn } from '@/lib/cn'

/** Estoque baixo a partir daqui (inclusive) — chip âmbar. */
const LOW_STOCK_THRESHOLD = 10

export interface StockChipProps {
  stock: number
  className?: string
}

/**
 * Chip de estoque do card de produto. O vendedor decide a venda por esta cor,
 * então ela mostra sempre o NÚMERO junto — "baixo" sozinho não serve para
 * responder "dá para levar 8?".
 */
export function StockChip({ stock, className }: StockChipProps) {
  const out = stock <= 0
  const low = !out && stock <= LOW_STOCK_THRESHOLD

  const tone = out
    ? 'bg-red-100 text-red-700'
    : low
      ? 'bg-amber-100 text-amber-800'
      : 'bg-green-100 text-green-700'

  const label = out ? 'Esgotado' : `${stock} em estoque`

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold',
        tone,
        className
      )}
    >
      <span
        className={cn(
          'h-1.5 w-1.5 rounded-full',
          out ? 'bg-red-500' : low ? 'bg-amber-500' : 'bg-green-500'
        )}
        aria-hidden="true"
      />
      {label}
    </span>
  )
}
