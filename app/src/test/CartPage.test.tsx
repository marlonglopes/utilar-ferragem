import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import CartPage from '@/pages/cart/CartPage'
import { useCartStore } from '@/store/cartStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useCartStore.setState({ items: [] })
})

function renderCartPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <CartPage />
      </MemoryRouter>
    </I18nextProvider>
  )
}

function addItemToCart(overrides = {}) {
  useCartStore.getState().addItem({
    productId: 'prod-1',
    sellerId: 'seller-1',
    sellerName: 'Ferragens SP',
    name: 'Furadeira Bosch GSB',
    icon: '🔧',
    priceSnapshot: 299.9,
    quantity: 1,
    stock: 10,
    ...overrides,
  })
}

describe('CartPage — estado vazio', () => {
  it('exibe mensagem de carrinho vazio', () => {
    renderCartPage()
    expect(screen.getByText(/carrinho está vazio/i)).toBeInTheDocument()
  })

  it('exibe link para explorar catálogo', () => {
    renderCartPage()
    expect(screen.getByRole('link', { name: /explorar catálogo/i })).toBeInTheDocument()
  })
})

describe('CartPage — com itens', () => {
  it('exibe o nome do produto', () => {
    addItemToCart()
    renderCartPage()
    expect(screen.getByText(/furadeira bosch gsb/i)).toBeInTheDocument()
  })

  it('exibe o vendedor', () => {
    addItemToCart()
    renderCartPage()
    expect(screen.getByText(/ferragens sp/i)).toBeInTheDocument()
  })

  it('exibe o preço formatado', () => {
    addItemToCart()
    renderCartPage()
    expect(screen.getAllByText(/299/).length).toBeGreaterThan(0)
  })

  it('remove item ao clicar em remover', () => {
    addItemToCart()
    renderCartPage()
    fireEvent.click(screen.getByRole('button', { name: /remover item/i }))
    expect(useCartStore.getState().items).toHaveLength(0)
  })

  it('agrupa itens de vendedores diferentes', () => {
    addItemToCart({ productId: 'a', sellerName: 'Loja A', sellerId: 's1' })
    addItemToCart({ productId: 'b', sellerName: 'Loja B', sellerId: 's2' })
    renderCartPage()
    expect(screen.getByText(/loja a/i)).toBeInTheDocument()
    expect(screen.getByText(/loja b/i)).toBeInTheDocument()
  })

  it('exibe link para finalizar compra', () => {
    addItemToCart()
    renderCartPage()
    expect(screen.getByRole('link', { name: /finalizar compra/i })).toBeInTheDocument()
  })
})
