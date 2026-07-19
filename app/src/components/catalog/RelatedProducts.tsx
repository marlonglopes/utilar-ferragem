import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Info, ArrowRight } from 'lucide-react'
import { ProductCard, ProductCardSkeleton } from './ProductCard'
import type { RelatedProduct, RelatedStrategy } from '@/hooks/useRelatedProducts'

export interface RelatedProductsProps {
  products: RelatedProduct[]
  /** De onde a lista veio. Define o título — e o título é uma promessa. */
  strategy: RelatedStrategy
  /** `true` ⇒ algum item veio do preenchimento por categoria. */
  fallback?: boolean
  loading?: boolean
  /** Rótulo humano da categoria (ex.: "Elétrica"), para o título e o link. */
  categoryLabel?: string
  categorySlug?: string
}

/**
 * Bloco de produtos relacionados — com o título dizendo a VERDADE sobre a lista.
 *
 * O catalog-service devolve `meta.strategy` + `meta.fallback` e um `reason` por
 * item (`RecommendationHandler.Related`). A lista pode ser co-compra real,
 * complemento curado, mistura, ou simples preenchimento por categoria.
 *
 * O preenchimento por categoria é `mesma categoria ORDER BY rating`: TODO
 * produto de uma categoria ampla devolve os mesmos itens. Rotular isso como
 * recomendação é promessa que o dado não cumpre — o cliente que abre cinco
 * cabos vê a mesma vitrine cinco vezes e aprende a ignorar o bloco.
 *
 * Regra desta UI:
 * - `copurchase` / `mixed` → "Quem comprou este levou também";
 * - `complement`           → "Costuma ir junto";
 * - `category_fallback`, ou `meta.fallback = true` em qualquer estratégia
 *                          → "Outros produtos de {categoria}", com ressalva
 *                            visível e link para a categoria inteira.
 *
 * `fallback` derruba o título otimista de propósito: meia lista completada por
 * categoria, anunciada como recomendação, é promessa quebrada para metade dos
 * itens.
 */
export function RelatedProducts({
  products,
  strategy,
  fallback,
  loading = false,
  categoryLabel,
  categorySlug,
}: RelatedProductsProps) {
  const { t } = useTranslation(['catalog', 'common'])

  if (!loading && products.length === 0) return null

  // Só promete recomendação quando a lista INTEIRA é recomendação. Se o
  // backend teve que completar com categoria (`fallback`), o bloco volta a se
  // chamar pelo nome modesto — meia recomendação rotulada como recomendação
  // ainda é promessa quebrada para metade dos itens.
  const isFallback = fallback ?? strategy === 'category_fallback'
  const boughtTogether = !isFallback && (strategy === 'copurchase' || strategy === 'mixed')
  const complement = !isFallback && strategy === 'complement'

  const title = boughtTogether
    ? t('catalog:product.relatedBoughtTogether')
    : complement
      ? t('catalog:product.relatedComplement')
      : categoryLabel
      ? t('catalog:product.relatedSameCategory', { category: categoryLabel })
      : t('catalog:product.relatedSameCategoryGeneric')

  const hint = boughtTogether
    ? t('catalog:product.relatedBoughtTogetherHint')
    : complement
      ? t('catalog:product.relatedComplementHint')
      : t('catalog:product.relatedSameCategoryHint')

  return (
    <section className="mt-12" aria-labelledby="related-heading">
      <div className="mb-1 flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="related-heading" className="font-display text-lg font-bold text-gray-900">
          {title}
        </h2>
        {isFallback && categorySlug && (
          <Link
            to={`/categoria/${categorySlug}`}
            className="inline-flex items-center gap-1 text-sm font-medium text-brand-orange hover:underline"
          >
            {t('catalog:product.relatedSeeCategory')}
            <ArrowRight className="h-3.5 w-3.5" aria-hidden />
          </Link>
        )}
      </div>

      {/*
        A ressalva fica visível, não escondida num tooltip. É ela que impede o
        bloco de ser lido como recomendação personalizada.
      */}
      <p className="mb-4 flex items-start gap-1.5 text-xs text-gray-400">
        <Info className="mt-0.5 h-3 w-3 flex-shrink-0" aria-hidden />
        {hint}
      </p>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {loading
          ? Array.from({ length: 4 }).map((_, i) => <ProductCardSkeleton key={i} />)
          : products.map((p) => (
              <div key={p.id} className="flex flex-col">
                <ProductCard product={p} className="flex-1" />
                {/*
                  Motivo por item, vindo do backend. É o que separa
                  "recomendamos" de "12 clientes levaram junto": a evidência
                  aparece junto do produto, não só no título da seção.
                */}
                {p.reason?.label && (
                  <p className="mt-1.5 px-0.5 text-[11px] leading-tight text-gray-500">
                    {p.reason.label}
                  </p>
                )}
              </div>
            ))}
      </div>
    </section>
  )
}
