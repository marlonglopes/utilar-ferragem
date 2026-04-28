import { render, screen, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from 'vitest'
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

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
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

// Re-exibição persistida do boleto (gap UX corrigido):
// Quando user volta na página `/pedido/<id>?method=boleto` depois de fechar
// a aba, OrderConfirmation faz GET /api/v1/payments/:id e renderiza
// BoletoPayment inline com a linha digitável + voucher URL.
//
// Estratégia: como `isApiEnabled` em api.ts é resolvido em import-time a
// partir de `import.meta.env.VITE_API_URL`, usamos vi.stubEnv ANTES do
// dynamic import do componente (mesmo padrão de usePayment.stripe.test.ts).
describe('OrderConfirmationPage — recuperação de boleto', () => {
  const BOLETO_DETAIL = {
    id: 'pay-bol-1',
    method: 'boleto',
    status: 'pending',
    psp_payment_id: 'pi_boleto_xyz',
    provider: 'stripe',
    psp_payload: {
      type: 'boleto_display_details',
      next_action: {
        boleto_display_details: {
          number: '34191.09008 09133.610947 91020.150008 1 00010000012345',
          pdf: 'https://stripe.com/voucher.pdf',
          hosted_voucher_url: 'https://payments.stripe.com/voucher/abc',
          expires_at: Math.floor(Date.now() / 1000) + 3 * 86400,
        },
      },
    },
  }

  async function renderWithDynamicImport(paymentId: string, method: string) {
    // Reset cached modules pra que api.ts re-leia VITE_API_URL com o stub atual.
    vi.resetModules()
    const Page = (await import('@/pages/checkout/OrderConfirmationPage')).default
    return render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter
          initialEntries={[`/pedido/${paymentId}?method=${method}`]}
          future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        >
          <Routes>
            <Route path="/pedido/:id" element={<Page />} />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )
  }

  it('faz fetch do payment e renderiza linha digitável + voucher URL', async () => {
    vi.stubEnv('VITE_API_URL', 'http://localhost:8090')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => BOLETO_DETAIL,
    } as Response))

    await renderWithDynamicImport('pay-bol-1', 'boleto')

    // Linha digitável aparece — prova direta que BoletoPayment foi montado
    // com os dados retornados pelo GET /payments/:id.
    await waitFor(() => {
      expect(
        screen.getByDisplayValue(BOLETO_DETAIL.psp_payload.next_action.boleto_display_details.number),
      ).toBeInTheDocument()
    })

    // Link "Visualizar boleto" (hosted voucher Stripe) — botão público de impressão.
    const voucherLink = screen.getByRole('link', { name: /visualizar boleto/i })
    expect(voucherLink).toHaveAttribute(
      'href',
      BOLETO_DETAIL.psp_payload.next_action.boleto_display_details.hosted_voucher_url,
    )
  })

  it('não tenta fetch quando API não está habilitada (modo mock)', async () => {
    // Sem stubEnv VITE_API_URL → isApiEnabled=false → fetch não chamado.
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await renderWithDynamicImport('pay-bol-2', 'boleto')

    // Mensagem default ainda aparece, mas sem fetch.
    expect(screen.getByText(/pague o boleto até/i)).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
