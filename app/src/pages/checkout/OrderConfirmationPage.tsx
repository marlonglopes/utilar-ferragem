import { useEffect, useRef, useCallback } from 'react'
import { useParams, useSearchParams, Link } from 'react-router-dom'
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
  const token = useAuthStore((s) => s.token())
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const countRef = useRef(0)

  const stop = useCallback(() => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }, [])

  useEffect(() => {
    // Only poll for pix — card and boleto don't need real-time confirmation here
    if (method !== 'pix') return
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

export default function OrderConfirmationPage() {
  const { t } = useTranslation('checkout')
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const method = searchParams.get('method') ?? 'pix'
  const userEmail = useAuthStore((s) => s.user?.email ?? '')

  // In dev mode, status is derived from the URL method (always pending initially)
  // Real polling handled by hook above
  const paymentId = id ?? ''

  usePollingConfirmation(paymentId, method, (_status) => {
    // In a real app, update local state; for now the page re-reads on nav
  })

  const boletoDate = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toLocaleDateString('pt-BR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
  })

  function StatusBadge() {
    if (method === 'card') {
      return (
        <div className="flex items-center gap-2 px-3 py-1 bg-green-100 text-green-700 rounded-full text-sm font-semibold">
          <CheckCircle2 className="h-4 w-4" />
          {t('confirmation.confirmed')}
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

  const IconEl = method === 'card' ? CheckCircle2 : method === 'failed' ? XCircle : Package

  return (
    <div className="container py-16 flex flex-col items-center gap-6 text-center max-w-md mx-auto">
      <IconEl
        className={`h-20 w-20 ${method === 'card' ? 'text-green-500' : method === 'failed' ? 'text-red-500' : 'text-brand-orange'}`}
      />

      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-bold text-gray-900">{t('confirmation.title')}</h1>
        <p className="text-sm text-gray-400">{t('confirmation.orderNumber', { id: paymentId })}</p>
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
