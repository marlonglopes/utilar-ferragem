import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft, Package, Check, Truck, MapPin, CreditCard,
  RotateCcw, X, ExternalLink, Loader2, AlertTriangle,
} from 'lucide-react'
import { useOrder, useOrders } from '@/hooks/useOrders'
import { useCartStore } from '@/store/cartStore'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import type { OrderStatus } from '@/lib/mockOrders'

// ─── Status timeline ─────────────────────────────────────────────────────────

const TIMELINE_STEPS: OrderStatus[] = ['pending_payment', 'paid', 'picking', 'shipped', 'delivered']

function Timeline({ status }: { status: OrderStatus }) {
  const { t } = useTranslation()
  if (status === 'cancelled') {
    return (
      <div className="flex items-center gap-2 text-sm text-red-600 bg-red-50 border border-red-100 rounded-xl px-4 py-3">
        <X className="h-4 w-4 flex-shrink-0" />
        {t('orderStatus.cancelled')}
      </div>
    )
  }

  const currentIdx = TIMELINE_STEPS.indexOf(status)

  return (
    <div className="flex items-start gap-0 overflow-x-auto pb-1">
      {TIMELINE_STEPS.map((step, idx) => {
        const done = idx < currentIdx
        const active = idx === currentIdx
        return (
          <div key={step} className="flex items-center gap-0 flex-1 min-w-[80px]">
            <div className="flex flex-col items-center gap-1 flex-1">
              <div className={cn(
                'w-7 h-7 rounded-full flex items-center justify-center flex-shrink-0',
                done && 'bg-green-500',
                active && 'bg-brand-orange',
                !done && !active && 'bg-gray-100'
              )}>
                {done
                  ? <Check className="h-3.5 w-3.5 text-white" />
                  : active
                    ? <div className="w-2.5 h-2.5 rounded-full bg-white" />
                    : <div className="w-2.5 h-2.5 rounded-full bg-gray-300" />
                }
              </div>
              <span className={cn(
                'text-[10px] text-center leading-tight px-1',
                active ? 'text-brand-orange font-semibold' : done ? 'text-green-600' : 'text-gray-400'
              )}>
                {t(`orders.timeline.${step}`)}
              </span>
            </div>
            {idx < TIMELINE_STEPS.length - 1 && (
              <div className={cn('h-0.5 flex-1 mx-1 -mt-5', idx < currentIdx ? 'bg-green-400' : 'bg-gray-200')} />
            )}
          </div>
        )
      })}
    </div>
  )
}

// ─── Status badge ─────────────────────────────────────────────────────────────

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

// ─── Cancel modal ─────────────────────────────────────────────────────────────

function CancelModal({
  onConfirm,
  onClose,
  loading,
}: {
  onConfirm: () => void
  onClose: () => void
  loading: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40">
      <div className="bg-white rounded-2xl p-6 max-w-sm w-full flex flex-col gap-4 shadow-xl">
        <div className="flex items-start gap-3">
          <AlertTriangle className="h-5 w-5 text-amber-500 flex-shrink-0 mt-0.5" />
          <div>
            <p className="font-semibold text-gray-900 text-sm">{t('orders.cancelOrder')}</p>
            <p className="text-sm text-gray-500 mt-1">{t('orders.cancelConfirm')}</p>
          </div>
        </div>
        <div className="flex gap-3">
          <button
            onClick={onClose}
            className="flex-1 h-10 rounded-xl border border-gray-300 text-sm font-semibold text-gray-600 hover:bg-gray-50 transition-colors"
          >
            {t('back')}
          </button>
          <button
            onClick={onConfirm}
            disabled={loading}
            className="flex-1 h-10 rounded-xl bg-red-500 text-white text-sm font-semibold hover:bg-red-600 transition-colors disabled:opacity-60 flex items-center justify-center gap-2"
          >
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('orders.cancelOrder')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── OrderDetailPage ──────────────────────────────────────────────────────────

export default function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { order, loading, error } = useOrder(id ?? '')
  const addItem = useCartStore((s) => s.addItem)
  const [cancelling, setCancelling] = useState(false)
  const [showCancelModal, setShowCancelModal] = useState(false)
  const [cancelDone, setCancelDone] = useState(false)

  const { cancelOrder } = useOrders()

  async function handleCancel() {
    setCancelling(true)
    const ok = await cancelOrder(id ?? '')
    setCancelling(false)
    setShowCancelModal(false)
    if (ok) setCancelDone(true)
  }

  function handleBuyAgain() {
    if (!order) return
    for (const item of order.items) {
      addItem({
        productId: item.productId,
        sellerId: item.sellerId,
        sellerName: item.sellerName,
        name: item.name,
        icon: item.icon,
        priceSnapshot: item.unitPrice,
        quantity: 1,
        stock: 99,
      })
    }
    navigate('/carrinho')
  }

  if (loading) {
    return (
      <div className="container py-16 flex justify-center">
        <Loader2 className="h-8 w-8 text-gray-300 animate-spin" />
      </div>
    )
  }

  if (error || !order) {
    return (
      <div className="container py-16 text-center">
        <p className="text-gray-400 text-sm">{error || 'Pedido não encontrado.'}</p>
        <Link to="/conta" className="text-brand-orange text-sm hover:underline mt-3 block">
          {t('back')}
        </Link>
      </div>
    )
  }

  const createdDate = new Date(order.createdAt).toLocaleDateString('pt-BR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
  })
  const updatedDate = new Date(order.updatedAt).toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit',
  })
  const itemCount = order.items.reduce((s, i) => s + i.quantity, 0)

  return (
    <div className="container py-6 max-w-3xl">
      {/* Back */}
      <Link
        to="/conta"
        className="flex items-center gap-1.5 text-sm text-gray-400 hover:text-gray-700 transition-colors mb-5"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('account.orders')}
      </Link>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6">
        <div>
          <h1 className="font-display font-bold text-xl text-gray-900">
            {t('orders.number', { number: order.number })}
          </h1>
          <p className="text-sm text-gray-400 mt-0.5">
            {t('orders.placedOn', { date: createdDate })} ·{' '}
            {t('orders.items', { count: itemCount })}
          </p>
        </div>
        <span className={cn('text-xs font-semibold px-3 py-1 rounded-full self-start sm:self-auto', statusColor(order.status))}>
          {cancelDone ? t('orderStatus.cancelled') : t(`orderStatus.${order.status}`)}
        </span>
      </div>

      <div className="flex flex-col gap-5">
        {/* Timeline */}
        <div className="bg-white border border-gray-200 rounded-xl p-5">
          <Timeline status={cancelDone ? 'cancelled' : order.status} />
          <p className="text-xs text-gray-400 mt-3">{t('orders.lastUpdate', { date: updatedDate })}</p>
        </div>

        {/* Items */}
        <div className="bg-white border border-gray-200 rounded-xl overflow-hidden">
          <div className="px-5 py-3 border-b border-gray-100 bg-gray-50">
            <p className="text-sm font-semibold text-gray-700">
              <Package className="inline h-4 w-4 mr-1.5 -mt-0.5" />
              {t('orders.items', { count: itemCount })}
            </p>
          </div>
          <div className="divide-y divide-gray-100">
            {order.items.map((item) => (
              <div key={item.productId} className="flex items-center gap-4 px-5 py-4">
                <div className="w-12 h-12 rounded-lg bg-gray-50 flex items-center justify-center text-2xl flex-shrink-0">
                  {item.icon}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-gray-900 truncate">{item.name}</p>
                  <p className="text-xs text-gray-400">
                    {t('orders.soldBy')} {item.sellerName}
                  </p>
                  <p className="text-xs text-gray-400 mt-0.5">
                    {t('orders.unitPrice')}: {formatCurrency(item.unitPrice)} · qty {item.quantity}
                  </p>
                </div>
                <p className="font-semibold text-gray-900 text-sm flex-shrink-0">
                  {formatCurrency(item.unitPrice * item.quantity)}
                </p>
              </div>
            ))}
          </div>
          {/* Totals */}
          <div className="px-5 py-4 border-t border-gray-100 flex flex-col gap-1.5 text-sm">
            <div className="flex justify-between text-gray-500">
              <span>Subtotal</span>
              <span>{formatCurrency(order.subtotal)}</span>
            </div>
            <div className="flex justify-between text-gray-500">
              <span>Frete</span>
              <span>{order.shippingCost === 0 ? <span className="text-green-600">Grátis</span> : formatCurrency(order.shippingCost)}</span>
            </div>
            <div className="flex justify-between font-bold text-gray-900 text-base pt-1 border-t border-gray-100">
              <span>Total</span>
              <span>{formatCurrency(order.total)}</span>
            </div>
          </div>
        </div>

        {/* Info cards — 2 col on desktop */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Shipping address */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-3 flex items-center gap-1.5">
              <MapPin className="h-3.5 w-3.5" />
              {t('orders.shippingAddress')}
            </p>
            <p className="text-sm text-gray-700 leading-relaxed">
              {order.address.street}, {order.address.number}
              {order.address.complement ? ` — ${order.address.complement}` : ''}
              <br />
              {order.address.neighborhood}, {order.address.city} – {order.address.state}
              <br />
              CEP {order.address.cep}
            </p>
          </div>

          {/* Payment */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-3 flex items-center gap-1.5">
              <CreditCard className="h-3.5 w-3.5" />
              {t('orders.paymentMethod')}
            </p>
            <p className="text-sm text-gray-700">{order.paymentInfo}</p>
          </div>

          {/* Tracking */}
          <div className="bg-white border border-gray-200 rounded-xl p-4">
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-3 flex items-center gap-1.5">
              <Truck className="h-3.5 w-3.5" />
              {t('orders.tracking')}
            </p>
            {order.trackingCode ? (
              <div className="flex items-center gap-2">
                <code className="text-sm font-mono bg-gray-50 px-2 py-1 rounded text-gray-700">
                  {order.trackingCode}
                </code>
                <a
                  href={`https://rastreamento.correios.com.br/app/index.php?objeto=${order.trackingCode}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-brand-orange hover:underline text-xs flex items-center gap-1"
                >
                  <ExternalLink className="h-3 w-3" />
                  Rastrear
                </a>
              </div>
            ) : (
              <p className="text-sm text-gray-400">{t('orders.noTracking')}</p>
            )}
          </div>

          {/* Support */}
          <div className="bg-white border border-gray-200 rounded-xl p-4 flex flex-col justify-between gap-3">
            <p className="text-sm text-gray-500">{t('orders.support')}</p>
            <a
              href={`mailto:suporte@utilarferragem.com.br?subject=Pedido%20${order.number}`}
              className="flex items-center gap-2 text-sm font-semibold text-brand-orange hover:underline"
            >
              {t('orders.supportLink')}
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </div>
        </div>

        {/* CTAs */}
        <div className="flex flex-col sm:flex-row gap-3">
          <button
            onClick={handleBuyAgain}
            className="flex-1 h-11 rounded-xl border border-gray-300 text-sm font-semibold text-gray-700 hover:bg-gray-50 transition-colors flex items-center justify-center gap-2"
          >
            <RotateCcw className="h-4 w-4" />
            {t('orders.buyAgain')}
          </button>

          {(order.status === 'pending_payment' && !cancelDone) && (
            <button
              onClick={() => setShowCancelModal(true)}
              className="flex-1 h-11 rounded-xl border border-red-200 text-sm font-semibold text-red-600 hover:bg-red-50 transition-colors flex items-center justify-center gap-2"
            >
              <X className="h-4 w-4" />
              {t('orders.cancelOrder')}
            </button>
          )}
        </div>
      </div>

      {showCancelModal && (
        <CancelModal
          onConfirm={handleCancel}
          onClose={() => setShowCancelModal(false)}
          loading={cancelling}
        />
      )}
    </div>
  )
}
