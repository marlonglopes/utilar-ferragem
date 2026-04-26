import { useState, useEffect, useRef, useCallback } from 'react'
import { apiPost, apiGet, isApiEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

export type PaymentMethod = 'pix' | 'boleto' | 'card'
export type PaymentStatus = 'idle' | 'creating' | 'pending' | 'confirmed' | 'failed' | 'expired'
export type PaymentProvider = 'stripe' | 'mercadopago' | 'mock'

export interface PaymentResult {
  paymentId: string
  pspId?: string
  provider: PaymentProvider
  method: PaymentMethod
  status: PaymentStatus
  // Stripe (card+pix+boleto): client_secret pra stripe.confirmPayment / Elements
  clientSecret?: string
  // Pix
  qrCodeBase64?: string
  copyPaste?: string
  expiresAt?: Date
  // Boleto
  barCode?: string
  pdfUrl?: string
  hostedVoucherUrl?: string
  boletoExpiresAt?: Date
  // Card (MP-only redirect)
  initPoint?: string
}

const MOCK_PIX_QR = 'iVBORw0KGgoAAAANSUhEUgAAAMgAAADICAYAAACtWK6eAAAACXBIWXMAAAsTAAALEwEAmpwYAAAF'
const MOCK_PIX_CODE = '00020126360014BR.GOV.BCB.PIX0114+5511999990000520400005303986540510.005802BR5913UtiLar Ferragem6009SAO PAULO62070503***6304B14A'
const MOCK_BOLETO = '34191.09008 09133.610947 91020.150008 1 00010000012345'

function mockPixResult(orderId: string): PaymentResult {
  const expiresAt = new Date(Date.now() + 15 * 60 * 1000)
  return {
    paymentId: `mock-pay-${orderId}`,
    provider: 'mock',
    method: 'pix',
    status: 'pending',
    qrCodeBase64: MOCK_PIX_QR,
    copyPaste: MOCK_PIX_CODE,
    expiresAt,
  }
}

function mockBoletoResult(orderId: string): PaymentResult {
  const boletoExpiresAt = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000)
  return {
    paymentId: `mock-pay-${orderId}`,
    provider: 'mock',
    method: 'boleto',
    status: 'pending',
    barCode: MOCK_BOLETO,
    pdfUrl: '#',
    boletoExpiresAt,
  }
}

function mockCardResult(orderId: string): PaymentResult {
  return {
    paymentId: `mock-pay-${orderId}`,
    provider: 'mock',
    method: 'card',
    status: 'pending',
    initPoint: '#',
  }
}

interface ApiPaymentResponse {
  id: string
  psp_id?: string
  provider?: PaymentProvider
  method: PaymentMethod
  status: string
  clientSecret?: string
  // Stripe: ClientData JSON com next_action; MP: payload bruto.
  psp_payload?: Record<string, unknown> | null
}

function parseStripeResult(raw: ApiPaymentResponse, method: PaymentMethod): PaymentResult {
  const clientData = (raw.psp_payload ?? {}) as Record<string, unknown>
  const nextAction = (clientData.next_action ?? {}) as Record<string, unknown>

  const result: PaymentResult = {
    paymentId: raw.id,
    pspId: raw.psp_id,
    provider: 'stripe',
    method,
    status: 'pending',
    clientSecret: raw.clientSecret ?? (clientData.client_secret as string | undefined),
  }

  if (method === 'pix') {
    const pixDisplay = (nextAction.pix_display_qr_code ?? {}) as Record<string, unknown>
    result.qrCodeBase64 = pixDisplay.image_url_png as string | undefined // Stripe entrega URL
    result.copyPaste = pixDisplay.data as string | undefined
    const expiresAt = pixDisplay.expires_at as number | undefined
    result.expiresAt = expiresAt
      ? new Date(expiresAt * 1000)
      : new Date(Date.now() + 15 * 60 * 1000)
  }
  if (method === 'boleto') {
    const boletoDisplay = (nextAction.boleto_display_details ?? {}) as Record<string, unknown>
    result.barCode = boletoDisplay.number as string | undefined
    result.pdfUrl = boletoDisplay.pdf as string | undefined
    result.hostedVoucherUrl = boletoDisplay.hosted_voucher_url as string | undefined
    const expiresAt = boletoDisplay.expires_at as number | undefined
    result.boletoExpiresAt = expiresAt
      ? new Date(expiresAt * 1000)
      : new Date(Date.now() + 3 * 24 * 60 * 60 * 1000)
  }
  // card: confirmPayment do Elements usa só clientSecret

  return result
}

function parseMercadoPagoResult(raw: ApiPaymentResponse, method: PaymentMethod): PaymentResult {
  const psp = (raw.psp_payload ?? {}) as Record<string, unknown>
  const txData = ((psp.point_of_interaction as Record<string, unknown> | undefined)?.transaction_data ?? {}) as Record<string, unknown>

  const result: PaymentResult = {
    paymentId: raw.id,
    pspId: raw.psp_id,
    provider: 'mercadopago',
    method,
    status: 'pending',
  }

  if (method === 'pix') {
    result.qrCodeBase64 = txData.qr_code_base64 as string
    result.copyPaste = txData.qr_code as string
    result.expiresAt = new Date(Date.now() + 15 * 60 * 1000)
  }
  if (method === 'boleto') {
    result.barCode = psp.barcode as string
    result.pdfUrl = psp.transaction_details
      ? ((psp.transaction_details as Record<string, unknown>).external_resource_url as string)
      : '#'
    result.boletoExpiresAt = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000)
  }
  if (method === 'card') {
    result.initPoint = psp.init_point as string
  }

  return result
}

function parseApiResult(raw: ApiPaymentResponse, method: PaymentMethod): PaymentResult {
  const provider = raw.provider ?? 'stripe' // backend default é stripe
  if (provider === 'mercadopago') return parseMercadoPagoResult(raw, method)
  return parseStripeResult(raw, method)
}

export function usePayment() {
  const token = useAuthStore((s) => s.token())
  const [result, setResult] = useState<PaymentResult | null>(null)
  const [error, setError] = useState('')
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const pollCountRef = useRef(0)
  const MAX_POLLS = 300 // 15min @ 3s

  const stopPolling = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }, [])

  useEffect(() => () => stopPolling(), [stopPolling])

  const startPolling = useCallback((paymentId: string) => {
    stopPolling()
    pollCountRef.current = 0

    if (!isApiEnabled) {
      pollingRef.current = setInterval(() => {
        pollCountRef.current++
        if (pollCountRef.current >= 2) {
          stopPolling()
          setResult((prev) => prev ? { ...prev, status: 'confirmed' } : prev)
        }
      }, 3000)
      return
    }

    pollingRef.current = setInterval(async () => {
      pollCountRef.current++
      if (pollCountRef.current > MAX_POLLS) {
        stopPolling()
        setResult((prev) => prev ? { ...prev, status: 'expired' } : prev)
        return
      }
      try {
        const data = await apiGet<{ status: string }>(`/api/v1/payments/${paymentId}`, token ?? undefined)
        if (data.status === 'confirmed') {
          stopPolling()
          setResult((prev) => prev ? { ...prev, status: 'confirmed' } : prev)
        } else if (data.status === 'failed') {
          stopPolling()
          setResult((prev) => prev ? { ...prev, status: 'failed' } : prev)
        }
      } catch {
        // transient — keep polling
      }
    }, 3000)
  }, [token, stopPolling])

  const createPayment = useCallback(async (
    orderId: string,
    method: PaymentMethod,
    amount: number,
    extras?: { payer_cpf?: string; payer_name?: string },
  ) => {
    setError('')
    setResult((prev) => prev
      ? { ...prev, status: 'creating' }
      : { paymentId: '', provider: 'mock', method, status: 'creating' })

    try {
      if (!isApiEnabled) {
        await new Promise((r) => setTimeout(r, 600))
        let mock: PaymentResult
        if (method === 'pix') mock = mockPixResult(orderId)
        else if (method === 'boleto') mock = mockBoletoResult(orderId)
        else mock = mockCardResult(orderId)
        setResult(mock)
        if (method === 'pix') startPolling(mock.paymentId)
        return mock
      }

      const data = await apiPost<ApiPaymentResponse>(
        '/api/v1/payments',
        { order_id: orderId, method, amount, ...(extras ?? {}) },
        token ?? undefined,
      )
      const parsed = parseApiResult(data, method)
      setResult(parsed)
      // Pix/Boleto: poll. Card-Stripe: confirma via Elements (frontend marca confirmed manualmente).
      if (method === 'pix' || method === 'boleto') startPolling(parsed.paymentId)
      return parsed
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Erro ao criar pagamento'
      setError(msg)
      setResult(null)
    }
  }, [token, startPolling])

  const simulateConfirm = useCallback(() => {
    stopPolling()
    setResult((prev) => prev ? { ...prev, status: 'confirmed' } : prev)
  }, [stopPolling])

  // Marca como pendente — usado depois de stripe.confirmPayment retornar succeeded.
  // O webhook vai promover pra confirmed real, mas o frontend pode mostrar feedback otimista.
  const markPending = useCallback(() => {
    setResult((prev) => prev ? { ...prev, status: 'pending' } : prev)
  }, [])

  const markConfirmed = useCallback(() => {
    stopPolling()
    setResult((prev) => prev ? { ...prev, status: 'confirmed' } : prev)
  }, [stopPolling])

  const markFailed = useCallback((msg?: string) => {
    stopPolling()
    if (msg) setError(msg)
    setResult((prev) => prev ? { ...prev, status: 'failed' } : prev)
  }, [stopPolling])

  return {
    result,
    error,
    createPayment,
    simulateConfirm,
    markPending,
    markConfirmed,
    markFailed,
    stopPolling,
  }
}
