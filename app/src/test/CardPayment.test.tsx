import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll, vi } from 'vitest'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import type { PaymentResult } from '@/hooks/usePayment'

// ─── Mocks ────────────────────────────────────────────────────────────────────
// Mock @stripe/react-stripe-js antes de importar o componente.
// Isso evita carregar o iframe real do Stripe em teste e dá controle determinístico.

const confirmPaymentMock = vi.fn()

vi.mock('@stripe/react-stripe-js', () => ({
  Elements: ({ children }: { children: React.ReactNode }) => <div data-testid="elements-provider">{children}</div>,
  PaymentElement: ({ onReady }: { onReady?: () => void }) => {
    // Simula o "ready" event imediatamente
    queueMicrotask(() => onReady?.())
    return <div data-testid="payment-element">[Stripe Card iframe]</div>
  },
  useStripe: () => ({ confirmPayment: confirmPaymentMock }),
  useElements: () => ({}),
}))

vi.mock('@/lib/stripe', () => ({
  getStripe: () => Promise.resolve({} as unknown),
  isStripeConfigured: true,
}))

import CardPayment from '@/pages/checkout/CardPayment'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function renderWith(props: Partial<Parameters<typeof CardPayment>[0]> & { result: PaymentResult }) {
  const onConfirmed = props.onConfirmed ?? vi.fn()
  const onFailed = props.onFailed ?? vi.fn()
  const utils = render(
    <I18nextProvider i18n={i18n}>
      <CardPayment
        result={props.result}
        amount={props.amount ?? 99.9}
        onConfirmed={onConfirmed}
        onFailed={onFailed}
        onSimulateConfirm={props.onSimulateConfirm}
      />
    </I18nextProvider>,
  )
  return { ...utils, onConfirmed, onFailed }
}

const stripeResult: PaymentResult = {
  paymentId: 'pay-1',
  pspId: 'pi_test_123',
  provider: 'stripe',
  method: 'card',
  status: 'pending',
  clientSecret: 'pi_test_123_secret_xyz',
}

describe('CardPayment — Stripe Elements', () => {
  beforeAll(() => {
    confirmPaymentMock.mockReset()
  })

  it('renderiza PaymentElement quando provider=stripe + clientSecret presente', async () => {
    renderWith({ result: stripeResult })
    expect(await screen.findByTestId('payment-element')).toBeInTheDocument()
    expect(screen.getByText(/cartão de crédito ou débito/i)).toBeInTheDocument()
    expect(screen.getByText(/processado pela stripe/i)).toBeInTheDocument()
  })

  it('botão de pagar exibe o valor formatado', async () => {
    renderWith({ result: stripeResult, amount: 199.9 })
    await screen.findByTestId('payment-element')
    expect(screen.getByRole('button', { name: /pagar.*r\$\s*199,90/i })).toBeInTheDocument()
  })

  it('confirmPayment succeeded → chama onConfirmed', async () => {
    confirmPaymentMock.mockResolvedValueOnce({
      paymentIntent: { status: 'succeeded' },
      error: undefined,
    })

    const { onConfirmed } = renderWith({ result: stripeResult })

    await screen.findByTestId('payment-element')
    fireEvent.click(screen.getByRole('button', { name: /pagar/i }))

    await waitFor(() => expect(onConfirmed).toHaveBeenCalledTimes(1))
  })

  it('confirmPayment com erro → chama onFailed e exibe mensagem', async () => {
    confirmPaymentMock.mockResolvedValueOnce({
      paymentIntent: undefined,
      error: { message: 'Cartão recusado pelo emissor' },
    })

    const { onFailed } = renderWith({ result: stripeResult })

    await screen.findByTestId('payment-element')
    fireEvent.click(screen.getByRole('button', { name: /pagar/i }))

    await waitFor(() => {
      expect(onFailed).toHaveBeenCalledWith('Cartão recusado pelo emissor')
      expect(screen.getByText(/cartão recusado pelo emissor/i)).toBeInTheDocument()
    })
  })

  it('status=confirmed → exibe checkmark de pagamento confirmado', () => {
    const confirmed: PaymentResult = { ...stripeResult, status: 'confirmed' }
    renderWith({ result: confirmed })
    expect(screen.getByText(/pagamento confirmado/i)).toBeInTheDocument()
    expect(screen.queryByTestId('payment-element')).not.toBeInTheDocument()
  })
})

describe('CardPayment — Mercado Pago redirect (fallback)', () => {
  it('exibe link "Ir para pagamento seguro" quando provider=mercadopago', () => {
    const mp: PaymentResult = {
      paymentId: 'pay-mp',
      provider: 'mercadopago',
      method: 'card',
      status: 'pending',
      initPoint: 'https://mp.com/checkout/abc',
    }
    renderWith({ result: mp })
    const link = screen.getByRole('link', { name: /ir para pagamento seguro/i })
    expect(link).toHaveAttribute('href', 'https://mp.com/checkout/abc')
  })

  it('botão fica disabled quando initPoint="#"', () => {
    const mp: PaymentResult = {
      paymentId: 'pay-mp',
      provider: 'mercadopago',
      method: 'card',
      status: 'pending',
      initPoint: '#',
    }
    renderWith({ result: mp })
    expect(screen.getByRole('button', { name: /ir para pagamento seguro/i })).toBeDisabled()
  })
})
