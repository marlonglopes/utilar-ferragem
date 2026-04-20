import { Link } from 'react-router-dom'
import { cn } from '@/lib/cn'
import { formatCurrency } from '@/lib/format'
import { Skeleton } from '@/components/ui'
import type { Product } from '@/types/product'

export interface ProductCardProps {
  product: Product
  className?: string
}

const badgeClasses = {
  discount: 'bg-brand-orange text-white',
  free_shipping: 'bg-green-600 text-white',
  last_units: 'bg-yellow-500 text-white',
}

function Stars({ rating, count }: { rating: number; count: number }) {
  return (
    <div className="flex items-center gap-1 text-xs text-gray-500">
      <span className="text-yellow-400">
        {'★'.repeat(Math.round(rating))}
        {'☆'.repeat(5 - Math.round(rating))}
      </span>
      <span>({count})</span>
    </div>
  )
}

export function ProductCard({ product, className }: ProductCardProps) {
  const {
    slug, name, icon, seller, price, originalPrice, rating, reviewCount,
    cashbackAmount, badge, badgeLabel, installments, stock,
  } = product

  const discount = originalPrice
    ? Math.round(((originalPrice - price) / originalPrice) * 100)
    : null

  return (
    <Link
      to={`/produto/${slug}`}
      className={cn(
        'group flex flex-col bg-white border border-gray-200 rounded-xl overflow-hidden',
        'hover:shadow-md hover:border-gray-300 transition-all duration-200',
        className
      )}
    >
      <div className="relative bg-gray-50 flex items-center justify-center text-5xl h-40 select-none">
        {icon}
        {badge && badgeLabel && (
          <span
            className={cn(
              'absolute top-2 left-2 text-xs font-bold px-2 py-0.5 rounded-full',
              badgeClasses[badge]
            )}
          >
            {badgeLabel}
          </span>
        )}
        {stock <= 5 && stock > 0 && !badge && (
          <span className="absolute top-2 left-2 text-xs font-bold px-2 py-0.5 rounded-full bg-yellow-500 text-white">
            Últimas {stock}
          </span>
        )}
      </div>

      <div className="flex flex-col gap-1 p-3 flex-1">
        <p className="text-xs text-gray-400 truncate">{seller}</p>

        <p className="text-sm text-gray-900 font-medium leading-snug line-clamp-2 group-hover:text-brand-orange transition-colors">
          {name}
        </p>

        <Stars rating={rating} count={reviewCount} />

        <div className="mt-auto pt-2">
          {originalPrice && (
            <p className="text-xs text-gray-400 line-through">
              {formatCurrency(originalPrice)}
              {discount && <span className="ml-1 text-brand-orange font-semibold">-{discount}%</span>}
            </p>
          )}
          <p className="text-base font-bold text-gray-900">
            {formatCurrency(price)}
            {installments && (
              <span className="ml-1 text-xs font-normal text-gray-500">
                em {installments}x
              </span>
            )}
          </p>

          {cashbackAmount && (
            <p className="text-xs font-semibold text-brand-orange mt-1">
              Ganhe {formatCurrency(cashbackAmount)} em cashback
            </p>
          )}
        </div>
      </div>
    </Link>
  )
}

export function ProductCardSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn('flex flex-col bg-white border border-gray-200 rounded-xl overflow-hidden', className)}>
      <Skeleton className="h-40 w-full rounded-none" />
      <div className="p-3 flex flex-col gap-2">
        <Skeleton className="h-3 w-16" />
        <Skeleton variant="text" lines={2} />
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-5 w-20 mt-2" />
      </div>
    </div>
  )
}
