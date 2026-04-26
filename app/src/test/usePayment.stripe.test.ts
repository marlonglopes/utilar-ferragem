/**
 * Testa o caminho com VITE_API_URL configurada (modo "API enabled") e provider Stripe.
 *
 * Estratégia: monkeypatch global.fetch para simular respostas do payment-service.
 * Como `isApiEnabled` é resolvido em import-time a partir de `import.meta.env.VITE_API_URL`,
 * usamos vi.stubEnv antes de importar o módulo via dynamic import.
 */
import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const STRIPE_PI_RESPONSE = {
  id: 'pay-uuid-1',
  psp_id: 'pi_3TQa8WLQCtijFcSY12pJyBht',
  provider: 'stripe',
  method: 'card',
  status: 'pending',
  clientSecret: 'pi_3TQa8WLQCtijFcSY12pJyBht_secret_TvretvvbOmJsklIWVyHIeujmp',
  psp_payload: {
    type: 'card',
    client_secret: 'pi_3TQa8WLQCtijFcSY12pJyBht_secret_TvretvvbOmJsklIWVyHIeujmp',
  },
}

const STRIPE_PIX_RESPONSE = {
  id: 'pay-uuid-pix',
  psp_id: 'pi_pix_xyz',
  provider: 'stripe',
  method: 'pix',
  status: 'pending',
  clientSecret: 'pi_pix_xyz_secret_abc',
  psp_payload: {
    type: 'pix_display_qr_code',
    client_secret: 'pi_pix_xyz_secret_abc',
    next_action: {
      pix_display_qr_code: {
        data: '00020101021226...PIX_COPY_PASTE',
        image_url_png: 'https://stripe.com/qr.png',
        expires_at: Math.floor(Date.now() / 1000) + 900,
      },
    },
  },
}

const STRIPE_BOLETO_RESPONSE = {
  id: 'pay-uuid-boleto',
  psp_id: 'pi_boleto_xyz',
  provider: 'stripe',
  method: 'boleto',
  status: 'pending',
  clientSecret: 'pi_boleto_xyz_secret',
  psp_payload: {
    type: 'boleto_display_details',
    client_secret: 'pi_boleto_xyz_secret',
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

const MP_PIX_RESPONSE = {
  id: 'pay-uuid-mp-pix',
  psp_id: 'mp-pay-001',
  provider: 'mercadopago',
  method: 'pix',
  status: 'pending',
  psp_payload: {
    point_of_interaction: {
      transaction_data: {
        qr_code: '00020101...MP_PIX',
        qr_code_base64: 'base64_qr_string',
      },
    },
  },
}

beforeEach(() => {
  vi.stubEnv('VITE_API_URL', 'http://localhost:8090')
  vi.resetModules()
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.restoreAllMocks()
})

describe('usePayment — Stripe API mode', () => {
  it('parses Stripe card response: extrai clientSecret e provider', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_PI_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'card',
        99.9,
      )
    })

    expect(result.current.result?.provider).toBe('stripe')
    expect(result.current.result?.method).toBe('card')
    expect(result.current.result?.clientSecret).toBe(
      'pi_3TQa8WLQCtijFcSY12pJyBht_secret_TvretvvbOmJsklIWVyHIeujmp',
    )
    expect(result.current.result?.pspId).toBe('pi_3TQa8WLQCtijFcSY12pJyBht')
    expect(result.current.result?.status).toBe('pending')
  })

  it('parses Stripe pix response: extrai QR + copy-paste do next_action', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_PIX_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'pix',
        99.9,
      )
    })

    expect(result.current.result?.provider).toBe('stripe')
    expect(result.current.result?.copyPaste).toBe('00020101021226...PIX_COPY_PASTE')
    expect(result.current.result?.qrCodeBase64).toBe('https://stripe.com/qr.png')
    expect(result.current.result?.expiresAt).toBeInstanceOf(Date)
  })

  it('parses Stripe boleto response: extrai hostedVoucherUrl + barCode + pdf', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_BOLETO_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'boleto',
        99.9,
        { payer_cpf: '12345678901', payer_name: 'Ana Silva' },
      )
    })

    expect(result.current.result?.provider).toBe('stripe')
    expect(result.current.result?.barCode).toBe(
      '34191.09008 09133.610947 91020.150008 1 00010000012345',
    )
    expect(result.current.result?.pdfUrl).toBe('https://stripe.com/voucher.pdf')
    expect(result.current.result?.hostedVoucherUrl).toBe(
      'https://payments.stripe.com/voucher/abc',
    )
    expect(result.current.result?.boletoExpiresAt).toBeInstanceOf(Date)
  })

  it('boleto extras (cpf+name) são propagados no body do POST', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_BOLETO_RESPONSE,
    } as Response)
    vi.stubGlobal('fetch', fetchMock)

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'boleto',
        99.9,
        { payer_cpf: '12345678901', payer_name: 'Ana Silva' },
      )
    })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [, init] = fetchMock.mock.calls[0]
    const body = JSON.parse(init.body)
    expect(body).toMatchObject({
      method: 'boleto',
      payer_cpf: '12345678901',
      payer_name: 'Ana Silva',
    })
  })

  it('parses Mercado Pago pix response (provider=mercadopago)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => MP_PIX_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'pix',
        99.9,
      )
    })

    expect(result.current.result?.provider).toBe('mercadopago')
    expect(result.current.result?.copyPaste).toBe('00020101...MP_PIX')
    expect(result.current.result?.qrCodeBase64).toBe('base64_qr_string')
  })

  it('markConfirmed promove status para confirmed e para o polling', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_PI_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'card',
        99.9,
      )
    })

    expect(result.current.result?.status).toBe('pending')

    act(() => {
      result.current.markConfirmed()
    })

    expect(result.current.result?.status).toBe('confirmed')
  })

  it('markFailed seta status=failed e error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => STRIPE_PI_RESPONSE,
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'card',
        99.9,
      )
    })

    act(() => {
      result.current.markFailed('Cartão recusado')
    })

    expect(result.current.result?.status).toBe('failed')
    expect(result.current.error).toBe('Cartão recusado')
  })

  it('erro 4xx do backend seta error e zera o result', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ error: 'invalid request', messages: ['amount must be > 0'] }),
    } as Response))

    const { usePayment } = await import('@/hooks/usePayment')
    const { result } = renderHook(() => usePayment())

    await act(async () => {
      await result.current.createPayment(
        '4848be8b-320d-48ee-9629-09ca096f38d6',
        'card',
        0,
      )
    })

    expect(result.current.result).toBeNull()
    expect(result.current.error).toContain('amount must be > 0')
  })
})
