import { Helmet } from 'react-helmet-async'
import { SITE_NAME, DEFAULT_DESCRIPTION, absoluteUrl } from '@/lib/seo'

export interface SeoProps {
  /** Título da página. O sufixo com o nome da marca é adicionado automaticamente. */
  title?: string
  description?: string
  /** Path relativo da rota (ex: `/categoria/ferramentas`) para canonical + og:url. */
  path?: string
  /** Imagem de preview (Open Graph / Twitter). Aceita path relativo ou URL absoluta. */
  image?: string
  /** `website` na home, `product` na página de produto. */
  type?: 'website' | 'product' | 'article'
  /** Tira a página do índice — usado em conta, checkout, carrinho e 404. */
  noIndex?: boolean
  /** Blocos de JSON-LD (use os builders de `@/lib/seo`). */
  jsonLd?: object | object[]
}

/**
 * Meta tags por rota. Depende do <HelmetProvider /> montado no App.
 * Como a Utilar é SPA (sem SSR), o Googlebot lê estas tags após executar o JS —
 * o JSON-LD é o canal mais confiável para preço/estoque. Ver docs/seo-spa.md.
 */
export function Seo({
  title,
  description = DEFAULT_DESCRIPTION,
  path,
  image,
  type = 'website',
  noIndex = false,
  jsonLd,
}: SeoProps) {
  const fullTitle = title ? `${title} | ${SITE_NAME}` : `${SITE_NAME} — Solução em Ferragem`
  const canonical = path ? absoluteUrl(path) : undefined
  const ogImage = image ? absoluteUrl(image) : absoluteUrl('/favicon.svg')
  const blocks = jsonLd ? (Array.isArray(jsonLd) ? jsonLd : [jsonLd]) : []

  return (
    <Helmet>
      <title>{fullTitle}</title>
      <meta name="description" content={description} />
      {canonical && <link rel="canonical" href={canonical} />}
      {noIndex && <meta name="robots" content="noindex, nofollow" />}

      {/* Open Graph */}
      <meta property="og:site_name" content={SITE_NAME} />
      <meta property="og:locale" content="pt_BR" />
      <meta property="og:type" content={type} />
      <meta property="og:title" content={fullTitle} />
      <meta property="og:description" content={description} />
      <meta property="og:image" content={ogImage} />
      {canonical && <meta property="og:url" content={canonical} />}

      {/* Twitter */}
      <meta name="twitter:card" content="summary_large_image" />
      <meta name="twitter:title" content={fullTitle} />
      <meta name="twitter:description" content={description} />
      <meta name="twitter:image" content={ogImage} />

      {blocks.map((block, i) => (
        <script key={i} type="application/ld+json">
          {JSON.stringify(block)}
        </script>
      ))}
    </Helmet>
  )
}
