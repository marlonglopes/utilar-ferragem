import { describe, it, expect } from 'vitest'
import { paymentResultFromApi } from '@/hooks/usePayment'

/**
 * Regressão: `parseApiResult` só conhecia 'stripe' e 'mercadopago'. Com
 * PSP_PROVIDER=appmax o resultado caía no parser da Stripe, que procura
 * `next_action.pix_display_qr_code` — chave que a Appmax nunca envia. Resultado:
 * tela de Pix em branco, sem QR e sem copia-e-cola, ou seja, nenhuma venda.
 *
 * Os dois gateways Appmax (v3 admin e v1 AppStore) emitem o mesmo ClientData
 * plano, então ambos são cobertos aqui.
 */
describe('usePayment — parser Appmax', () => {
  const providers = ['appmax', 'appmax-v1'] as const

  describe.each(providers)('provider=%s', (provider) => {
    it('extrai QR e copia-e-cola do Pix', () => {
      const result = paymentResultFromApi(
        {
          id: 'pay-1',
          psp_id: '70719',
          provider,
          method: 'pix',
          status: 'pending',
          psp_payload: {
            provider,
            pix_qrcode: 'iVBORw0KGgoAAAANSUhEUg==',
            pix_emv: '00020126580014BR.GOV.BCB.PIX0136utilar-emv6304A1B2',
            pix_expires_at: '2026-07-18T18:30:00Z',
          },
        },
        'pix',
      )

      expect(result.provider).toBe(provider)
      expect(result.pspId).toBe('70719')
      // base64 cru, SEM prefixo data: — o PixPayment monta o data URI
      expect(result.qrCodeBase64).toBe('iVBORw0KGgoAAAANSUhEUg==')
      expect(result.qrCodeBase64?.startsWith('data:')).toBe(false)
      expect(result.copyPaste).toContain('BR.GOV.BCB.PIX')
      expect(result.expiresAt?.toISOString()).toBe('2026-07-18T18:30:00.000Z')
    })

    it('cai para expiração de 30min quando a Appmax não manda a data', () => {
      const before = Date.now()
      const result = paymentResultFromApi(
        {
          id: 'pay-2',
          provider,
          method: 'pix',
          status: 'pending',
          psp_payload: { provider, pix_qrcode: 'QR', pix_emv: 'EMV' },
        },
        'pix',
      )

      const ms = result.expiresAt!.getTime() - before
      expect(ms).toBeGreaterThan(29 * 60 * 1000)
      expect(ms).toBeLessThanOrEqual(30 * 60 * 1000 + 1000)
    })

    it('ignora data de expiração inválida em vez de propagar Invalid Date', () => {
      const result = paymentResultFromApi(
        {
          id: 'pay-3',
          provider,
          method: 'pix',
          status: 'pending',
          psp_payload: { provider, pix_qrcode: 'QR', pix_emv: 'EMV', pix_expires_at: 'nao-e-data' },
        },
        'pix',
      )
      expect(Number.isNaN(result.expiresAt!.getTime())).toBe(false)
    })

    it('extrai PDF e linha digitável do boleto', () => {
      const result = paymentResultFromApi(
        {
          id: 'pay-4',
          provider,
          method: 'boleto',
          status: 'pending',
          psp_payload: {
            provider,
            boleto_url: 'https://appmax.com.br/boleto/123.pdf',
            boleto_line: '34191.79001 01043.510047 91020.150008 9 98770000128450',
          },
        },
        'boleto',
      )

      expect(result.pdfUrl).toBe('https://appmax.com.br/boleto/123.pdf')
      expect(result.barCode).toBe('34191.79001 01043.510047 91020.150008 9 98770000128450')
      expect(result.boletoExpiresAt).toBeInstanceOf(Date)
    })

    it('expõe as parcelas do cartão', () => {
      const result = paymentResultFromApi(
        {
          id: 'pay-5',
          provider,
          method: 'card',
          status: 'pending',
          psp_payload: { provider, installments: 6 },
        },
        'card',
      )
      expect(result.installments).toBe(6)
    })

    it('não quebra quando o psp_payload vem nulo', () => {
      const result = paymentResultFromApi(
        { id: 'pay-6', provider, method: 'pix', status: 'pending', psp_payload: null },
        'pix',
      )
      expect(result.paymentId).toBe('pay-6')
      expect(result.qrCodeBase64).toBeUndefined()
      expect(result.copyPaste).toBeUndefined()
      expect(result.status).toBe('pending')
    })
  })

  it('não desvia Stripe nem Mercado Pago para o parser da Appmax', () => {
    const stripe = paymentResultFromApi(
      {
        id: 'p',
        provider: 'stripe',
        method: 'pix',
        status: 'pending',
        psp_payload: {
          next_action: { pix_display_qr_code: { data: 'stripe-emv', image_url_png: 'https://s/qr.png' } },
        },
      },
      'pix',
    )
    expect(stripe.provider).toBe('stripe')
    expect(stripe.copyPaste).toBe('stripe-emv')

    const mp = paymentResultFromApi(
      {
        id: 'p',
        provider: 'mercadopago',
        method: 'pix',
        status: 'pending',
        psp_payload: {
          point_of_interaction: { transaction_data: { qr_code: 'mp-emv', qr_code_base64: 'mp-b64' } },
        },
      },
      'pix',
    )
    expect(mp.provider).toBe('mercadopago')
    expect(mp.copyPaste).toBe('mp-emv')
  })
})
