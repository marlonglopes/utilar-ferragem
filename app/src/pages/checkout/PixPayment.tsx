import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Check, RefreshCw, Loader2, CheckCircle2 } from 'lucide-react'
import { isApiEnabled } from '@/lib/api'
import type { PaymentResult } from '@/hooks/usePayment'

interface Props {
  result: PaymentResult
  onRegenerate: () => void
  onSimulateConfirm?: () => void
}

function useCountdown(expiresAt: Date | undefined) {
  const [remaining, setRemaining] = useState<number>(() =>
    expiresAt ? Math.max(0, Math.floor((expiresAt.getTime() - Date.now()) / 1000)) : 0
  )

  useEffect(() => {
    if (!expiresAt) return
    const id = setInterval(() => {
      const secs = Math.max(0, Math.floor((expiresAt.getTime() - Date.now()) / 1000))
      setRemaining(secs)
      if (secs === 0) clearInterval(id)
    }, 1000)
    return () => clearInterval(id)
  }, [expiresAt])

  const mm = String(Math.floor(remaining / 60)).padStart(2, '0')
  const ss = String(remaining % 60).padStart(2, '0')
  return { remaining, formatted: `${mm}:${ss}` }
}

export default function PixPayment({ result, onRegenerate, onSimulateConfirm }: Props) {
  const { t } = useTranslation('checkout')
  const [copied, setCopied] = useState(false)
  const { remaining, formatted } = useCountdown(result.expiresAt)

  const isExpired = remaining === 0 && result.status !== 'confirmed'
  const isConfirmed = result.status === 'confirmed'

  async function handleCopy() {
    if (!result.copyPaste) return
    try {
      await navigator.clipboard.writeText(result.copyPaste)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // fallback: select text
    }
  }

  if (isConfirmed) {
    return (
      <div className="flex flex-col items-center gap-4 py-10 text-center">
        <CheckCircle2 className="h-16 w-16 text-green-500" />
        <p className="text-xl font-bold text-gray-900">{t('pix.confirmed')}</p>
        {result.expiresAt && (
          <p className="text-sm text-green-600">{t('pix.discount')}</p>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <h2 className="text-lg font-bold text-gray-900">{t('pix.title')}</h2>

      {isExpired ? (
        <div className="flex flex-col items-center gap-4 py-8 text-center">
          <p className="text-gray-500">{t('pix.expired')}</p>
          <button
            onClick={onRegenerate}
            className="flex items-center gap-2 px-5 py-2.5 rounded-xl bg-brand-orange text-white font-semibold text-sm hover:bg-brand-orange-dark transition-colors"
          >
            <RefreshCw className="h-4 w-4" />
            {t('pix.regenerate')}
          </button>
        </div>
      ) : (
        <>
          {/* QR code */}
          <div className="flex flex-col items-center gap-3">
            <p className="text-sm text-gray-500">{t('pix.scanQR')}</p>
            {result.qrCodeBase64 ? (
              <img
                // Stripe entrega URL HTTP em pix_display_qr_code.image_url_png;
                // MP/mock entregam base64 nu — detecta e prefixa só nesse caso.
                src={
                  result.qrCodeBase64.startsWith('http')
                    ? result.qrCodeBase64
                    : `data:image/png;base64,${result.qrCodeBase64}`
                }
                alt="QR Code Pix"
                className="w-48 h-48 rounded-lg border border-gray-200"
              />
            ) : (
              <div className="w-48 h-48 rounded-lg border border-gray-200 bg-gray-50 flex items-center justify-center">
                <Loader2 className="h-8 w-8 text-gray-300 animate-spin" />
              </div>
            )}

            {/* Countdown */}
            {result.expiresAt && (
              <p className="text-sm text-gray-500">
                {t('pix.expiresIn')}{' '}
                <span className={remaining < 60 ? 'text-red-500 font-bold' : 'font-semibold text-gray-900'}>
                  {formatted}
                </span>
              </p>
            )}
          </div>

          {/* Copy-paste */}
          {result.copyPaste && (
            <div className="flex flex-col gap-2">
              <p className="text-sm text-gray-500">{t('pix.orCopyPaste')}</p>
              <div className="flex gap-2">
                <input
                  readOnly
                  value={result.copyPaste}
                  className="flex-1 h-10 px-3 rounded-xl border border-gray-200 bg-gray-50 text-xs font-mono text-gray-700 truncate focus:outline-none"
                  onClick={(e) => (e.target as HTMLInputElement).select()}
                />
                <button
                  onClick={handleCopy}
                  className="h-10 px-4 rounded-xl bg-brand-orange text-white font-semibold text-sm hover:bg-brand-orange-dark transition-colors flex items-center gap-2 flex-shrink-0"
                >
                  {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  {copied ? t('pix.copied') : t('pix.copy')}
                </button>
              </div>
            </div>
          )}

          {/* Polling status */}
          <div className="flex items-center gap-2 text-sm text-gray-400">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('pix.waiting')}
          </div>

          {/* Sandbox shortcut */}
          {!isApiEnabled && onSimulateConfirm && (
            <button
              onClick={onSimulateConfirm}
              className="text-xs text-gray-400 underline hover:text-gray-600 self-start"
            >
              Simular confirmação (sandbox)
            </button>
          )}
        </>
      )}
    </div>
  )
}
