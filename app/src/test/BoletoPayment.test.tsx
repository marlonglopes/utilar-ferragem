import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import BoletoPayment from '@/pages/checkout/BoletoPayment'
import type { PaymentResult } from '@/hooks/usePayment'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function renderWith(result: PaymentResult) {
  return render(
    <I18nextProvider i18n={i18n}>
      <BoletoPayment result={result} />
    </I18nextProvider>,
  )
}

describe('BoletoPayment', () => {
  const baseStripe: PaymentResult = {
    paymentId: 'pay-1',
    provider: 'stripe',
    method: 'boleto',
    status: 'pending',
    barCode: '34191.09008 09133.610947 91020.150008 1 00010000012345',
    pdfUrl: 'https://stripe.com/voucher.pdf',
    hostedVoucherUrl: 'https://payments.stripe.com/voucher/abc',
    boletoExpiresAt: new Date('2026-04-30'),
  }

  it('Stripe: exibe linha digitável', () => {
    renderWith(baseStripe)
    expect(screen.getByDisplayValue(/34191/)).toBeInTheDocument()
  })

  it('Stripe: exibe link "visualizar boleto" (hosted_voucher_url)', () => {
    renderWith(baseStripe)
    const link = screen.getByRole('link', { name: /visualizar boleto/i })
    expect(link).toHaveAttribute('href', 'https://payments.stripe.com/voucher/abc')
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('Stripe: exibe link de PDF separado', () => {
    renderWith(baseStripe)
    const link = screen.getByRole('link', { name: /baixar pdf/i })
    expect(link).toHaveAttribute('href', 'https://stripe.com/voucher.pdf')
  })

  it('Mercado Pago: exibe só link de PDF (sem hosted_voucher_url)', () => {
    const mp: PaymentResult = {
      paymentId: 'pay-mp',
      provider: 'mercadopago',
      method: 'boleto',
      status: 'pending',
      barCode: '34191.09008 09133.610947 91020.150008 1 00010000012345',
      pdfUrl: 'https://mp.com/boleto.pdf',
      boletoExpiresAt: new Date('2026-04-30'),
    }
    renderWith(mp)
    expect(screen.queryByRole('link', { name: /visualizar boleto/i })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /baixar boleto em pdf/i })).toBeInTheDocument()
  })

  it('mock mode: pdfUrl="#" não renderiza link', () => {
    const mock: PaymentResult = {
      paymentId: 'mock-1',
      provider: 'mock',
      method: 'boleto',
      status: 'pending',
      barCode: 'XXXXXXX',
      pdfUrl: '#',
      boletoExpiresAt: new Date('2026-04-30'),
    }
    renderWith(mock)
    expect(screen.queryByRole('link', { name: /baixar/i })).not.toBeInTheDocument()
  })

  it('exibe data de vencimento formatada', () => {
    // ISO sem timezone vira UTC midnight; o toLocaleDateString rende localTZ-dependent
    // (BR: -03h faz 2026-04-30 UTC virar 2026-04-29 local). Mantemos só validação
    // de formato dd/mm/aaaa.
    renderWith(baseStripe)
    expect(screen.getByText(/\d{2}\/\d{2}\/2026/)).toBeInTheDocument()
  })
})
