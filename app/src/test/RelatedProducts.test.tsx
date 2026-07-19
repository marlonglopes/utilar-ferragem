import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { RelatedProducts } from '@/components/catalog/RelatedProducts'
import { MOCK_PRODUCTS } from '@/lib/mockProducts'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import type { RelatedStrategy } from '@/hooks/useRelatedProducts'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

const PRODUCTS = MOCK_PRODUCTS.slice(0, 4)

function renderBlock(strategy: RelatedStrategy, extra: Record<string, unknown> = {}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <RelatedProducts
          products={PRODUCTS}
          strategy={strategy}
          categoryLabel="Elétrica"
          categorySlug="eletrica"
          {...extra}
        />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('RelatedProducts — honestidade do título', () => {
  /**
   * REGRESSÃO: chamar "mesma categoria ORDER BY rating" de recomendação.
   *
   * O backend (`ProductHandler.Related`) devolve os 4 produtos mais bem
   * avaliados da categoria. Isso significa que TODO produto de uma categoria
   * ampla devolve exatamente a mesma lista — abrir 5 cabos diferentes mostra a
   * mesma vitrine 5 vezes. Rotular isso como "quem comprou levou também" é
   * promessa que o dado não cumpre, e o cliente aprende a ignorar o bloco.
   */
  it('rotula a lista de categoria pelo que ela é', () => {
    renderBlock('category_fallback')

    expect(screen.getByRole('heading', { name: /outros produtos de elétrica/i })).toBeInTheDocument()
    expect(screen.queryByText(/quem comprou/i)).not.toBeInTheDocument()
  })

  it('explica em texto visível que não é recomendação', () => {
    renderBlock('category_fallback')
    expect(screen.getByText(/não é uma recomendação/i)).toBeInTheDocument()
  })

  it('oferece a categoria inteira, que é o que o cliente quer nesse momento', () => {
    renderBlock('category_fallback')
    expect(screen.getByRole('link', { name: /ver a categoria inteira/i })).toHaveAttribute(
      'href',
      '/categoria/eletrica'
    )
  })

  it('sem rótulo de categoria, cai num título genérico — nunca em recomendação', () => {
    renderBlock('category_fallback', { categoryLabel: undefined })
    expect(
      screen.getByRole('heading', { name: /outros produtos desta categoria/i })
    ).toBeInTheDocument()
  })

  it('só promete co-compra quando o backend declara a origem', () => {
    renderBlock('copurchase')

    expect(screen.getByRole('heading', { name: /quem comprou este levou também/i })).toBeInTheDocument()
    expect(screen.getByText(/pedidos reais de outros clientes/i)).toBeInTheDocument()
    // Aí não faz sentido empurrar a categoria: a lista já é específica.
    expect(screen.queryByRole('link', { name: /ver a categoria inteira/i })).not.toBeInTheDocument()
  })
})

describe('RelatedProducts — estados', () => {
  it('não renderiza nada quando não há produtos', () => {
    const { container } = render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <RelatedProducts products={[]} strategy="category_fallback" />
        </MemoryRouter>
      </I18nextProvider>
    )
    expect(container.firstChild).toBeNull()
  })

  it('mostra esqueletos enquanto carrega', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <RelatedProducts products={[]} strategy="category_fallback" loading />
        </MemoryRouter>
      </I18nextProvider>
    )
    // Carregando ≠ vazio: some com o bloco só depois de saber que não há nada.
    expect(screen.getByRole('heading')).toBeInTheDocument()
  })

  it('a seção é anunciada pelo próprio título', () => {
    renderBlock('category_fallback')
    expect(screen.getByRole('region', { name: /outros produtos de elétrica/i })).toBeInTheDocument()
  })
})

describe('taxonomia — rótulo de categoria', () => {
  /**
   * REGRESSÃO: chave de i18n crua exibida no lugar do nome da categoria.
   *
   * `TOP_LEVEL_CATEGORIES[].labelKey` JÁ vem com o namespace embutido
   * ("common:taxonomy.eletrica"). A página de produto prefixava de novo
   * (`t(\`common:${labelKey}\`)`), gerando "common:common:taxonomy.eletrica",
   * que o i18next não resolve — e devolve a chave. Resultado: a migalha de pão
   * e o JSON-LD mandado ao Google diziam "common:taxonomy.eletrica" em vez de
   * "Elétrica".
   */
  it('resolve o labelKey sem prefixar o namespace duas vezes', () => {
    const eletrica = TOP_LEVEL_CATEGORIES.find((c) => c.slug === 'eletrica')!

    expect(eletrica.labelKey).toBe('common:taxonomy.eletrica')
    expect(i18n.t(eletrica.labelKey)).toBe('Elétrica')
    // O jeito antigo: dupla prefixação devolve a chave, não o rótulo.
    expect(i18n.t(`common:${eletrica.labelKey}`)).not.toBe('Elétrica')
  })

  it('toda categoria de topo tem tradução em pt-BR', () => {
    for (const c of TOP_LEVEL_CATEGORIES) {
      const label = i18n.t(c.labelKey)
      expect(label).not.toContain('taxonomy.')
      expect(label.length).toBeGreaterThan(0)
    }
  })
})

describe('RelatedProducts — estratégias do backend', () => {
  it('"mixed" também conta como co-compra', () => {
    renderBlock('mixed')
    expect(screen.getByRole('heading', { name: /quem comprou este levou também/i })).toBeInTheDocument()
  })

  it('"complement" tem título próprio, sem prometer co-compra', () => {
    renderBlock('complement')
    expect(screen.getByRole('heading', { name: /costuma ir junto/i })).toBeInTheDocument()
    expect(screen.queryByText(/quem comprou/i)).not.toBeInTheDocument()
  })

  /**
   * REGRESSÃO: anunciar recomendação para uma lista meio completada por categoria.
   *
   * O backend devolve `meta.fallback = true` quando teve que preencher a lista
   * com itens da categoria por falta de co-compra suficiente. Se a UI olhasse
   * só `strategy`, um `copurchase` parcialmente preenchido apareceria como
   * "quem comprou levou também" — promessa quebrada para metade dos itens.
   */
  it('fallback do backend derruba o título otimista', () => {
    renderBlock('copurchase', { fallback: true })

    expect(screen.queryByText(/quem comprou/i)).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /outros produtos de elétrica/i })).toBeInTheDocument()
    expect(screen.getByText(/não é uma recomendação/i)).toBeInTheDocument()
  })

  it('estratégia desconhecida cai no rótulo modesto, nunca no otimista', () => {
    // Backend mais novo, front mais velho: errar para o lado humilde.
    renderBlock('coisa_nova_do_futuro' as RelatedStrategy)
    expect(screen.getByRole('heading', { name: /outros produtos de elétrica/i })).toBeInTheDocument()
  })

  it('mostra a evidência por item quando o backend manda o motivo', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <RelatedProducts
            strategy="copurchase"
            products={[
              { ...PRODUCTS[0], reason: { kind: 'copurchase', label: '12 clientes levaram junto', orders: 12 } },
            ]}
          />
        </MemoryRouter>
      </I18nextProvider>
    )

    // "12 clientes levaram junto" é o que separa recomendação de palpite.
    expect(screen.getByText('12 clientes levaram junto')).toBeInTheDocument()
  })
})
