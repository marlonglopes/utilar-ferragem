import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import OrdersTab from '@/pages/account/OrdersTab'
import { useAuthStore } from '@/store/authStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useAuthStore.setState({
    user: { id: 'u1', email: 'test@test.com', name: 'Test', role: 'customer', token: 'tok' },
  })
})

function renderTab() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <OrdersTab />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('OrdersTab', () => {
  it('exibe spinner enquanto carrega', () => {
    renderTab()
    // Spinner present immediately before the 300ms mock delay
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('exibe lista de pedidos após carregar', async () => {
    renderTab()
    await waitFor(
      () => expect(screen.getAllByText(/Pedido #/i).length).toBeGreaterThan(0),
      { timeout: 3000 }
    )
  }, 8000)

  it('exibe todos os pedidos no filtro "Todos"', async () => {
    renderTab()
    await waitFor(() => screen.getAllByText(/Pedido #/i), { timeout: 3000 })
    expect(screen.getAllByText(/Pedido #/i).length).toBe(4)
  }, 8000)

  it('filtra apenas pedidos em andamento', async () => {
    renderTab()
    await waitFor(() => screen.getAllByText(/Pedido #/i), { timeout: 3000 })

    fireEvent.click(screen.getByRole('button', { name: /em andamento/i }))

    // Active = pending_payment, paid, picking, shipped (3 in mock data)
    await waitFor(() => {
      expect(screen.getAllByText(/Pedido #/i).length).toBe(3)
    })
  }, 8000)

  it('filtra apenas pedidos concluídos', async () => {
    renderTab()
    await waitFor(() => screen.getAllByText(/Pedido #/i), { timeout: 3000 })

    fireEvent.click(screen.getByRole('button', { name: /concluídos/i }))

    // Done = delivered, cancelled (1 in mock data)
    await waitFor(() => {
      expect(screen.getAllByText(/Pedido #/i).length).toBe(1)
    })
  }, 8000)

  it('cada pedido tem link para o detalhe', async () => {
    renderTab()
    await waitFor(() => screen.getAllByText(/Pedido #/i), { timeout: 3000 })

    const links = screen.getAllByRole('link')
    expect(links.length).toBeGreaterThanOrEqual(4)
    expect(links[0].getAttribute('href')).toMatch(/\/conta\/pedidos\//)
  }, 8000)

  it('exibe status de cada pedido', async () => {
    renderTab()
    await waitFor(() => screen.getAllByText(/Pedido #/i), { timeout: 3000 })

    expect(screen.getByText('Entregue')).toBeInTheDocument()
    expect(screen.getByText('Enviado')).toBeInTheDocument()
    expect(screen.getByText('Pago')).toBeInTheDocument()
    expect(screen.getByText('Aguardando pagamento')).toBeInTheDocument()
  }, 8000)
})
