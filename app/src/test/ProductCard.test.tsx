import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { ProductCard } from '@/components/catalog/ProductCard'
import type { Product } from '@/types/product'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function makeProduct(overrides: Partial<Product> = {}): Product {
  return {
    id: 'p1',
    name: 'Furadeira Bosch GSB 13 RE',
    slug: 'furadeira-bosch-gsb-13-re',
    price: 299.9,
    currency: 'BRL',
    icon: 'drill',
    seller: 'Bosch Store',
    stock: 20,
    rating: 4.5,
    reviewCount: 128,
    ...overrides,
  } as Product
}

function renderCard(p: Product) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <ProductCard product={p} />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('ProductCard', () => {
  it('mostra nome e link para o detalhe do produto', () => {
    renderCard(makeProduct())
    expect(screen.getByText('Furadeira Bosch GSB 13 RE')).toBeInTheDocument()
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/produto/furadeira-bosch-gsb-13-re')
  })

  it('formata o preço em BRL', () => {
    renderCard(makeProduct({ price: 1299.9 }))
    expect(screen.getByText(/R\$\s?1\.299,90/)).toBeInTheDocument()
  })

  it('exibe o badge quando badge + badgeLabel estão presentes', () => {
    renderCard(makeProduct({ badge: 'discount', badgeLabel: '-20%' }))
    expect(screen.getByText('-20%')).toBeInTheDocument()
  })
})
