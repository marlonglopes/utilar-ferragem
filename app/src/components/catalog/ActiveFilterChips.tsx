import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import type { SearchFilters } from '@/hooks/useSearchFilters'
import { formatCurrency } from '@/lib/format'

interface ActiveFilterChipsProps {
  filters: SearchFilters
  onChange: (updates: Partial<SearchFilters>) => void
  onClearAll: () => void
}

export function ActiveFilterChips({ filters, onChange, onClearAll }: ActiveFilterChipsProps) {
  const { t } = useTranslation(['catalog', 'common'])

  const chips: { label: string; onRemove: () => void }[] = []

  if (filters.category) {
    const cat = TOP_LEVEL_CATEGORIES.find((c) => c.slug === filters.category)
    chips.push({
      label: cat ? `${cat.icon} ${t(cat.labelKey)}` : filters.category,
      onRemove: () => onChange({ category: '' }),
    })
  }

  if (filters.brand) {
    chips.push({ label: filters.brand, onRemove: () => onChange({ brand: '' }) })
  }

  if (filters.priceMin) {
    chips.push({
      label: `≥ ${formatCurrency(Number(filters.priceMin))}`,
      onRemove: () => onChange({ priceMin: '' }),
    })
  }

  if (filters.priceMax) {
    chips.push({
      label: `≤ ${formatCurrency(Number(filters.priceMax))}`,
      onRemove: () => onChange({ priceMax: '' }),
    })
  }

  if (filters.inStock) {
    chips.push({
      label: t('catalog:search.inStockOnly'),
      onRemove: () => onChange({ inStock: false }),
    })
  }

  if (chips.length === 0) return null

  return (
    <div className="flex flex-wrap items-center gap-2">
      {chips.map(({ label, onRemove }) => (
        <span
          key={label}
          className="inline-flex items-center gap-1 bg-brand-orange/10 text-brand-orange text-xs font-semibold px-2.5 py-1 rounded-full"
        >
          {label}
          <button
            onClick={onRemove}
            className="hover:bg-brand-orange/20 rounded-full p-0.5 transition-colors"
            aria-label={`Remover filtro ${label}`}
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      {chips.length > 1 && (
        <button
          onClick={onClearAll}
          className="text-xs text-gray-500 hover:text-gray-800 underline"
        >
          {t('catalog:search.clearFilters')}
        </button>
      )}
    </div>
  )
}
