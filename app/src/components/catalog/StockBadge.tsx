import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/cn'

interface StockBadgeProps {
  stock: number | null | undefined
  className?: string
}

export function StockBadge({ stock, className }: StockBadgeProps) {
  const { t } = useTranslation('catalog')

  if (stock == null) {
    return <span className={cn('inline-flex items-center gap-1.5 text-sm font-medium text-gray-500', className)}>
      <span className="h-2 w-2 rounded-full bg-gray-400" />
      {t('product.onRequest')}
    </span>
  }

  if (stock === 0) {
    return <span className={cn('inline-flex items-center gap-1.5 text-sm font-medium text-red-600', className)}>
      <span className="h-2 w-2 rounded-full bg-red-500" />
      {t('product.outOfStock')}
    </span>
  }

  if (stock < 10) {
    return <span className={cn('inline-flex items-center gap-1.5 text-sm font-medium text-yellow-600', className)}>
      <span className="h-2 w-2 rounded-full bg-yellow-500" />
      {t('product.lastUnits')} ({stock})
    </span>
  }

  return <span className={cn('inline-flex items-center gap-1.5 text-sm font-medium text-green-700', className)}>
    <span className="h-2 w-2 rounded-full bg-green-500" />
    {t('product.inStock')}
  </span>
}
