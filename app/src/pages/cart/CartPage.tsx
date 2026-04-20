import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { Minus, Plus, Trash2, ShoppingBag } from 'lucide-react'
import { useCartStore, type CartItem } from '@/store/cartStore'
import { formatCurrency } from '@/lib/format'

function QtyControl({ item }: { item: CartItem }) {
  const { updateQuantity } = useCartStore()
  return (
    <div className="flex items-center gap-0">
      <button
        onClick={() => updateQuantity(item.productId, item.quantity - 1)}
        disabled={item.quantity <= 1}
        className="h-8 w-8 rounded-l border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label="Diminuir quantidade"
      >
        <Minus className="h-3.5 w-3.5" />
      </button>
      <div className="h-8 w-10 border-t border-b border-gray-300 flex items-center justify-center text-sm font-semibold">
        {item.quantity}
      </div>
      <button
        onClick={() => updateQuantity(item.productId, item.quantity + 1)}
        disabled={item.quantity >= item.stock}
        className="h-8 w-8 rounded-r border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label="Aumentar quantidade"
      >
        <Plus className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}

export default function CartPage() {
  const { t } = useTranslation()
  const { items, removeItem, clearCart } = useCartStore()

  const total = items.reduce((sum, i) => sum + i.priceSnapshot * i.quantity, 0)
  const totalCount = items.reduce((sum, i) => sum + i.quantity, 0)

  // Group items by seller
  const bySeller = items.reduce<Record<string, CartItem[]>>((acc, item) => {
    if (!acc[item.sellerId]) acc[item.sellerId] = []
    acc[item.sellerId].push(item)
    return acc
  }, {})

  if (items.length === 0) {
    return (
      <div className="container py-16 flex flex-col items-center justify-center gap-5 text-center">
        <ShoppingBag className="h-16 w-16 text-gray-300" />
        <div>
          <h1 className="font-display font-bold text-xl text-gray-900">{t('cartPage.empty')}</h1>
          <p className="text-sm text-gray-400 mt-2">{t('cartPage.emptyHint')}</p>
        </div>
        <Link
          to="/"
          className="px-6 py-2.5 rounded-xl bg-brand-orange text-white font-semibold text-sm hover:bg-brand-orange-dark transition-colors"
        >
          {t('cartPage.exploreCatalog')}
        </Link>
      </div>
    )
  }

  return (
    <div className="container py-6">
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="font-display font-bold text-2xl text-gray-900">
          {t('cartPage.title')}
          <span className="ml-2 text-base font-normal text-gray-400">
            {t('cartPage.items', { count: totalCount })}
          </span>
        </h1>
        <button
          onClick={clearCart}
          className="text-sm text-gray-400 hover:text-red-500 transition-colors"
        >
          Limpar carrinho
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        {/* Items list */}
        <div className="lg:col-span-2 flex flex-col gap-6">
          {Object.entries(bySeller).map(([sellerId, sellerItems]) => (
            <div key={sellerId} className="bg-white border border-gray-200 rounded-xl overflow-hidden">
              <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
                <p className="text-sm font-semibold text-gray-600">
                  {t('cartPage.soldBy')} <span className="text-gray-900">{sellerItems[0].sellerName}</span>
                </p>
              </div>
              <div className="divide-y divide-gray-100">
                {sellerItems.map((item) => (
                  <div key={item.productId} className="flex gap-4 p-4">
                    <div className="w-16 h-16 flex-shrink-0 bg-gray-50 rounded-lg flex items-center justify-center text-3xl select-none">
                      {item.icon}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-gray-900 leading-snug">{item.name}</p>
                      <p className="text-base font-bold text-gray-900 mt-1">{formatCurrency(item.priceSnapshot)}</p>
                      <div className="flex items-center gap-4 mt-2">
                        <QtyControl item={item} />
                        <button
                          onClick={() => removeItem(item.productId)}
                          className="flex items-center gap-1 text-sm text-gray-400 hover:text-red-500 transition-colors"
                          aria-label={t('cartPage.remove')}
                        >
                          <Trash2 className="h-4 w-4" />
                          <span className="hidden sm:inline">Remover</span>
                        </button>
                      </div>
                    </div>
                    <div className="flex-shrink-0 text-right">
                      <p className="font-bold text-gray-900">
                        {formatCurrency(item.priceSnapshot * item.quantity)}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* Order summary */}
        <div className="bg-white border border-gray-200 rounded-xl p-5 flex flex-col gap-4 lg:sticky lg:top-24">
          <h2 className="font-semibold text-gray-900 text-base">Resumo do pedido</h2>

          <div className="flex flex-col gap-2 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-gray-500">{t('cartPage.subtotal')}</span>
              <span className="font-medium text-gray-900">{formatCurrency(total)}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-500">{t('cartPage.shippingEstimate')}</span>
              <span className="text-gray-400 text-xs">Calculado no checkout</span>
            </div>
          </div>

          <div className="border-t border-gray-100 pt-3 flex items-center justify-between">
            <span className="font-semibold text-gray-900">{t('cartPage.total')}</span>
            <span className="font-bold text-xl text-gray-900">{formatCurrency(total)}</span>
          </div>

          <Link
            to="/checkout"
            className="w-full h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm flex items-center justify-center transition-colors"
          >
            {t('cartPage.goToCheckout')}
          </Link>
          <Link
            to="/"
            className="text-center text-sm text-brand-orange hover:underline"
          >
            {t('cartPage.exploreCatalog')}
          </Link>
        </div>
      </div>
    </div>
  )
}
