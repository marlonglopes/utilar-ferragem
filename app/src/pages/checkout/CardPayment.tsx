import { useTranslation } from 'react-i18next'
import { ExternalLink, CheckCircle2 } from 'lucide-react'
import { isApiEnabled } from '@/lib/api'
import type { PaymentResult } from '@/hooks/usePayment'

interface Props {
  result: PaymentResult
  onSimulateConfirm?: () => void
}

export default function CardPayment({ result, onSimulateConfirm }: Props) {
  const { t } = useTranslation('checkout')

  if (result.status === 'confirmed') {
    return (
      <div className="flex flex-col items-center gap-4 py-10 text-center">
        <CheckCircle2 className="h-16 w-16 text-green-500" />
        <p className="text-xl font-bold text-gray-900">{t('pix.confirmed')}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('card.title')}</h2>

      <p className="text-sm text-gray-500">{t('card.hosted')}</p>

      {result.initPoint && result.initPoint !== '#' ? (
        <a
          href={result.initPoint}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center justify-center gap-2 h-11 rounded-xl bg-brand-blue text-white font-semibold text-sm hover:opacity-90 transition-opacity"
        >
          <ExternalLink className="h-4 w-4" />
          {t('card.proceed')}
        </a>
      ) : (
        <button
          disabled
          className="flex items-center justify-center gap-2 h-11 rounded-xl bg-brand-blue text-white font-semibold text-sm opacity-60 cursor-not-allowed"
        >
          <ExternalLink className="h-4 w-4" />
          {t('card.proceed')}
        </button>
      )}

      {!isApiEnabled && onSimulateConfirm && (
        <button
          onClick={onSimulateConfirm}
          className="h-10 rounded-xl border border-dashed border-gray-300 text-sm text-gray-500 hover:text-gray-700 hover:border-gray-400 transition-colors"
        >
          {t('card.simulate')}
        </button>
      )}
    </div>
  )
}
