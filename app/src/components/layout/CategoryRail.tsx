import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import { cn } from '@/lib/cn'

export function CategoryRail() {
  const { t } = useTranslation()
  const { slug } = useParams<{ slug?: string }>()

  return (
    <div className="bg-white border-b border-gray-100">
      <div className="container">
        <div
          className="flex gap-2 overflow-x-auto py-2 scrollbar-none"
          style={{ scrollbarWidth: 'none' }}
          role="navigation"
          aria-label={t('taxonomy.ferragens')}
        >
          {TOP_LEVEL_CATEGORIES.map((cat) => {
            const active = slug === cat.slug
            return (
              <Link
                key={cat.slug}
                to={`/categoria/${cat.slug}`}
                className={cn(
                  'flex-shrink-0 flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium transition-colors whitespace-nowrap',
                  active
                    ? 'bg-brand-orange text-white'
                    : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                )}
                aria-current={active ? 'page' : undefined}
              >
                <span aria-hidden>{cat.icon}</span>
                {t(cat.labelKey)}
              </Link>
            )
          })}
        </div>
      </div>
    </div>
  )
}
