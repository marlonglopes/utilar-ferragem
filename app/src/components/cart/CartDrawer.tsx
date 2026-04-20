import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Minus, Plus, Trash2, ShoppingBag } from 'lucide-react'
import { Drawer } from '@/components/ui'
import { useCartStore, type CartItem } from '@/store/cartStore'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'

interface CartDrawerProps {
  open: boolean
  onClose: () => void
}

function QtyControl({ item }: { item: CartItem }) {
  const { updateQuantity } = useCartStore()
  return (
    <div className="flex items-center gap-0">
      <button
        onClick={() => updateQuantity(item.productId, item.quantity - 1)}
        disabled={item.quantity <= 1}
        className="h-7 w-7 rounded-l border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label="Diminuir"
      >
        <Minus className="h-3 w-3" />
      </button>
      <div className="h-7 w-8 border-t border-b border-gray-300 flex items-center justify-center text-xs font-semibold">
        {item.quantity}
      </div>
      <button
        onClick={() => updateQuantity(item.productId, item.quantity + 1)}
        disabled={item.quantity >= item.stock}
        className="h-7 w-7 rounded-r border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed"
        aria-label="Aumentar"
      >
        <Plus className="h-3 w-3" />
      </button>
    </div>
  )
}

export function CartDrawer({ open, onClose }: CartDrawerProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { items, removeItem } = useCartStore()

  const total = items.reduce((sum, i) => sum + i.priceSnapshot * i.quantity, 0)
  const totalCount = items.reduce((sum, i) => sum + i.quantity, 0)

  // Group items by seller
  const bySeller = items.reduce<Record<string, CartItem[]>>((acc, item) => {
    if (!acc[item.sellerId]) acc[item.sellerId] = []
    acc[item.sellerId].push(item)
    return acc
  }, {})

  function goToCart() {
    onClose()
    navigate('/carrinho')
  }

  function goToCheckout() {
    onClose()
    navigate('/checkout')
  }

  const titleLabel = totalCount > 0
    ? t('cartPage.title') + ` · ` + t('cartPage.items', { count: totalCount, postProcess: 'interval' })
    : t('cartPage.title')

  return (
    <Drawer open={open} onClose={onClose} title={titleLabel} side="right">
      {items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <ShoppingBag className="h-12 w-12 text-gray-300" />
          <div>
            <p className="font-semibold text-gray-700">{t('cartPage.empty')}</p>
            <p className="text-sm text-gray-400 mt-1">{t('cartPage.emptyHint')}</p>
          </div>
          <button
            onClick={() => { onClose(); navigate('/') }}
            className="mt-2 px-4 py-2 rounded-lg bg-brand-orange text-white text-sm font-semibold hover:bg-brand-orange-dark transition-colors"
          >
            {t('cartPage.exploreCatalog')}
          </button>
        </div>
      ) : (
        <div className="flex flex-col gap-5">
          {Object.entries(bySeller).map(([sellerId, sellerItems]) => (
            <div key={sellerId}>
              <p className="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">
                {t('cartPage.soldBy')} {sellerItems[0].sellerName}
              </p>
              <div className="flex flex-col gap-3">
                {sellerItems.map((item) => (
                  <div key={item.productId} className="flex gap-3">
                    <div className="w-12 h-12 flex-shrink-0 bg-gray-50 rounded-lg flex items-center justify-center text-2xl select-none">
                      {item.icon}
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm text-gray-900 font-medium line-clamp-2 leading-snug">{item.name}</p>
                      <p className="text-sm font-bold text-gray-900 mt-0.5">{formatCurrency(item.priceSnapshot)}</p>
                      <div className="flex items-center gap-3 mt-1.5">
                        <QtyControl item={item} />
                        <button
                          onClick={() => removeItem(item.productId)}
                          className="text-gray-400 hover:text-red-500 transition-colors p-1"
                          aria-label={t('cartPage.remove')}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </div>
                    <p className="text-sm font-bold text-gray-900 flex-shrink-0 pt-0.5">
                      {formatCurrency(item.priceSnapshot * item.quantity)}
                    </p>
                  </div>
                ))}
              </div>
            </div>
          ))}

          <div className={cn(
            'border-t border-gray-100 pt-4 flex flex-col gap-3',
            'sticky bottom-0 bg-white -mx-4 px-4 pb-4'
          )}>
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-500">{t('cartPage.subtotal')}</span>
              <span className="font-bold text-gray-900 text-base">{formatCurrency(total)}</span>
            </div>
            <button
              onClick={goToCheckout}
              className="w-full h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm transition-colors"
            >
              {t('cartPage.goToCheckout')}
            </button>
            <button
              onClick={goToCart}
              className="w-full h-9 rounded-xl border border-gray-300 text-gray-700 text-sm font-medium hover:bg-gray-50 transition-colors"
            >
              {t('cartPage.viewCart')}
            </button>
          </div>
        </div>
      )}
    </Drawer>
  )
}
