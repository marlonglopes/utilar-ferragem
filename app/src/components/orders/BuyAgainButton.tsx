import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  RotateCcw,
  Loader2,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  TrendingUp,
} from 'lucide-react'
import { useBuyAgain } from '@/hooks/useBuyAgain'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import type { BuyAgainLine, BuyAgainResult } from '@/lib/buyAgain'
import type { Order } from '@/lib/mockOrders'

function outcomeTone(outcome: BuyAgainLine['outcome']) {
  switch (outcome) {
    case 'added':
      return 'text-green-700'
    case 'reduced':
      return 'text-amber-700'
    case 'lookup_failed':
      return 'text-gray-500'
    default:
      return 'text-red-600'
  }
}

/**
 * Resumo do que entrou e do que não entrou.
 *
 * Esta é a parte que importa da recompra. Adicionar 3 de 5 itens em silêncio
 * faz o cliente fechar o pedido achando que levou os 5 — e descobrir a falta no
 * meio da obra, que é quando o erro custa uma viagem. Então o resumo é
 * bloqueante (modal), não um toast que some em 4 segundos.
 */
function ResultDialog({ result, onClose }: { result: BuyAgainResult; onClose: () => void }) {
  const { t } = useTranslation()

  const problems = [
    result.outOfStock.length && t('orders.buyAgainOutOfStock', { count: result.outOfStock.length }),
    result.discontinued.length &&
      t('orders.buyAgainDiscontinued', { count: result.discontinued.length }),
    result.reduced.length && t('orders.buyAgainReduced', { count: result.reduced.length }),
    result.failed.length && t('orders.buyAgainFailed', { count: result.failed.length }),
  ].filter(Boolean) as string[]

  const allGood = !result.partial && !result.nothingAdded && problems.length === 0
  const Icon = result.nothingAdded ? XCircle : allGood ? CheckCircle2 : AlertTriangle
  const tone = result.nothingAdded
    ? 'text-red-500'
    : allGood
      ? 'text-green-500'
      : 'text-amber-500'

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 p-0 sm:items-center sm:p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="buy-again-title"
        className="flex max-h-[85vh] w-full max-w-md flex-col rounded-t-2xl bg-white shadow-xl sm:rounded-2xl"
      >
        <div className="flex items-start gap-3 border-b border-gray-100 p-5">
          <Icon className={cn('mt-0.5 h-5 w-5 flex-shrink-0', tone)} aria-hidden />
          <div className="min-w-0">
            <h2 id="buy-again-title" className="text-sm font-semibold text-gray-900">
              {t('orders.buyAgainTitle')}
            </h2>
            <p className="mt-1 text-sm text-gray-600">
              {result.nothingAdded
                ? t('orders.buyAgainNothing')
                : allGood
                  ? t('orders.buyAgainAllAdded', { count: result.addedLines })
                  : t('orders.buyAgainSummary', {
                      added: result.addedLines,
                      total: result.totalLines,
                    })}
            </p>
            {problems.length > 0 && (
              <p className="mt-1 text-xs text-gray-500">{problems.join(' · ')}</p>
            )}
            {result.priceChanged.length > 0 && (
              <p className="mt-1.5 flex items-center gap-1.5 text-xs font-medium text-amber-700">
                <TrendingUp className="h-3.5 w-3.5 flex-shrink-0" aria-hidden />
                {t('orders.buyAgainPriceChanged', { count: result.priceChanged.length })}
              </p>
            )}
          </div>
        </div>

        {/* Detalhe item a item — o resumo diz quanto, isto diz qual. */}
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
            {t('orders.buyAgainDetail')}
          </p>
          <ul className="flex flex-col gap-2.5">
            {result.lines.map((line) => (
              <li key={line.productId} className="flex items-start justify-between gap-3 text-sm">
                <span className="min-w-0 flex-1 text-gray-700">{line.name}</span>
                <span
                  className={cn('flex-shrink-0 text-right text-xs', outcomeTone(line.outcome))}
                >
                  <span className="block font-medium">
                    {t(`orders.outcome.${line.outcome}`, {
                      added: line.addedQty,
                      requested: line.requestedQty,
                    })}
                  </span>
                  {line.newPrice !== undefined &&
                    Math.round(line.newPrice * 100) !== Math.round(line.oldPrice * 100) &&
                    line.addedQty > 0 && (
                      <span className="mt-0.5 block text-[11px] text-gray-400">
                        {t('orders.priceWas', {
                          old: formatCurrency(line.oldPrice),
                          new: formatCurrency(line.newPrice),
                        })}
                      </span>
                    )}
                </span>
              </li>
            ))}
          </ul>
        </div>

        <div className="flex flex-col gap-2 border-t border-gray-100 p-5 sm:flex-row-reverse">
          {!result.nothingAdded && (
            <Link
              to="/carrinho"
              onClick={onClose}
              className="flex h-11 flex-1 items-center justify-center rounded-xl bg-brand-orange text-sm font-semibold text-white transition-colors hover:bg-brand-orange-dark"
            >
              {t('orders.buyAgainGoToCart')}
            </Link>
          )}
          <button
            type="button"
            onClick={onClose}
            className="h-11 flex-1 rounded-xl border border-gray-300 text-sm font-semibold text-gray-600 transition-colors hover:bg-gray-50"
          >
            {result.nothingAdded ? t('close') : t('orders.buyAgainKeepShopping')}
          </button>
        </div>
      </div>
    </div>
  )
}

export interface BuyAgainButtonProps {
  order: Order
  /** `full` é o CTA do detalhe; `compact` é o botão discreto na lista. */
  variant?: 'full' | 'compact'
  className?: string
}

export function BuyAgainButton({ order, variant = 'full', className }: BuyAgainButtonProps) {
  const { t } = useTranslation()
  const { run, result, loading, dismiss } = useBuyAgain()

  return (
    <>
      <button
        type="button"
        onClick={(e) => {
          // Na lista de pedidos, este botão vive dentro de um card cujo link é
          // "esticado" por cima de tudo. Sem parar a propagação, recomprar
          // navegaria pro detalhe do pedido no mesmo clique.
          e.preventDefault()
          e.stopPropagation()
          void run(order.items)
        }}
        disabled={loading}
        aria-busy={loading}
        className={cn(
          'relative z-10 flex items-center justify-center gap-2 rounded-xl font-semibold transition-colors disabled:opacity-60',
          variant === 'full' &&
            'h-11 flex-1 border border-gray-300 text-sm text-gray-700 hover:bg-gray-50',
          variant === 'compact' &&
            'h-9 whitespace-nowrap border border-gray-300 px-3 text-xs text-gray-600 hover:border-brand-orange hover:text-brand-orange',
          className
        )}
      >
        {loading ? (
          <Loader2 className={cn('animate-spin', variant === 'full' ? 'h-4 w-4' : 'h-3.5 w-3.5')} aria-hidden />
        ) : (
          <RotateCcw className={variant === 'full' ? 'h-4 w-4' : 'h-3.5 w-3.5'} aria-hidden />
        )}
        {loading ? t('orders.buyAgainRunning') : t('orders.buyAgain')}
      </button>

      {result && <ResultDialog result={result} onClose={dismiss} />}
    </>
  )
}
