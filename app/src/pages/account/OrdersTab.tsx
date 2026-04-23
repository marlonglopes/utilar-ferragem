import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Package, ChevronRight, Loader2 } from 'lucide-react'
import { useOrders } from '@/hooks/useOrders'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import type { Order, OrderStatus } from '@/lib/mockOrders'

type Filter = 'all' | 'active' | 'done'

const ACTIVE_STATUSES: OrderStatus[] = ['pending_payment', 'paid', 'picking', 'shipped']
const DONE_STATUSES: OrderStatus[] = ['delivered', 'cancelled']

function statusColor(status: OrderStatus) {
  switch (status) {
    case 'delivered': return 'bg-green-100 text-green-700'
    case 'shipped': return 'bg-blue-100 text-blue-700'
    case 'picking':
    case 'paid': return 'bg-amber-100 text-amber-700'
    case 'pending_payment': return 'bg-orange-100 text-orange-700'
    case 'cancelled': return 'bg-gray-100 text-gray-500'
  }
}

function OrderCard({ order }: { order: Order }) {
  const { t } = useTranslation()
  const itemCount = order.items.reduce((s, i) => s + i.quantity, 0)
  const date = new Date(order.createdAt).toLocaleDateString('pt-BR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
  })

  return (
    <Link
      to={`/conta/pedidos/${order.id}`}
      className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 bg-white border border-gray-200 rounded-xl hover:border-gray-300 hover:shadow-sm transition-all"
    >
      <div className="flex items-start gap-3">
        <div className="w-10 h-10 rounded-lg bg-gray-50 flex items-center justify-center text-xl flex-shrink-0">
          {order.items[0]?.icon ?? '📦'}
        </div>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-gray-900">
            {t('orders.number', { number: order.number })}
          </p>
          <p className="text-xs text-gray-400 mt-0.5">
            {t('orders.placedOn', { date })} ·{' '}
            {t('orders.items', { count: itemCount })}
          </p>
          <p className="text-xs text-gray-500 mt-0.5 truncate">
            {order.items.map((i) => i.name).join(', ')}
          </p>
        </div>
      </div>
      <div className="flex sm:flex-col items-center sm:items-end gap-3 sm:gap-1 flex-shrink-0">
        <span className={cn('text-xs font-semibold px-2 py-0.5 rounded-full', statusColor(order.status))}>
          {t(`orderStatus.${order.status}`)}
        </span>
        <span className="text-sm font-bold text-gray-900">{formatCurrency(order.total)}</span>
        <ChevronRight className="h-4 w-4 text-gray-300 hidden sm:block" />
      </div>
    </Link>
  )
}

export default function OrdersTab() {
  const { t } = useTranslation()
  const { orders, loading, error } = useOrders()
  const [filter, setFilter] = useState<Filter>('all')

  const filtered = orders.filter((o) => {
    if (filter === 'active') return ACTIVE_STATUSES.includes(o.status)
    if (filter === 'done') return DONE_STATUSES.includes(o.status)
    return true
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-8 w-8 text-gray-300 animate-spin" />
      </div>
    )
  }

  if (error) {
    return (
      <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-xl px-4 py-3">{error}</p>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Filter tabs */}
      <div className="flex gap-1 border-b border-gray-100 pb-1">
        {(['all', 'active', 'done'] as Filter[]).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={cn(
              'px-3 py-1.5 text-xs font-semibold rounded-lg transition-colors',
              filter === f ? 'bg-brand-orange text-white' : 'text-gray-500 hover:text-gray-800 hover:bg-gray-50'
            )}
          >
            {t(`orders.filter.${f}`)}
          </button>
        ))}
      </div>

      {filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-16 text-center">
          <Package className="h-12 w-12 text-gray-200" />
          <div>
            <p className="text-sm font-medium text-gray-500">{t('orders.empty')}</p>
            <p className="text-xs text-gray-400 mt-1">{t('orders.emptyHint')}</p>
          </div>
          <Link
            to="/"
            className="text-sm text-brand-orange hover:underline font-medium"
          >
            {t('cartPage.exploreCatalog')}
          </Link>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {filtered.map((order) => (
            <OrderCard key={order.id} order={order} />
          ))}
        </div>
      )}
    </div>
  )
}
