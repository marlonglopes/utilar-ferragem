import { useState, useEffect, useRef, useCallback } from 'react'
import { apiPost, apiGet, isApiEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

export type PaymentMethod = 'pix' | 'boleto' | 'card'
export type PaymentStatus = 'idle' | 'creating' | 'pending' | 'confirmed' | 'failed' | 'expired'

export interface PaymentResult {
  paymentId: string
  method: PaymentMethod
  status: PaymentStatus
  // Pix
  qrCodeBase64?: string
  copyPaste?: string
  expiresAt?: Date
  // Boleto
  barCode?: string
  pdfUrl?: string
  boletoExpiresAt?: Date
  // Card
  initPoint?: string
}

const MOCK_PIX_QR = 'iVBORw0KGgoAAAANSUhEUgAAAMgAAADICAYAAACtWK6eAAAACXBIWXMAAAsTAAALEwEAmpwYAAAF'
const MOCK_PIX_CODE = '00020126360014BR.GOV.BCB.PIX0114+5511999990000520400005303986540510.005802BR5913UtiLar Ferragem6009SAO PAULO62070503***6304B14A'
const MOCK_BOLETO = '34191.09008 09133.610947 91020.150008 1 00010000012345'

function mockPixResult(orderId: string): PaymentResult {
  const expiresAt = new Date(Date.now() + 15 * 60 * 1000) // 15 min
  return {
    paymentId: `mock-pay-${orderId}`,
    method: 'pix',
    status: 'pending',
    qrCodeBase64: MOCK_PIX_QR,
    copyPaste: MOCK_PIX_CODE,
    expiresAt,
  }
}

function mockBoletoResult(orderId: string): PaymentResult {
  const boletoExpiresAt = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000) // 3 days
  return {
    paymentId: `mock-pay-${orderId}`,
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
    method: 'card',
    status: 'pending',
    initPoint: '#',
  }
}

function parseApiResult(raw: Record<string, unknown>, method: PaymentMethod): PaymentResult {
  const psp = (raw.psp_payload ?? {}) as Record<string, unknown>
  const txData = ((psp.point_of_interaction as Record<string, unknown> | undefined)?.transaction_data ?? {}) as Record<string, unknown>

  const result: PaymentResult = {
    paymentId: raw.id as string,
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
      // Mock: auto-confirm after 6s in dev
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
        // transient error — keep polling
      }
    }, 3000)
  }, [token, stopPolling])

  const createPayment = useCallback(async (
    orderId: string,
    method: PaymentMethod,
    amount: number,
  ) => {
    setError('')
    setResult((prev) => prev ? { ...prev, status: 'creating' } : { paymentId: '', method, status: 'creating' })

    try {
      if (!isApiEnabled) {
        await new Promise((r) => setTimeout(r, 600)) // simulate latency
        let mock: PaymentResult
        if (method === 'pix') mock = mockPixResult(orderId)
        else if (method === 'boleto') mock = mockBoletoResult(orderId)
        else mock = mockCardResult(orderId)
        setResult(mock)
        if (method === 'pix') startPolling(mock.paymentId)
        return mock
      }

      const data = await apiPost<Record<string, unknown>>(
        '/api/v1/payments',
        { order_id: orderId, method, amount },
        token ?? undefined,
      )
      const parsed = parseApiResult(data, method)
      setResult(parsed)
      if (method === 'pix') startPolling(parsed.paymentId)
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

  return { result, error, createPayment, simulateConfirm, stopPolling }
}
