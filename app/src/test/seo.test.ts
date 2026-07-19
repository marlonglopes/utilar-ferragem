import { describe, it, expect } from 'vitest'
import {
  productSchema,
  breadcrumbListSchema,
  organizationSchema,
  webSiteSchema,
  absoluteUrl,
  SITE_URL,
} from '@/lib/seo'

describe('absoluteUrl', () => {
  it('prefixa paths relativos com a origem do site', () => {
    expect(absoluteUrl('/produto/furadeira')).toBe(`${SITE_URL}/produto/furadeira`)
  })

  it('aceita path sem barra inicial', () => {
    expect(absoluteUrl('sobre')).toBe(`${SITE_URL}/sobre`)
  })

  it('deixa URLs absolutas intactas', () => {
    expect(absoluteUrl('https://cdn.exemplo.com/foto.jpg')).toBe('https://cdn.exemplo.com/foto.jpg')
  })
})

describe('productSchema', () => {
  const base = {
    name: 'Furadeira de Impacto 650W',
    slug: 'furadeira-impacto-650w',
    price: 289.9,
    stock: 12,
    seller: 'Ferragem do Zé',
  }

  it('gera Product + Offer com preço e moeda BRL', () => {
    const schema = productSchema(base) as unknown as {
      '@type': string
      offers: Record<string, string>
    }

    expect(schema['@type']).toBe('Product')
    expect(schema.offers['@type']).toBe('Offer')
    expect(schema.offers.price).toBe('289.90')
    expect(schema.offers.priceCurrency).toBe('BRL')
  })

  it('deriva availability InStock quando há estoque', () => {
    const schema = productSchema(base) as unknown as { offers: { availability: string } }
    expect(schema.offers.availability).toBe('https://schema.org/InStock')
  })

  it('deriva availability OutOfStock quando o estoque é zero', () => {
    const schema = productSchema({ ...base, stock: 0 }) as unknown as {
      offers: { availability: string }
    }
    expect(schema.offers.availability).toBe('https://schema.org/OutOfStock')
  })

  it('omite aggregateRating quando não há avaliações', () => {
    // Rating sem review é erro no Search Console.
    expect(productSchema(base)).not.toHaveProperty('aggregateRating')
  })

  it('inclui aggregateRating quando há nota e contagem', () => {
    const schema = productSchema({ ...base, rating: 4.6, reviewCount: 37 }) as unknown as {
      aggregateRating: { ratingValue: string; reviewCount: number }
    }
    expect(schema.aggregateRating.ratingValue).toBe('4.6')
    expect(schema.aggregateRating.reviewCount).toBe(37)
  })

  it('transforma imagens relativas em URLs absolutas', () => {
    const schema = productSchema({ ...base, images: ['/img/a.jpg'] }) as unknown as {
      image: string[]
    }
    expect(schema.image).toEqual([`${SITE_URL}/img/a.jpg`])
  })
})

describe('breadcrumbListSchema', () => {
  it('numera as posições a partir de 1 e omite o item do último nível', () => {
    const schema = breadcrumbListSchema([
      { name: 'Início', path: '/' },
      { name: 'Ferramentas', path: '/categoria/ferramentas' },
      { name: 'Furadeira' },
    ]) as unknown as {
      itemListElement: { position: number; name: string; item?: string }[]
    }

    expect(schema.itemListElement).toHaveLength(3)
    expect(schema.itemListElement[0].position).toBe(1)
    expect(schema.itemListElement[1].item).toBe(`${SITE_URL}/categoria/ferramentas`)
    // O item corrente não recebe href — é a página atual.
    expect(schema.itemListElement[2].item).toBeUndefined()
  })
})

describe('organization e website', () => {
  it('Organization aponta para a origem canônica', () => {
    const schema = organizationSchema() as unknown as { '@type': string; url: string }
    expect(schema['@type']).toBe('Organization')
    expect(schema.url).toBe(SITE_URL)
  })

  it('WebSite expõe SearchAction apontando para /busca', () => {
    const schema = webSiteSchema() as unknown as {
      potentialAction: { target: { urlTemplate: string }; 'query-input': string }
    }
    expect(schema.potentialAction.target.urlTemplate).toContain('/busca?q={search_term_string}')
    expect(schema.potentialAction['query-input']).toBe('required name=search_term_string')
  })
})
