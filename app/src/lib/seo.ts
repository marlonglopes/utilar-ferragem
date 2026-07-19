// Constantes e helpers de SEO (meta tags + JSON-LD structured data).
//
// SITE_URL é a origem canônica usada em <link rel="canonical">, og:url e nos
// @id do JSON-LD. Em produção defina VITE_SITE_URL; o fallback abaixo é um
// PLACEHOLDER e precisa ser confirmado antes do go-live.
export const SITE_URL = (
  import.meta.env.VITE_SITE_URL || 'https://www.utilarferragem.com.br'
).replace(/\/$/, '')

export const SITE_NAME = 'UtiLar Ferragem'
export const SITE_TAGLINE = 'Solução em Ferragem'
export const DEFAULT_DESCRIPTION =
  'Ferramentas, materiais de construção, elétrica e hidráulica com entrega rápida. ' +
  'Pague no Pix, boleto ou cartão em até 12x. Vendedores com CNPJ verificado e nota fiscal.'

/** Monta uma URL absoluta a partir de um path relativo. */
export function absoluteUrl(path: string): string {
  if (!path) return SITE_URL
  if (/^https?:\/\//i.test(path)) return path
  return `${SITE_URL}${path.startsWith('/') ? path : `/${path}`}`
}

// ─── JSON-LD builders ─────────────────────────────────────────────────────────
// Retornam objetos schema.org puros; a serialização acontece no <JsonLd />.

export interface BreadcrumbEntry {
  name: string
  /** Path relativo (ex: `/categoria/ferramentas`). Omitido no último item. */
  path?: string
}

export function breadcrumbListSchema(items: BreadcrumbEntry[]) {
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: items.map((item, i) => ({
      '@type': 'ListItem',
      position: i + 1,
      name: item.name,
      ...(item.path ? { item: absoluteUrl(item.path) } : {}),
    })),
  }
}

export function organizationSchema() {
  return {
    '@context': 'https://schema.org',
    '@type': 'Organization',
    '@id': `${SITE_URL}/#organization`,
    name: SITE_NAME,
    url: SITE_URL,
    logo: absoluteUrl('/favicon.svg'),
    description: DEFAULT_DESCRIPTION,
    // [PREENCHER: perfis sociais reais antes do go-live]
    sameAs: [],
  }
}

export function webSiteSchema() {
  return {
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    '@id': `${SITE_URL}/#website`,
    name: SITE_NAME,
    url: SITE_URL,
    inLanguage: 'pt-BR',
    publisher: { '@id': `${SITE_URL}/#organization` },
    // Habilita a sitelinks searchbox do Google apontando pra /busca?q=
    potentialAction: {
      '@type': 'SearchAction',
      target: {
        '@type': 'EntryPoint',
        urlTemplate: `${SITE_URL}/busca?q={search_term_string}`,
      },
      'query-input': 'required name=search_term_string',
    },
  }
}

export interface ProductSchemaInput {
  name: string
  slug: string
  description?: string
  price: number
  currency?: string
  stock: number
  brand?: string
  seller?: string
  rating?: number
  reviewCount?: number
  images?: string[]
}

/**
 * Product + Offer. `availability` é derivado do estoque real — é o que faz o
 * Google mostrar preço e disponibilidade no resultado de busca.
 */
export function productSchema(p: ProductSchemaInput) {
  const url = absoluteUrl(`/produto/${p.slug}`)
  return {
    '@context': 'https://schema.org',
    '@type': 'Product',
    '@id': `${url}#product`,
    name: p.name,
    url,
    ...(p.description ? { description: p.description } : {}),
    ...(p.images?.length ? { image: p.images.map(absoluteUrl) } : {}),
    ...(p.brand ? { brand: { '@type': 'Brand', name: p.brand } } : {}),
    offers: {
      '@type': 'Offer',
      url,
      price: p.price.toFixed(2),
      priceCurrency: p.currency ?? 'BRL',
      availability:
        p.stock > 0
          ? 'https://schema.org/InStock'
          : 'https://schema.org/OutOfStock',
      itemCondition: 'https://schema.org/NewCondition',
      ...(p.seller ? { seller: { '@type': 'Organization', name: p.seller } } : {}),
    },
    // aggregateRating só é válido com pelo menos uma avaliação — sem isso o
    // Search Console acusa "rating sem reviews".
    ...(p.rating && p.reviewCount
      ? {
          aggregateRating: {
            '@type': 'AggregateRating',
            ratingValue: p.rating.toFixed(1),
            reviewCount: p.reviewCount,
            bestRating: 5,
            worstRating: 1,
          },
        }
      : {}),
  }
}
