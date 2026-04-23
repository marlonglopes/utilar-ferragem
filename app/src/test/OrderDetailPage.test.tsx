import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import OrderDetailPage from '@/pages/orders/OrderDetailPage'
import { useAuthStore } from '@/store/authStore'
import { MOCK_ORDERS } from '@/lib/mockOrders'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useAuthStore.setState({
    user: { id: 'u1', email: 'test@test.com', name: 'Test', role: 'customer', token: 'tok' },
  })
})

const DELIVERED = MOCK_ORDERS.find((o) => o.status === 'delivered')!
const PENDING = MOCK_ORDERS.find((o) => o.status === 'pending_payment')!
const SHIPPED = MOCK_ORDERS.find((o) => o.status === 'shipped')!

function renderPage(orderId: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter
        initialEntries={[`/conta/pedidos/${orderId}`]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/conta/pedidos/:id" element={<OrderDetailPage />} />
          <Route path="/carrinho" element={<div>Carrinho</div>} />
          <Route path="/conta" element={<div>Conta</div>} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>
  )
}

async function waitForLoad() {
  await waitFor(
    () => expect(screen.queryByText('Carregando...')).not.toBeInTheDocument(),
    { timeout: 3000 }
  )
}

describe('OrderDetailPage — pedido entregue', () => {
  it('exibe número do pedido', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => screen.getByText(new RegExp(DELIVERED.number)), { timeout: 3000 })
    expect(screen.getByText(new RegExp(DELIVERED.number))).toBeInTheDocument()
  }, 8000)

  it('exibe status Entregue', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => expect(screen.getAllByText('Entregue').length).toBeGreaterThan(0), { timeout: 3000 })
  }, 8000)

  it('exibe itens do pedido', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => screen.getByText(/Furadeira Bosch/i), { timeout: 3000 })
    expect(screen.getByText(/Furadeira Bosch/i)).toBeInTheDocument()
    expect(screen.getByText(/Jogo de Brocas/i)).toBeInTheDocument()
  }, 8000)

  it('exibe endereço de entrega', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => screen.getByText(/Endereço de entrega/i), { timeout: 3000 })
    expect(screen.getByText(/Av. Paulista/i)).toBeInTheDocument()
  }, 8000)

  it('exibe código de rastreamento', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => screen.getByText(DELIVERED.trackingCode!), { timeout: 3000 })
    expect(screen.getByText(DELIVERED.trackingCode!)).toBeInTheDocument()
  }, 8000)

  it('exibe botão "Comprar novamente"', async () => {
    renderPage(DELIVERED.id)
    await waitFor(() => screen.getByRole('button', { name: /comprar novamente/i }), { timeout: 3000 })
  }, 8000)

  it('não exibe botão de cancelar para pedido entregue', async () => {
    renderPage(DELIVERED.id)
    await waitForLoad()
    await waitFor(() => screen.getByText(/Furadeira Bosch/i), { timeout: 3000 })
    expect(screen.queryByRole('button', { name: /cancelar pedido/i })).not.toBeInTheDocument()
  }, 8000)
})

describe('OrderDetailPage — pedido aguardando pagamento', () => {
  it('exibe botão cancelar', async () => {
    renderPage(PENDING.id)
    await waitFor(() => screen.getByRole('button', { name: /cancelar pedido/i }), { timeout: 3000 })
  }, 8000)

  it('abre modal de confirmação ao clicar em cancelar', async () => {
    renderPage(PENDING.id)
    await waitFor(() => screen.getByRole('button', { name: /cancelar pedido/i }), { timeout: 3000 })
    fireEvent.click(screen.getByRole('button', { name: /cancelar pedido/i }))
    expect(screen.getByText(/esta ação não pode ser desfeita/i)).toBeInTheDocument()
  }, 8000)

  it('fecha modal ao clicar em voltar', async () => {
    renderPage(PENDING.id)
    await waitFor(() => screen.getByRole('button', { name: /cancelar pedido/i }), { timeout: 3000 })
    fireEvent.click(screen.getByRole('button', { name: /cancelar pedido/i }))
    fireEvent.click(screen.getByRole('button', { name: /voltar/i }))
    await waitFor(() => {
      expect(screen.queryByText(/esta ação não pode ser desfeita/i)).not.toBeInTheDocument()
    })
  }, 8000)
})

describe('OrderDetailPage — pedido com rastreamento', () => {
  it('exibe código de rastreamento para enviado', async () => {
    renderPage(SHIPPED.id)
    await waitFor(() => screen.getByText(SHIPPED.trackingCode!), { timeout: 3000 })
  }, 8000)
})

describe('OrderDetailPage — pedido não encontrado', () => {
  it('exibe mensagem de não encontrado', async () => {
    renderPage('ordem-inexistente')
    await waitFor(
      () => expect(screen.queryByText(/não encontrado/i)).toBeInTheDocument(),
      { timeout: 3000 }
    )
  }, 8000)
})
