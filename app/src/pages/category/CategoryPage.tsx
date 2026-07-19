import { useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontal } from 'lucide-react'
import { useProducts } from '@/hooks/useProducts'
import { useFacets } from '@/hooks/useFacets'
import { useSearchFilters } from '@/hooks/useSearchFilters'
import { ProductCard, ProductCardSkeleton } from '@/components/catalog/ProductCard'
import { FacetSidebar } from '@/components/catalog/FacetSidebar'
import { ActiveFilterChips } from '@/components/catalog/ActiveFilterChips'
import { Seo } from '@/components/seo/Seo'
import { Breadcrumb, Drawer, Pagination, Select } from '@/components/ui'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import { breadcrumbListSchema } from '@/lib/seo'
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

export default function CategoryPage() {
  const { slug } = useParams<{ slug: string }>()
  const { t } = useTranslation(['common', 'catalog'])
  const [drawerOpen, setDrawerOpen] = useState(false)

  // Marca, preço, estoque, ordenação e página vivem na URL (useSearchFilters),
  // não em useState — assim a listagem filtrada é compartilhável, sobrevive ao
  // reload e ao botão voltar. A categoria vem da rota, não do query string.
  const { filters, set, toProductsParams, activeCount, clearAll } = useSearchFilters()

  const category = TOP_LEVEL_CATEGORIES.find((c) => c.slug === slug)

  const { data, isLoading } = useProducts({
    ...toProductsParams(),
    category: slug,
    per_page: PER_PAGE,
  })
  const { data: facets } = useFacets({ category: slug })

  if (!category) return <Navigate to="/404" replace />

  const sortOptions = SORT_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))
  const categoryLabel = t(category.labelKey)
  const path = `/categoria/${category.slug}`

  const breadcrumb = [
    { label: t('common:home.categories'), href: '/categorias' },
    { label: categoryLabel },
  ]

  return (
    <div className="container py-4">
      <Seo
        title={categoryLabel}
        description={`${categoryLabel} na UtiLar Ferragem: compare preços, marcas e disponibilidade de vendedores com CNPJ verificado. Pague no Pix, boleto ou cartão.`}
        path={path}
        jsonLd={breadcrumbListSchema([
          { name: 'Início', path: '/' },
          { name: 'Categorias', path: '/categorias' },
          { name: categoryLabel },
        ])}
      />

      <Breadcrumb items={breadcrumb} className="mb-3" />

      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
        <div>
          <h1 className="font-display font-black text-2xl text-gray-900">
            {category.icon} {categoryLabel}
          </h1>
          {data && (
            <p className="text-sm text-gray-500 mt-0.5">
              {t('catalog:category.productsCount', { count: data.meta.total })}
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
            onChange={(e) => set({ sort: e.target.value as SortOption, page: 1 })}
            className="text-sm w-44"
            aria-label={t('catalog:sort.label')}
          />
        </div>
      </div>

      <ActiveFilterChips filters={filters} onChange={set} onClearAll={clearAll} />

      <div className="flex gap-6 mt-4">
        {/* Sidebar desktop */}
        <div className="hidden lg:block w-56 flex-shrink-0">
          <FacetSidebar
            filters={filters}
            brands={facets?.brands ?? []}
            priceMin={facets?.priceMin ?? 0}
            priceMax={facets?.priceMax ?? 9999}
            onChange={set}
            showCategoryFacet={false}
          />
        </div>

        {/* Bottom sheet mobile */}
        <Drawer
          open={drawerOpen}
          onClose={() => setDrawerOpen(false)}
          side="bottom"
          title={t('catalog:filters')}
        >
          <FacetSidebar
            filters={filters}
            brands={facets?.brands ?? []}
            priceMin={facets?.priceMin ?? 0}
            priceMax={facets?.priceMax ?? 9999}
            onChange={(u) => {
              set(u)
              setDrawerOpen(false)
            }}
            showCategoryFacet={false}
          />
        </Drawer>

        <div className="flex-1 min-w-0">
          <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-4">
            {isLoading ? (
              Array.from({ length: PER_PAGE }).map((_, i) => <ProductCardSkeleton key={i} />)
            ) : data?.data.length === 0 ? (
              <div className="col-span-full flex flex-col items-center justify-center py-20 gap-3 text-gray-400">
                <span className="text-5xl">{category.icon}</span>
                <p className="font-semibold">{t('catalog:noResults')}</p>
                {activeCount > 0 && (
                  <button
                    onClick={clearAll}
                    className="text-sm font-semibold text-brand-orange hover:underline"
                  >
                    {t('catalog:clearFilters')}
                  </button>
                )}
              </div>
            ) : (
              data?.data.map((p) => <ProductCard key={p.id} product={p} />)
            )}
          </div>

          {data && data.meta.total_pages > 1 && (
            <div className="flex justify-center mt-8">
              <Pagination
                page={filters.page}
                totalPages={data.meta.total_pages}
                onPageChange={(p) => {
                  set({ page: p })
                  window.scrollTo({ top: 0, behavior: 'smooth' })
                }}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
