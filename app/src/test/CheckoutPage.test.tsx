import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import CheckoutPage from '@/pages/checkout/CheckoutPage'
import { useCartStore } from '@/store/cartStore'
import { useAuthStore } from '@/store/authStore'
import { useAddressStore } from '@/store/addressStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useCartStore.setState({ items: [] })
  useAddressStore.setState({ addresses: [] })
  useAuthStore.setState({
    user: { id: 'u1', email: 'test@test.com', name: 'Test', role: 'customer', token: 'tok' },
  })
})

function addItem() {
  useCartStore.getState().addItem({
    productId: 'p1',
    sellerId: 's1',
    sellerName: 'Loja Teste',
    name: 'Martelo Stanley',
    icon: '🔨',
    priceSnapshot: 89.9,
    quantity: 1,
    stock: 5,
  })
}

function renderCheckout() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <CheckoutPage />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('CheckoutPage — passo endereço', () => {
  it('exibe o stepper com 3 passos', () => {
    addItem()
    renderCheckout()
    expect(screen.getAllByText(/endereço/i).length).toBeGreaterThan(0)
    expect(screen.getByText('Entrega')).toBeInTheDocument()
    expect(screen.getByText('Pagamento')).toBeInTheDocument()
  })

  it('exibe o formulário de endereço', () => {
    addItem()
    renderCheckout()
    expect(screen.getByText(/endereço de entrega/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/CEP/i)).toBeInTheDocument()
  })

  it('exibe o resumo do pedido com o produto', () => {
    addItem()
    renderCheckout()
    expect(screen.getByText(/martelo stanley/i)).toBeInTheDocument()
  })

  it('avança para entrega ao preencher endereço', async () => {
    addItem()
    renderCheckout()

    fireEvent.change(screen.getByLabelText(/CEP/i), { target: { value: '01310-100' } })
    fireEvent.change(screen.getByLabelText(/Rua/i), { target: { value: 'Av Paulista' } })
    fireEvent.change(screen.getByLabelText(/Número/i), { target: { value: '100' } })
    fireEvent.change(screen.getByLabelText(/Bairro/i), { target: { value: 'Bela Vista' } })
    fireEvent.change(screen.getByLabelText(/Cidade/i), { target: { value: 'São Paulo' } })
    fireEvent.change(screen.getByLabelText(/Estado/i), { target: { value: 'SP' } })

    fireEvent.click(screen.getByRole('button', { name: /continuar/i }))

    await waitFor(() => {
      expect(screen.getByText(/opções de entrega/i)).toBeInTheDocument()
    })
  })
})

describe('CheckoutPage — endereço salvo (usuário logado)', () => {
  function seedDefaultAddress() {
    useAddressStore.getState().addAddress({
      label: 'Casa',
      cep: '01310-100',
      street: 'Av Paulista',
      number: '100',
      complement: '',
      neighborhood: 'Bela Vista',
      city: 'São Paulo',
      state: 'SP',
    })
  }

  it('exibe lista de endereços salvos em vez do formulário', () => {
    addItem()
    seedDefaultAddress()
    renderCheckout()
    expect(screen.getByText(/av paulista/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/CEP/i)).not.toBeInTheDocument()
  })

  it('endereço padrão vem pré-selecionado e avança para entrega', async () => {
    addItem()
    seedDefaultAddress()
    renderCheckout()
    fireEvent.click(screen.getByRole('button', { name: /continuar/i }))
    await waitFor(() => {
      expect(screen.getByText(/opções de entrega/i)).toBeInTheDocument()
    })
  })

  it('permite abrir o formulário de novo endereço via "usar outro endereço"', () => {
    addItem()
    seedDefaultAddress()
    renderCheckout()
    fireEvent.click(screen.getByRole('button', { name: /usar outro endereço/i }))
    expect(screen.getByLabelText(/CEP/i)).toBeInTheDocument()
  })
})

describe('CheckoutPage — passo entrega', () => {
  async function goToShipping() {
    addItem()
    renderCheckout()
    fireEvent.change(screen.getByLabelText(/CEP/i), { target: { value: '01310-100' } })
    fireEvent.change(screen.getByLabelText(/Rua/i), { target: { value: 'Av Paulista' } })
    fireEvent.change(screen.getByLabelText(/Número/i), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText(/Bairro/i), { target: { value: 'Bela Vista' } })
    fireEvent.change(screen.getByLabelText(/Cidade/i), { target: { value: 'São Paulo' } })
    fireEvent.change(screen.getByLabelText(/Estado/i), { target: { value: 'SP' } })
    fireEvent.click(screen.getByRole('button', { name: /continuar/i }))
    await waitFor(() => screen.getByText(/opções de entrega/i))
  }

  it('exibe as opções PAC e SEDEX', async () => {
    await goToShipping()
    expect(screen.getByText(/PAC — Correios/i)).toBeInTheDocument()
    expect(screen.getByText(/SEDEX — Correios/i)).toBeInTheDocument()
  })

  it('avança para pagamento ao selecionar frete', async () => {
    await goToShipping()
    fireEvent.click(screen.getByRole('button', { name: /continuar para pagamento/i }))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /confirmar e pagar/i })).toBeInTheDocument()
    })
  })
})

describe('CheckoutPage — passo pagamento', () => {
  async function goToPayment() {
    addItem()
    renderCheckout()
    fireEvent.change(screen.getByLabelText(/CEP/i), { target: { value: '01310-100' } })
    fireEvent.change(screen.getByLabelText(/Rua/i), { target: { value: 'Av Paulista' } })
    fireEvent.change(screen.getByLabelText(/Número/i), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText(/Bairro/i), { target: { value: 'Bela Vista' } })
    fireEvent.change(screen.getByLabelText(/Cidade/i), { target: { value: 'São Paulo' } })
    fireEvent.change(screen.getByLabelText(/Estado/i), { target: { value: 'SP' } })
    fireEvent.click(screen.getByRole('button', { name: /continuar/i }))
    await waitFor(() => screen.getByText(/opções de entrega/i))
    fireEvent.click(screen.getByRole('button', { name: /continuar para pagamento/i }))
    await waitFor(() => screen.getByRole('button', { name: /confirmar e pagar/i }))
  }

  it('exibe seletor de método pix/boleto/cartão', async () => {
    await goToPayment()
    expect(screen.getAllByText(/pix/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/boleto bancário/i)).toBeInTheDocument()
    expect(screen.getByText(/cartão de crédito/i)).toBeInTheDocument()
  })

  it('exibe desconto pix quando selecionado', async () => {
    await goToPayment()
    expect(screen.getByText(/desconto no pix/i)).toBeInTheDocument()
  })

  it('exibe QR code após confirmar pix', async () => {
    await goToPayment()

    fireEvent.click(screen.getByRole('button', { name: /confirmar e pagar/i }))

    // 600ms simulated latency — wait for PixPayment to appear
    await waitFor(
      () => expect(screen.getByText(/pague com pix/i)).toBeInTheDocument(),
      { timeout: 3000 }
    )
  }, 10000)
})
