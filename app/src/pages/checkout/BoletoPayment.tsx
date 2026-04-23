import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Check, Download, AlertTriangle } from 'lucide-react'
import type { PaymentResult } from '@/hooks/usePayment'

interface Props {
  result: PaymentResult
}

export default function BoletoPayment({ result }: Props) {
  const { t } = useTranslation('checkout')
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    if (!result.barCode) return
    try {
      await navigator.clipboard.writeText(result.barCode)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // fallback
    }
  }

  const expiryFormatted = result.boletoExpiresAt
    ? result.boletoExpiresAt.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric' })
    : ''

  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('boleto.title')}</h2>

      {/* Warning banner */}
      <div className="flex gap-3 p-4 bg-amber-50 border border-amber-200 rounded-xl text-sm text-amber-800">
        <AlertTriangle className="h-5 w-5 flex-shrink-0 text-amber-500 mt-0.5" />
        <p>{t('boleto.warning')}</p>
      </div>

      {/* Bar code */}
      {result.barCode && (
        <div className="flex flex-col gap-2">
          <p className="text-sm font-medium text-gray-700">{t('boleto.code')}</p>
          <div className="flex gap-2">
            <input
              readOnly
              value={result.barCode}
              className="flex-1 h-10 px-3 rounded-xl border border-gray-200 bg-gray-50 text-xs font-mono text-gray-700 truncate focus:outline-none"
              onClick={(e) => (e.target as HTMLInputElement).select()}
            />
            <button
              onClick={handleCopy}
              className="h-10 px-4 rounded-xl bg-brand-orange text-white font-semibold text-sm hover:bg-brand-orange-dark transition-colors flex items-center gap-2 flex-shrink-0"
            >
              {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              {copied ? t('boleto.copied') : t('boleto.copy')}
            </button>
          </div>
        </div>
      )}

      {/* Expiry */}
      {expiryFormatted && (
        <p className="text-sm text-gray-500">
          {t('boleto.expires', { date: expiryFormatted })}
        </p>
      )}

      {/* PDF download */}
      {result.pdfUrl && result.pdfUrl !== '#' && (
        <a
          href={result.pdfUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-2 px-5 py-2.5 rounded-xl border border-gray-300 text-sm font-semibold text-gray-700 hover:bg-gray-50 transition-colors self-start"
        >
          <Download className="h-4 w-4" />
          {t('boleto.download')}
        </a>
      )}
    </div>
  )
}
