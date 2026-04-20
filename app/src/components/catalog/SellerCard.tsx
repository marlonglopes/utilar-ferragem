import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/cn'

interface SellerCardProps {
  sellerId?: string
  sellerName: string
  sellerRating?: number
  sellerReviewCount?: number
  className?: string
}

function Avatar({ name }: { name: string }) {
  const initials = name
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? '')
    .join('')
  return (
    <div className="h-10 w-10 rounded-full bg-brand-blue flex items-center justify-center text-white text-sm font-bold flex-shrink-0 select-none">
      {initials}
    </div>
  )
}

export function SellerCard({ sellerId, sellerName, sellerRating, sellerReviewCount, className }: SellerCardProps) {
  const { t } = useTranslation('catalog')

  return (
    <div className={cn('flex items-center gap-3 bg-gray-50 border border-gray-200 rounded-xl p-3', className)}>
      <Avatar name={sellerName} />
      <div className="flex-1 min-w-0">
        <p className="text-xs text-gray-500">{t('product.soldBy')}</p>
        <p className="text-sm font-semibold text-gray-900 truncate">{sellerName}</p>
        {sellerRating != null && (
          <p className="text-xs text-gray-500 mt-0.5">
            <span className="text-yellow-400">{'★'.repeat(Math.round(sellerRating))}</span>
            {' '}{sellerRating.toFixed(1)}
            {sellerReviewCount != null && <span className="ml-1">({sellerReviewCount.toLocaleString('pt-BR')})</span>}
          </p>
        )}
      </div>
      {sellerId && (
        <Link
          to={`/loja/${sellerId}`}
          className="text-xs font-semibold text-brand-orange hover:underline flex-shrink-0"
        >
          {t('product.viewStore')}
        </Link>
      )}
    </div>
  )
}
