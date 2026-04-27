import { useEffect, useRef, useCallback, useState } from 'react'
import { useParams, useSearchParams, Link, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, Clock, XCircle, Package } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { isApiEnabled } from '@/lib/api'
import { apiGet } from '@/lib/api'

type PaymentStatus = 'pending' | 'confirmed' | 'failed'

function usePollingConfirmation(
  paymentId: string,
  method: string,
  onUpdate: (status: PaymentStatus) => void
) {
  // U4 mesma justificativa: selector direto, não a função `s.token()`.
  const token = useAuthStore((s) => s.user?.token ?? null)
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const countRef = useRef(0)

  const stop = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }, [])

  useEffect(() => {
    // Pix e Boleto pendem confirmação async (PSP empurra via webhook).
    // Card vem como confirmed direto da Stripe Elements antes de chegar aqui.
    if (method !== 'pix' && method !== 'boleto') return
    if (!isApiEnabled) return

    pollingRef.current = setInterval(async () => {
      countRef.current++
      if (countRef.current > 100) {
        stop()
        return
      }
      try {
        const data = await apiGet<{ status: string }>(`/api/v1/payments/${paymentId}`, token ?? undefined)
        if (data.status === 'confirmed') {
          stop()
          onUpdate('confirmed')
        } else if (data.status === 'failed') {
          stop()
          onUpdate('failed')
        }
      } catch {
        // keep polling on transient errors
      }
    }, 3000)

    return stop
  }, [paymentId, method, token, stop, onUpdate])
}

interface ConfirmationLocationState {
  orderNumber?: string
}

export default function OrderConfirmationPage() {
  const { t } = useTranslation('checkout')
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const location = useLocation()
  const method = searchParams.get('method') ?? 'pix'
  const userEmail = useAuthStore((s) => s.user?.email ?? '')
  const paymentId = id ?? ''

  // U5: orderNumber humano (ex: "2026-ZGJWBMDE") passado via location.state pelo
  // CheckoutPage no momento da navegação. Fallback: 8 primeiros chars do paymentId.
  const orderNumber =
    (location.state as ConfirmationLocationState | null)?.orderNumber ??
    paymentId.slice(0, 8).toUpperCase()

  // B3: status reativo. Card já vem confirmed (frontend setou via Stripe Elements).
  // Pix/Boleto pendem confirmação webhook → polling atualiza este state.
  const [status, setStatus] = useState<PaymentStatus>(method === 'card' ? 'confirmed' : 'pending')

  const handleStatusUpdate = useCallback((newStatus: PaymentStatus) => {
    setStatus(newStatus)
  }, [])

  usePollingConfirmation(paymentId, method, handleStatusUpdate)

  const boletoDate = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toLocaleDateString('pt-BR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
  })

  function StatusBadge() {
    if (status === 'confirmed') {
      return (
        <div className="flex items-center gap-2 px-3 py-1 bg-green-100 text-green-700 rounded-full text-sm font-semibold">
          <CheckCircle2 className="h-4 w-4" />
          {t('confirmation.confirmed')}
        </div>
      )
    }
    if (status === 'failed') {
      return (
        <div className="flex items-center gap-2 px-3 py-1 bg-red-100 text-red-700 rounded-full text-sm font-semibold">
          <XCircle className="h-4 w-4" />
          {t('confirmation.failed', { defaultValue: 'Falhou' })}
        </div>
      )
    }
    return (
      <div className="flex items-center gap-2 px-3 py-1 bg-amber-100 text-amber-700 rounded-full text-sm font-semibold">
        <Clock className="h-4 w-4" />
        {t('confirmation.pending')}
      </div>
    )
  }

  function MethodMessage() {
    if (status === 'confirmed') {
      return <p className="text-sm text-gray-500 text-center">{t('confirmation.cardApproved')}</p>
    }
    if (status === 'failed') {
      return (
        <p className="text-sm text-gray-500 text-center">
          {t('confirmation.failedHint', {
            defaultValue: 'O pagamento foi recusado. Tente novamente em alguns minutos.',
          })}
        </p>
      )
    }
    if (method === 'pix') {
      return <p className="text-sm text-gray-500 text-center">{t('confirmation.pixPending')}</p>
    }
    if (method === 'boleto') {
      return (
        <p className="text-sm text-gray-500 text-center">
          {t('confirmation.boletoPending', { date: boletoDate })}
        </p>
      )
    }
    return <p className="text-sm text-gray-500 text-center">{t('confirmation.cardApproved')}</p>
  }

  const IconEl = status === 'confirmed' ? CheckCircle2 : status === 'failed' ? XCircle : Package

  return (
    <div className="container py-16 flex flex-col items-center gap-6 text-center max-w-md mx-auto">
      <IconEl
        className={`h-20 w-20 ${
          status === 'confirmed'
            ? 'text-green-500'
            : status === 'failed'
              ? 'text-red-500'
              : 'text-brand-orange'
        }`}
      />

      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-bold text-gray-900">{t('confirmation.title')}</h1>
        <p className="text-sm text-gray-400">{t('confirmation.orderNumber', { id: orderNumber })}</p>
      </div>

      <StatusBadge />

      <MethodMessage />

      {userEmail && (
        <p className="text-sm text-gray-400">
          {t('confirmation.emailSent', { email: userEmail })}
        </p>
      )}

      <div className="flex flex-col sm:flex-row gap-3 w-full mt-2">
        <Link
          to="/conta"
          className="flex-1 h-11 rounded-xl border border-gray-300 text-sm font-semibold text-gray-700 flex items-center justify-center hover:bg-gray-50 transition-colors"
        >
          {t('confirmation.trackOrder')}
        </Link>
        <Link
          to="/"
          className="flex-1 h-11 rounded-xl bg-brand-orange text-white text-sm font-semibold flex items-center justify-center hover:bg-brand-orange-dark transition-colors"
        >
          {t('confirmation.continueShopping')}
        </Link>
      </div>
    </div>
  )
}
