import { useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Minus, Plus, ShoppingCart } from 'lucide-react'
import { useProduct } from '@/hooks/useProduct'
import { useProducts } from '@/hooks/useProducts'
import { ImageGallery } from '@/components/catalog/ImageGallery'
import { StockBadge } from '@/components/catalog/StockBadge'
import { SellerCard } from '@/components/catalog/SellerCard'
import { SpecSheet } from '@/components/catalog/SpecSheet'
import { ProductCard, ProductCardSkeleton } from '@/components/catalog/ProductCard'
import { Breadcrumb, Skeleton } from '@/components/ui'
import { formatCurrency } from '@/lib/format'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import { cn } from '@/lib/cn'

type Tab = 'description' | 'specs' | 'reviews'

function Stars({ rating, count }: { rating: number; count: number }) {
  return (
    <div className="flex items-center gap-1.5 text-sm text-gray-500">
      <span className="text-yellow-400 text-base">
        {'★'.repeat(Math.round(rating))}{'☆'.repeat(5 - Math.round(rating))}
      </span>
      <span className="font-medium text-gray-700">{rating.toFixed(1)}</span>
      <span>({count.toLocaleString('pt-BR')})</span>
    </div>
  )
}

function QuantitySelector({
  value,
  max,
  onChange,
}: {
  value: number
  max: number
  onChange: (v: number) => void
}) {
  return (
    <div className="flex items-center gap-0">
      <button
        onClick={() => onChange(Math.max(1, value - 1))}
        disabled={value <= 1}
        className="h-10 w-10 rounded-l-lg border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        aria-label="Diminuir quantidade"
      >
        <Minus className="h-4 w-4" />
      </button>
      <div className="h-10 w-12 border-t border-b border-gray-300 flex items-center justify-center text-sm font-semibold">
        {value}
      </div>
      <button
        onClick={() => onChange(Math.min(max, value + 1))}
        disabled={value >= max}
        className="h-10 w-10 rounded-r-lg border border-gray-300 flex items-center justify-center hover:bg-gray-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        aria-label="Aumentar quantidade"
      >
        <Plus className="h-4 w-4" />
      </button>
    </div>
  )
}

function DetailSkeleton() {
  return (
    <div className="container py-6">
      <Skeleton className="h-4 w-64 mb-6" />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        <Skeleton className="aspect-square w-full rounded-2xl" />
        <div className="flex flex-col gap-4">
          <Skeleton className="h-4 w-32" />
          <Skeleton variant="text" lines={3} />
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-8 w-36 mt-2" />
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-12 w-full mt-4" />
        </div>
      </div>
    </div>
  )
}

export default function ProductDetailPage() {
  const { slug = '' } = useParams<{ slug: string }>()
  const { t } = useTranslation(['catalog', 'common'])
  const [qty, setQty] = useState(1)
  const [tab, setTab] = useState<Tab>('description')

  const { data: product, isLoading, isError } = useProduct(slug)

  const category = product ? TOP_LEVEL_CATEGORIES.find((c) => c.slug === product.category) : undefined

  const { data: relatedData } = useProducts({
    category: product?.category,
    per_page: 5,
  })

  const related = relatedData?.data.filter((p) => p.slug !== slug).slice(0, 4) ?? []

  if (isLoading) return <DetailSkeleton />
  if (isError || product === null) return <Navigate to="/404" replace />
  if (!product) return null

  const discount = product.originalPrice
    ? Math.round(((product.originalPrice - product.price) / product.originalPrice) * 100)
    : null

  const breadcrumb = [
    { label: t('common:home.categories'), href: '/' },
    ...(category ? [{ label: t(category.labelKey), href: `/categoria/${category.slug}` }] : []),
    { label: product.name },
  ]

  const tabs: { id: Tab; label: string }[] = [
    { id: 'description', label: t('catalog:product.description') },
    { id: 'specs', label: t('catalog:product.specifications') },
    { id: 'reviews', label: t('catalog:product.reviews') },
  ]

  return (
    <>
      <div className="container py-4 pb-28 lg:pb-6">
        <Breadcrumb items={breadcrumb} className="mb-4" />

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 items-start">
          {/* Gallery */}
          <ImageGallery images={product.images} icon={product.icon} productName={product.name} />

          {/* Info panel */}
          <div className="flex flex-col gap-4">
            <div>
              <p className="text-xs text-gray-400 mb-1">{product.seller}</p>
              <h1 className="font-display font-black text-2xl text-gray-900 leading-tight">{product.name}</h1>
              <div className="mt-2">
                <Stars rating={product.rating} count={product.reviewCount} />
              </div>
            </div>

            <StockBadge stock={product.stock} />

            {/* Price block */}
            <div>
              {product.originalPrice && (
                <p className="text-sm text-gray-400 line-through">
                  {formatCurrency(product.originalPrice)}
                  {discount && (
                    <span className="ml-2 text-brand-orange font-semibold">-{discount}%</span>
                  )}
                </p>
              )}
              <p className="text-3xl font-bold text-gray-900">{formatCurrency(product.price)}</p>
              {product.installments && (
                <p className="text-sm text-gray-500 mt-0.5">
                  {t('catalog:product.installments', { count: product.installments })}
                </p>
              )}
              {product.cashbackAmount && (
                <p className="text-sm font-semibold text-brand-orange mt-1">
                  {t('catalog:product.cashback', { amount: formatCurrency(product.cashbackAmount) })}
                </p>
              )}
            </div>

            {/* Quantity */}
            {product.stock > 0 && (
              <div className="flex items-center gap-3">
                <span className="text-sm text-gray-600">{t('catalog:product.quantity')}</span>
                <QuantitySelector value={qty} max={product.stock} onChange={setQty} />
              </div>
            )}

            {/* CTA */}
            <button
              disabled={product.stock === 0}
              className={cn(
                'flex items-center justify-center gap-2 h-12 rounded-xl font-semibold text-base transition-colors',
                product.stock === 0
                  ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
                  : 'bg-brand-orange hover:bg-brand-orange-dark text-white'
              )}
            >
              <ShoppingCart className="h-5 w-5" />
              {t('catalog:product.addToCart')}
            </button>

            {/* Seller card */}
            <SellerCard
              sellerId={product.sellerId}
              sellerName={product.seller}
              sellerRating={product.sellerRating}
              sellerReviewCount={product.sellerReviewCount}
            />
          </div>
        </div>

        {/* Tabs */}
        <div className="mt-10">
          <div className="flex border-b border-gray-200 gap-1">
            {tabs.map(({ id, label }) => (
              <button
                key={id}
                onClick={() => setTab(id)}
                className={cn(
                  'px-4 py-2.5 text-sm font-semibold transition-colors border-b-2 -mb-px',
                  tab === id
                    ? 'border-brand-orange text-brand-orange'
                    : 'border-transparent text-gray-500 hover:text-gray-800'
                )}
              >
                {label}
              </button>
            ))}
          </div>

          <div className="pt-5">
            {tab === 'description' && (
              <p className="text-sm text-gray-700 leading-relaxed whitespace-pre-line max-w-2xl">
                {product.description ?? t('catalog:product.noDescription')}
              </p>
            )}
            {tab === 'specs' && <SpecSheet specs={product.specs} className="max-w-2xl" />}
            {tab === 'reviews' && (
              <div className="flex flex-col items-center justify-center py-12 gap-3 text-gray-400">
                <span className="text-4xl">★</span>
                <p className="text-sm">{t('catalog:product.reviewsStub')}</p>
              </div>
            )}
          </div>
        </div>

        {/* Related products */}
        {related.length > 0 && (
          <div className="mt-12">
            <h2 className="font-display font-bold text-lg text-gray-900 mb-4">
              {t('catalog:product.relatedProducts')}
            </h2>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              {isLoading
                ? Array.from({ length: 4 }).map((_, i) => <ProductCardSkeleton key={i} />)
                : related.map((p) => <ProductCard key={p.id} product={p} />)
              }
            </div>
          </div>
        )}
      </div>

      {/* Mobile fixed CTA */}
      <div className="fixed bottom-0 left-0 right-0 z-40 lg:hidden bg-white border-t border-gray-200 px-4 py-3 flex items-center gap-3 safe-area-bottom">
        <div className="flex-1 min-w-0">
          {product.originalPrice && (
            <p className="text-xs text-gray-400 line-through leading-none">{formatCurrency(product.originalPrice)}</p>
          )}
          <p className="text-lg font-bold text-gray-900 leading-tight">{formatCurrency(product.price)}</p>
        </div>
        <button
          disabled={product.stock === 0}
          className={cn(
            'flex items-center gap-2 h-11 px-5 rounded-xl font-semibold text-sm transition-colors flex-shrink-0',
            product.stock === 0
              ? 'bg-gray-200 text-gray-400 cursor-not-allowed'
              : 'bg-brand-orange hover:bg-brand-orange-dark text-white'
          )}
        >
          <ShoppingCart className="h-4 w-4" />
          {t('catalog:product.addToCart')}
        </button>
      </div>
    </>
  )
}
