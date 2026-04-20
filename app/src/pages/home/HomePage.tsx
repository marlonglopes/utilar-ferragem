import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useProducts } from '@/hooks/useProducts'
import { ProductCard, ProductCardSkeleton } from '@/components/catalog/ProductCard'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'

function Hero() {
  const { t } = useTranslation()
  return (
    <section className="bg-brand-blue text-white py-16 sm:py-20">
      <div className="container">
        <div className="flex flex-col sm:flex-row items-center gap-10">
          <div className="flex-1 min-w-0">
            <span className="inline-flex items-center gap-1.5 text-xs font-semibold bg-white/10 text-brand-orange-light px-3 py-1 rounded-full mb-5">
              ⚡ {t('hero.badge')}
            </span>
            <h1 className="font-display font-black text-3xl sm:text-4xl leading-tight mb-4">
              {t('hero.title.prefix')}{' '}
              <span className="text-brand-orange">{t('hero.title.highlight')}</span>
              {t('hero.title.suffix')}
            </h1>
            <p className="text-blue-200 text-base leading-relaxed mb-7 max-w-lg">
              {t('hero.description')}
            </p>
            <div className="flex flex-wrap gap-3">
              <Link to="/categoria/ferramentas" className="inline-flex items-center gap-2 bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold px-5 py-2.5 rounded-lg transition-colors">
                {t('hero.cta.catalog')} →
              </Link>
              <Link to="/vender" className="inline-flex items-center gap-2 border border-white/30 hover:bg-white/10 text-white font-semibold px-5 py-2.5 rounded-lg transition-colors">
                {t('hero.cta.sell')}
              </Link>
            </div>
          </div>
          <div className="hidden sm:grid grid-cols-3 gap-2 flex-shrink-0">
            {['⚒', '▦', '⚡', '◡', '⛏', '▥', '▣', '⚠', '❀'].map((icon, i) => (
              <div
                key={i}
                className={`w-16 h-16 rounded-xl flex items-center justify-center text-2xl select-none ${
                  i === 4 ? 'bg-brand-orange/30 text-brand-orange-light' :
                  i === 1 || i === 7 ? 'bg-white/10 text-white/60' :
                  'bg-white/10 text-white/80'
                }`}
              >
                {icon}
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

function CategoryGrid() {
  const { t } = useTranslation()
  return (
    <section className="py-10">
      <div className="container">
        <div className="flex items-center justify-between mb-5">
          <h2 className="font-display font-black text-xl text-gray-900">{t('home.categories')}</h2>
          <Link to="/categorias" className="text-sm text-brand-orange hover:underline font-medium">
            {t('home.seeAll')} →
          </Link>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-8 gap-3">
          {TOP_LEVEL_CATEGORIES.map((cat) => (
            <Link
              key={cat.slug}
              to={`/categoria/${cat.slug}`}
              className="flex flex-col items-center gap-2 p-3 rounded-xl border border-gray-200 bg-white hover:border-brand-orange hover:shadow-sm transition-all group"
            >
              <span className="text-3xl select-none">{cat.icon}</span>
              <span className="text-xs font-semibold text-gray-700 text-center group-hover:text-brand-orange transition-colors leading-tight">
                {t(cat.labelKey)}
              </span>
            </Link>
          ))}
        </div>
      </div>
    </section>
  )
}

function FeaturedProducts() {
  const { t } = useTranslation()
  const { data, isLoading } = useProducts({ per_page: 8 })

  return (
    <section className="py-10 bg-gray-50">
      <div className="container">
        <div className="flex items-center justify-between mb-5">
          <h2 className="font-display font-black text-xl text-gray-900">{t('home.featured')}</h2>
          <Link to="/categoria/ferramentas" className="text-sm text-brand-orange hover:underline font-medium">
            {t('home.seeAll')} →
          </Link>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
          {isLoading
            ? Array.from({ length: 8 }).map((_, i) => <ProductCardSkeleton key={i} />)
            : data?.data.map((p) => <ProductCard key={p.id} product={p} />)}
        </div>
      </div>
    </section>
  )
}

const TRUST_ITEMS = [
  { icon: '⚡', titleKey: 'trust.pix.title', descKey: 'trust.pix.desc' },
  { icon: '✓', titleKey: 'trust.verified.title', descKey: 'trust.verified.desc' },
  { icon: '⎈', titleKey: 'trust.delivery.title', descKey: 'trust.delivery.desc' },
  { icon: '☎', titleKey: 'trust.support.title', descKey: 'trust.support.desc' },
]

function TrustRow() {
  const { t } = useTranslation()
  return (
    <section className="py-10 border-t border-gray-200">
      <div className="container">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {TRUST_ITEMS.map(({ icon, titleKey, descKey }) => (
            <div key={titleKey} className="flex items-start gap-3">
              <span className="w-10 h-10 rounded-xl bg-brand-orange-light flex items-center justify-center text-xl flex-shrink-0 select-none">
                {icon}
              </span>
              <div>
                <p className="font-display font-bold text-sm text-gray-900">{t(titleKey)}</p>
                <p className="text-xs text-gray-500 leading-relaxed mt-0.5">{t(descKey)}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

export default function HomePage() {
  return (
    <>
      <Hero />
      <CategoryGrid />
      <FeaturedProducts />
      <TrustRow />
    </>
  )
}
