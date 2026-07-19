import { useState, useEffect } from 'react'
import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/cn'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import type { SearchFilters } from '@/hooks/useSearchFilters'

interface FacetSidebarProps {
  filters: SearchFilters
  brands: { value: string; count: number }[]
  priceMin: number
  priceMax: number
  onChange: (updates: Partial<SearchFilters>) => void
  /**
   * Esconde o grupo de categorias. Usado na CategoryPage, onde a categoria vem
   * da própria rota (`/categoria/:slug`) e trocá-la significa navegar, não
   * filtrar.
   */
  showCategoryFacet?: boolean
}

function FacetGroup({ title, defaultOpen = true, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="bg-white border border-gray-200 rounded-xl overflow-hidden">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between px-4 py-3 text-sm font-semibold text-gray-900 hover:bg-gray-50 transition-colors"
      >
        {title}
        <ChevronDown className={cn('h-4 w-4 text-gray-400 transition-transform', open && 'rotate-180')} />
      </button>
      {open && <div className="px-4 pb-4 flex flex-col gap-2">{children}</div>}
    </div>
  )
}

export function FacetSidebar({
  filters,
  brands,
  priceMin,
  priceMax,
  onChange,
  showCategoryFacet = true,
}: FacetSidebarProps) {
  const { t } = useTranslation(['catalog', 'common'])

  const [localMin, setLocalMin] = useState(filters.priceMin)
  const [localMax, setLocalMax] = useState(filters.priceMax)

  useEffect(() => {
    setLocalMin(filters.priceMin)
    setLocalMax(filters.priceMax)
  }, [filters.priceMin, filters.priceMax])

  function applyPrice() {
    onChange({ priceMin: localMin, priceMax: localMax })
  }

  return (
    <aside className="flex flex-col gap-3">
      {/* Category */}
      {showCategoryFacet && (
      <FacetGroup title={t('common:home.categories')}>
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="radio"
            name="category"
            checked={!filters.category}
            onChange={() => onChange({ category: '' })}
            className="text-brand-orange focus:ring-brand-orange"
          />
          <span className="text-gray-700">{t('catalog:search.allCategories')}</span>
        </label>
        {TOP_LEVEL_CATEGORIES.map((cat) => (
          <label key={cat.slug} className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="radio"
              name="category"
              checked={filters.category === cat.slug}
              onChange={() => onChange({ category: cat.slug })}
              className="text-brand-orange focus:ring-brand-orange"
            />
            <span className="text-gray-700">{cat.icon} {t(cat.labelKey)}</span>
          </label>
        ))}
      </FacetGroup>
      )}

      {/* Brand — o filtro aceita UMA marca por vez (o backend recebe `brand`
          como string única), então a UI usa radio. Antes eram checkboxes, que
          sugeriam multi-seleção e desmarcavam a anterior a cada clique. */}
      {brands.length > 0 && (
        <FacetGroup title={t('catalog:search.brands')}>
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="radio"
              name="brand"
              checked={!filters.brand}
              onChange={() => onChange({ brand: '' })}
              className="text-brand-orange focus:ring-brand-orange"
            />
            <span className="flex-1 text-gray-700">{t('catalog:search.allBrands')}</span>
          </label>
          {brands.map(({ value, count }) => (
            <label key={value} className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="radio"
                name="brand"
                checked={filters.brand === value}
                onChange={() => onChange({ brand: value })}
                className="text-brand-orange focus:ring-brand-orange"
              />
              <span className="flex-1 text-gray-700">{value}</span>
              <span className="text-xs text-gray-400">{count}</span>
            </label>
          ))}
        </FacetGroup>
      )}

      {/* Price range */}
      <FacetGroup title={t('catalog:search.priceRange')}>
        <div className="flex gap-2">
          <input
            type="number"
            placeholder={`R$ ${priceMin}`}
            value={localMin}
            onChange={(e) => setLocalMin(e.target.value)}
            onBlur={applyPrice}
            onKeyDown={(e) => e.key === 'Enter' && applyPrice()}
            className="w-full rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
            min={0}
          />
          <input
            type="number"
            placeholder={`R$ ${priceMax}`}
            value={localMax}
            onChange={(e) => setLocalMax(e.target.value)}
            onBlur={applyPrice}
            onKeyDown={(e) => e.key === 'Enter' && applyPrice()}
            className="w-full rounded-lg border border-gray-300 px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-orange"
            min={0}
          />
        </div>
      </FacetGroup>

      {/* In stock */}
      <div className="bg-white border border-gray-200 rounded-xl px-4 py-3">
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={filters.inStock}
            onChange={(e) => onChange({ inStock: e.target.checked })}
            className="h-4 w-4 rounded border-gray-300 text-brand-orange focus:ring-brand-orange focus:ring-offset-0"
          />
          <span className="font-medium text-gray-900">{t('catalog:search.inStockOnly')}</span>
        </label>
      </div>
    </aside>
  )
}
