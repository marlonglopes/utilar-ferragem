import { useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontal, X } from 'lucide-react'
import { useProducts } from '@/hooks/useProducts'
import { ProductCard, ProductCardSkeleton } from '@/components/catalog/ProductCard'
import { Breadcrumb, Pagination, Select } from '@/components/ui'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import type { ProductsParams } from '@/types/product'

const PER_PAGE = 12

type SortOption = NonNullable<ProductsParams['sort']>

const SORT_OPTIONS: { value: SortOption; labelKey: string }[] = [
  { value: 'relevance', labelKey: 'catalog:sort.relevance' },
  { value: 'price_asc', labelKey: 'catalog:sort.priceAsc' },
  { value: 'price_desc', labelKey: 'catalog:sort.priceDesc' },
  { value: 'top_rated', labelKey: 'catalog:sort.topRated' },
  { value: 'newest', labelKey: 'catalog:sort.newest' },
]

function FilterSidebar({
  onClose,
}: {
  onClose?: () => void
}) {
  const { t } = useTranslation('catalog')
  return (
    <aside className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h3 className="font-display font-bold text-sm text-gray-900">{t('filters')}</h3>
        {onClose && (
          <button onClick={onClose} className="text-gray-400 hover:text-gray-700 lg:hidden">
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {[
        { label: t('facets.brand'), options: ['Bosch', 'Makita', 'DeWalt', 'Black+Decker', 'Tramontina'] },
        { label: t('facets.inStock'), options: ['Em estoque', 'Com frete grátis', 'Com cashback'] },
      ].map(({ label, options }) => (
        <div key={label} className="bg-white border border-gray-200 rounded-xl p-4">
          <h4 className="text-sm font-semibold text-gray-900 mb-3">{label}</h4>
          <div className="flex flex-col gap-2">
            {options.map((opt) => (
              <label key={opt} className="flex items-center gap-2 text-sm text-gray-700 cursor-pointer">
                <input
                  type="checkbox"
                  className="h-4 w-4 rounded border-gray-300 text-brand-orange focus:ring-brand-orange focus:ring-offset-0"
                />
                {opt}
              </label>
            ))}
          </div>
        </div>
      ))}

      <div className="bg-white border border-gray-200 rounded-xl p-4">
        <h4 className="text-sm font-semibold text-gray-900 mb-3">{t('facets.price')}</h4>
        <div className="flex gap-2">
          <input
            type="number"
            placeholder="R$ Mín"
            className="w-full rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
          />
          <input
            type="number"
            placeholder="R$ Máx"
            className="w-full rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
          />
        </div>
      </div>
    </aside>
  )
}

export default function CategoryPage() {
  const { slug } = useParams<{ slug: string }>()
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [sort, setSort] = useState<SortOption>('relevance')
  const [filtersOpen, setFiltersOpen] = useState(false)

  const category = TOP_LEVEL_CATEGORIES.find((c) => c.slug === slug)
  const { data, isLoading } = useProducts({
    category: category ? slug : undefined,
    page,
    per_page: PER_PAGE,
    sort,
  })

  if (!category) return <Navigate to="/404" replace />

  const sortOptions = SORT_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))

  const breadcrumb = [
    { label: t('home.categories'), href: '/' },
    { label: t(category.labelKey) },
  ]

  return (
    <div className="container py-4">
      <Breadcrumb items={breadcrumb} className="mb-3" />

      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
        <div>
          <h1 className="font-display font-black text-2xl text-gray-900">
            {category.icon} {t(category.labelKey)}
          </h1>
          {data && (
            <p className="text-sm text-gray-500 mt-0.5">
              {data.meta.total} {t('catalog:category.productsCount', { count: data.meta.total })}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setFiltersOpen((v) => !v)}
            className="lg:hidden flex items-center gap-1.5 border border-gray-300 rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            <SlidersHorizontal className="h-4 w-4" />
            {t('catalog:filters')}
          </button>
          <Select
            options={sortOptions}
            value={sort}
            onChange={(e) => { setSort(e.target.value as SortOption); setPage(1) }}
            className="text-sm w-44"
          />
        </div>
      </div>

      <div className="flex gap-6">
        <div className={`hidden lg:block w-56 flex-shrink-0`}>
          <FilterSidebar />
        </div>

        {filtersOpen && (
          <div className="fixed inset-0 z-50 lg:hidden">
            <div className="absolute inset-0 bg-black/40" onClick={() => setFiltersOpen(false)} />
            <div className="absolute left-0 top-0 bottom-0 w-72 bg-gray-50 p-4 overflow-y-auto">
              <FilterSidebar onClose={() => setFiltersOpen(false)} />
            </div>
          </div>
        )}

        <div className="flex-1 min-w-0">
          <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-4">
            {isLoading
              ? Array.from({ length: PER_PAGE }).map((_, i) => <ProductCardSkeleton key={i} />)
              : data?.data.length === 0
                ? (
                  <div className="col-span-full flex flex-col items-center justify-center py-20 text-gray-400">
                    <span className="text-5xl mb-4">{category.icon}</span>
                    <p className="font-semibold">{t('catalog:noResults')}</p>
                  </div>
                )
                : data?.data.map((p) => <ProductCard key={p.id} product={p} />)}
          </div>

          {data && data.meta.total_pages > 1 && (
            <div className="flex justify-center mt-8">
              <Pagination
                page={page}
                totalPages={data.meta.total_pages}
                onPageChange={(p) => { setPage(p); window.scrollTo({ top: 0, behavior: 'smooth' }) }}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
