import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontal } from 'lucide-react'
import { useProducts } from '@/hooks/useProducts'
import { useSearchFilters } from '@/hooks/useSearchFilters'
import { getMockFacets } from '@/lib/mockProducts'
import { ProductCard, ProductCardSkeleton } from '@/components/catalog/ProductCard'
import { FacetSidebar } from '@/components/catalog/FacetSidebar'
import { ActiveFilterChips } from '@/components/catalog/ActiveFilterChips'
import { Drawer, Pagination, Select } from '@/components/ui'
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

function EmptyState({ query, onClear }: { query: string; onClear: () => void }) {
  const { t } = useTranslation(['catalog', 'common'])
  return (
    <div className="flex flex-col items-center justify-center py-20 gap-4 text-center">
      <span className="text-6xl select-none">🔍</span>
      <div>
        <p className="font-display font-bold text-lg text-gray-900">
          {t('catalog:search.emptyTitle')} &ldquo;{query}&rdquo;
        </p>
        <p className="text-sm text-gray-500 mt-1">{t('catalog:search.emptySubtitle')}</p>
      </div>
      <button
        onClick={onClear}
        className="text-sm font-semibold text-brand-orange hover:underline"
      >
        {t('catalog:search.clearFilters')}
      </button>
      <div className="mt-4 grid grid-cols-2 sm:grid-cols-4 gap-3 w-full max-w-lg">
        {TOP_LEVEL_CATEGORIES.slice(0, 4).map((cat) => (
          <Link
            key={cat.slug}
            to={`/categoria/${cat.slug}`}
            className="flex flex-col items-center gap-1.5 p-3 bg-white border border-gray-200 rounded-xl hover:border-brand-orange hover:shadow-sm transition-all text-sm font-medium text-gray-700"
          >
            <span className="text-2xl">{cat.icon}</span>
            {t(cat.labelKey)}
          </Link>
        ))}
      </div>
    </div>
  )
}

export default function SearchPage() {
  const { t } = useTranslation(['catalog', 'common'])
  const { filters, set, toProductsParams, activeCount, clearAll } = useSearchFilters()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const productsParams = toProductsParams()
  const { data, isLoading } = useProducts(productsParams)

  const facets = getMockFacets({ q: filters.q, category: filters.category })

  const sortOptions = SORT_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))

  useEffect(() => {
    if (inputRef.current && filters.q) {
      inputRef.current.value = filters.q
    }
  }, [filters.q])

  function handleSearchSubmit(e: React.FormEvent) {
    e.preventDefault()
    const q = inputRef.current?.value.trim() ?? ''
    set({ q })
  }

  const total = data?.meta.total ?? 0
  const totalPages = data?.meta.total_pages ?? 1

  return (
    <div className="container py-4">
      {/* Search bar */}
      <form onSubmit={handleSearchSubmit} className="mb-4 flex gap-2">
        <input
          ref={inputRef}
          type="search"
          defaultValue={filters.q}
          placeholder={t('common:searchPlaceholder')}
          className="flex-1 rounded-xl border border-gray-300 px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
          aria-label={t('common:search')}
        />
        <button
          type="submit"
          className="bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold px-4 py-2.5 rounded-xl text-sm transition-colors"
        >
          {t('common:search')}
        </button>
      </form>

      {/* Title + count */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-3">
        <div>
          {filters.q ? (
            <h1 className="font-display font-black text-xl text-gray-900">
              {t('catalog:search.title')} &ldquo;{filters.q}&rdquo;
            </h1>
          ) : (
            <h1 className="font-display font-black text-xl text-gray-900">
              {t('catalog:search.titleEmpty')}
            </h1>
          )}
          {!isLoading && data && (
            <p className="text-sm text-gray-500 mt-0.5">
              {t('catalog:search.resultsCount', { count: total })}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setDrawerOpen(true)}
            className="lg:hidden flex items-center gap-1.5 border border-gray-300 rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 relative"
          >
            <SlidersHorizontal className="h-4 w-4" />
            {t('catalog:filters')}
            {activeCount > 0 && (
              <span className="absolute -top-1.5 -right-1.5 h-4 w-4 rounded-full bg-brand-orange text-white text-[10px] font-bold flex items-center justify-center">
                {activeCount}
              </span>
            )}
          </button>
          <Select
            options={sortOptions}
            value={filters.sort}
            onChange={(e) => set({ sort: e.target.value as SortOption })}
            className="text-sm w-44"
          />
        </div>
      </div>

      {/* Active chips */}
      <ActiveFilterChips filters={filters} onChange={set} onClearAll={clearAll} />

      <div className="flex gap-6 mt-4">
        {/* Desktop sidebar */}
        <div className="hidden lg:block w-56 flex-shrink-0">
          <FacetSidebar
            filters={filters}
            brands={facets.brands}
            priceMin={facets.priceMin}
            priceMax={facets.priceMax}
            onChange={set}
          />
        </div>

        {/* Mobile bottom sheet */}
        <Drawer
          open={drawerOpen}
          onClose={() => setDrawerOpen(false)}
          side="bottom"
          title={t('catalog:filters')}
        >
          <FacetSidebar
            filters={filters}
            brands={facets.brands}
            priceMin={facets.priceMin}
            priceMax={facets.priceMax}
            onChange={(u) => { set(u); setDrawerOpen(false) }}
          />
        </Drawer>

        {/* Results grid */}
        <div className="flex-1 min-w-0">
          {isLoading ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-4">
              {Array.from({ length: PER_PAGE }).map((_, i) => <ProductCardSkeleton key={i} />)}
            </div>
          ) : total === 0 ? (
            <EmptyState query={filters.q} onClear={clearAll} />
          ) : (
            <>
              <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-4">
                {data!.data.map((p) => <ProductCard key={p.id} product={p} />)}
              </div>
              {totalPages > 1 && (
                <div className="flex justify-center mt-8">
                  <Pagination
                    page={filters.page}
                    totalPages={totalPages}
                    onPageChange={(p) => { set({ page: p }); window.scrollTo({ top: 0, behavior: 'smooth' }) }}
                  />
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
