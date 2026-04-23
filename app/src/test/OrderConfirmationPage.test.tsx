import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import OrderConfirmationPage from '@/pages/checkout/OrderConfirmationPage'
import { useAuthStore } from '@/store/authStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useAuthStore.setState({
    user: { id: 'u1', email: 'cliente@teste.com', name: 'Cliente', role: 'customer', token: 'tok' },
  })
})

function renderPage(paymentId = 'pay-123', method = 'pix') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter
        initialEntries={[`/pedido/${paymentId}?method=${method}`]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/pedido/:id" element={<OrderConfirmationPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('OrderConfirmationPage', () => {
  it('exibe título de pedido confirmado', () => {
    renderPage()
    expect(screen.getByText(/pedido confirmado/i)).toBeInTheDocument()
  })

  it('exibe número do pedido', () => {
    renderPage('pay-abc')
    expect(screen.getByText(/pay-abc/i)).toBeInTheDocument()
  })

  it('exibe mensagem de aguardando para pix', () => {
    renderPage('pay-1', 'pix')
    expect(screen.getByText(/aguardando confirmação do pix/i)).toBeInTheDocument()
  })

  it('exibe mensagem de aprovado para cartão', () => {
    renderPage('pay-2', 'card')
    expect(screen.getByText(/pagamento aprovado/i)).toBeInTheDocument()
  })

  it('exibe mensagem com data de vencimento para boleto', () => {
    renderPage('pay-3', 'boleto')
    expect(screen.getByText(/pague o boleto até/i)).toBeInTheDocument()
  })

  it('exibe e-mail de confirmação do usuário logado', () => {
    renderPage()
    expect(screen.getByText(/cliente@teste.com/i)).toBeInTheDocument()
  })

  it('exibe link para continuar comprando', () => {
    renderPage()
    expect(screen.getByRole('link', { name: /continuar comprando/i })).toBeInTheDocument()
  })

  it('exibe link para acompanhar pedido', () => {
    renderPage()
    expect(screen.getByRole('link', { name: /acompanhar pedido/i })).toBeInTheDocument()
  })
})
