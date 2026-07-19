import { Minus, Plus, Trash2, ShoppingCart, CreditCard } from 'lucide-react'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import { CustomerBlock } from './CustomerBlock'
import { NegotiationBlock } from './NegotiationBlock'
import type { BalcaoCustomer, BalcaoItem, BalcaoPricing, Comanda } from '@/store/balcaoStore'

interface LineProps {
  item: BalcaoItem
  onIncrement: (productId: string) => void
  onDecrement: (productId: string) => void
  onRemove: (productId: string) => void
}

function OrderLine({ item, onIncrement, onDecrement, onRemove }: LineProps) {
  const atStockCap = item.quantity >= item.stock
  return (
    <li className="flex gap-3 border-b border-gray-100 p-3 last:border-b-0">
      <div className="min-w-0 flex-1">
        <p className="line-clamp-2 text-sm font-semibold leading-snug text-gray-900">
          {item.name}
        </p>
        <p className="font-mono text-[11px] text-gray-500">
          {item.sku} · {formatCurrency(item.unitPrice)}/{item.unit}
        </p>

        <div className="mt-2 flex items-center gap-2">
          <button
            type="button"
            onClick={() => onDecrement(item.productId)}
            aria-label={`Diminuir quantidade de ${item.name}`}
            className="flex h-12 w-12 items-center justify-center rounded-lg border border-gray-300 bg-white text-gray-700 hover:bg-gray-100"
          >
            <Minus className="h-4 w-4" aria-hidden="true" />
          </button>

          <span
            aria-live="polite"
            className="min-w-[3rem] text-center font-display text-lg font-bold text-gray-900"
          >
            {item.quantity}
          </span>

          <button
            type="button"
            disabled={atStockCap}
            onClick={() => onIncrement(item.productId)}
            aria-label={`Aumentar quantidade de ${item.name}`}
            className="flex h-12 w-12 items-center justify-center rounded-lg border border-gray-300 bg-white text-gray-700 hover:bg-gray-100 disabled:opacity-40"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
          </button>

          <button
            type="button"
            onClick={() => onRemove(item.productId)}
            aria-label={`Remover ${item.name}`}
            className="ml-auto flex h-12 w-12 items-center justify-center rounded-lg text-gray-400 hover:bg-red-50 hover:text-red-600"
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </div>

      <span className="shrink-0 font-display text-sm font-bold text-gray-900">
        {formatCurrency(item.unitPrice * item.quantity)}
      </span>
    </li>
  )
}

export interface OrderPanelProps {
  comanda: Comanda
  pricing: BalcaoPricing
  onIncrement: (productId: string) => void
  onDecrement: (productId: string) => void
  onRemove: (productId: string) => void
  onDiscountChange: (pct: number) => void
  onCustomerChange: (customer: BalcaoCustomer | null) => void
  onCharge: () => void
}

/**
 * Painel direito fixo: cliente, itens, negociação, totais e o botão Cobrar.
 *
 * O botão Cobrar fica ancorado no rodapé (não rola junto): é a ação mais usada
 * do dia e não pode exigir que o vendedor role a lista para achá-la.
 */
export function OrderPanel({
  comanda,
  pricing,
  onIncrement,
  onDecrement,
  onRemove,
  onDiscountChange,
  onCustomerChange,
  onCharge,
}: OrderPanelProps) {
  const empty = comanda.items.length === 0
  const canCharge = !empty && !pricing.blocked && comanda.customer !== null

  return (
    <div className="flex h-full min-h-0 flex-col bg-white">
      <div className="flex items-center justify-between border-b border-gray-200 px-4 py-3">
        <h2 className="flex items-center gap-2 font-display text-base font-bold text-gray-900">
          <ShoppingCart className="h-5 w-5 text-brand-blue" aria-hidden="true" />
          Pedido do balcão
        </h2>
        <span className="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-bold text-gray-700">
          {pricing.itemCount} {pricing.itemCount === 1 ? 'item' : 'itens'}
        </span>
      </div>

      <CustomerBlock customer={comanda.customer} onChange={onCustomerChange} />

      <div className="min-h-0 flex-1 overflow-y-auto">
        {empty ? (
          <div className="flex h-full min-h-[140px] flex-col items-center justify-center px-6 text-center">
            <p className="text-sm font-semibold text-gray-700">Nenhum item</p>
            <p className="mt-1 text-xs text-gray-500">
              Toque nos produtos à esquerda ou leia o código de barras.
            </p>
          </div>
        ) : (
          <ul>
            {comanda.items.map((item) => (
              <OrderLine
                key={item.productId}
                item={item}
                onIncrement={onIncrement}
                onDecrement={onDecrement}
                onRemove={onRemove}
              />
            ))}
          </ul>
        )}
      </div>

      <NegotiationBlock
        pricing={pricing}
        onDiscountChange={onDiscountChange}
        disabled={empty}
      />

      {/* Totais + Cobrar */}
      <div className="border-t border-gray-200 bg-white p-4">
        <dl className="mb-3 space-y-1 text-sm">
          <div className="flex justify-between text-gray-600">
            <dt>Subtotal</dt>
            <dd>{formatCurrency(pricing.subtotal)}</dd>
          </div>
          {pricing.discountAmount > 0 && (
            <div className="flex justify-between font-semibold text-brand-orange">
              <dt>Desconto ({pricing.discountPct.toFixed(1).replace('.', ',')}%)</dt>
              <dd>− {formatCurrency(pricing.discountAmount)}</dd>
            </div>
          )}
          <div className="flex items-baseline justify-between border-t border-gray-200 pt-2">
            <dt className="font-display text-base font-bold text-gray-900">Total</dt>
            <dd className="font-display text-2xl font-bold text-brand-blue">
              {formatCurrency(pricing.total)}
            </dd>
          </div>
        </dl>

        {!empty && comanda.customer === null && (
          <p className="mb-2 text-xs font-semibold text-amber-700">
            Identifique o cliente para cobrar.
          </p>
        )}

        <button
          type="button"
          disabled={!canCharge}
          onClick={onCharge}
          className={cn(
            'flex h-16 w-full items-center justify-center gap-2 rounded-xl font-display text-lg font-bold text-white transition-colors',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange focus-visible:ring-offset-2',
            canCharge
              ? 'bg-brand-orange hover:bg-brand-orange-dark active:bg-brand-orange-dark'
              : 'cursor-not-allowed bg-gray-300'
          )}
        >
          <CreditCard className="h-6 w-6" aria-hidden="true" />
          Cobrar {formatCurrency(pricing.total)}
        </button>
        <p className="mt-1 text-center text-[11px] text-gray-400">F8</p>
      </div>
    </div>
  )
}
