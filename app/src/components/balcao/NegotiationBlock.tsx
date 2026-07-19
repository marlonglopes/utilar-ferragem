import { AlertTriangle, Percent, ShieldCheck, TrendingDown } from 'lucide-react'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import { HEALTHY_MARGIN_PCT, type BalcaoPricing } from '@/store/balcaoStore'

/** Escala da barra: 40% de margem enche a barra inteira. */
const MARGIN_BAR_SCALE = 40
/** Teto do slider — desconto de balcão acima de 50% não é negociação, é erro. */
const MAX_SLIDER_PCT = 50

const QUICK_PCTS = [0, 5, 10, 12]

const STATUS_BAR: Record<BalcaoPricing['status'], string> = {
  healthy: 'bg-green-500',
  warning: 'bg-amber-500',
  negative: 'bg-red-500',
}

const STATUS_TEXT: Record<BalcaoPricing['status'], string> = {
  healthy: 'text-green-700',
  warning: 'text-amber-700',
  negative: 'text-red-700',
}

const STATUS_LABEL: Record<BalcaoPricing['status'], string> = {
  healthy: 'Margem saudável',
  warning: 'Margem apertada',
  negative: 'Venda no prejuízo',
}

export interface NegotiationBlockProps {
  pricing: BalcaoPricing
  onDiscountChange: (pct: number) => void
  disabled?: boolean
  /**
   * O teto exibido veio mesmo de `GET /api/v1/store/me`? Quando `false` o
   * número é fallback de demonstração ou fail-closed, e dizer isso evita que o
   * vendedor negocie contra um limite que a loja não reconhece.
   */
  ceilingFromBackend?: boolean
}

/**
 * Bloco de negociação: desconto ao vivo com feedback de margem.
 *
 * Todo o cálculo vem pronto de `computeBalcaoPricing` (função pura, testada).
 * Este componente só pinta — nenhuma regra de negócio mora aqui.
 */
export function NegotiationBlock({ pricing, onDiscountChange, disabled }: NegotiationBlockProps) {
  const { discountPct, discountAmount, marginPct, status, belowCost, ceilingPct, overCeiling } =
    pricing

  // Barra nunca fica negativa visualmente: margem < 0 vira barra cheia vermelha.
  const fillPct =
    marginPct < 0 ? 100 : Math.min(100, (marginPct / MARGIN_BAR_SCALE) * 100)

  return (
    <div className="border-t border-gray-200 bg-gray-50 p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="flex items-center gap-2 font-display text-sm font-bold uppercase tracking-wide text-gray-700">
          <Percent className="h-4 w-4" aria-hidden="true" />
          Negociação
        </h3>
        <span className="text-xs font-medium text-gray-500">Seu teto: {ceilingPct}%</span>
      </div>

      {/* Slider de desconto */}
      <label htmlFor="balcao-desconto" className="sr-only">
        Desconto em porcentagem
      </label>
      <input
        id="balcao-desconto"
        type="range"
        min={0}
        max={MAX_SLIDER_PCT}
        step={0.5}
        value={Math.min(discountPct, MAX_SLIDER_PCT)}
        disabled={disabled}
        onChange={(e) => onDiscountChange(Number(e.target.value))}
        aria-valuetext={`${discountPct}% de desconto`}
        className="h-12 w-full cursor-pointer accent-brand-orange disabled:opacity-50"
      />

      <div className="mt-1 flex items-baseline justify-between">
        <span className="font-display text-2xl font-bold text-brand-orange">
          {discountPct.toFixed(1).replace('.', ',')}%
        </span>
        <span className="text-sm font-semibold text-gray-700">
          − {formatCurrency(discountAmount)}
        </span>
      </div>

      {/* Atalhos de desconto */}
      <div className="mt-2 flex gap-2">
        {QUICK_PCTS.map((pct) => (
          <button
            key={pct}
            type="button"
            disabled={disabled}
            onClick={() => onDiscountChange(pct)}
            className={cn(
              'h-12 flex-1 rounded-lg border text-sm font-semibold transition-colors disabled:opacity-50',
              discountPct === pct
                ? 'border-brand-orange bg-brand-orange text-white'
                : 'border-gray-300 bg-white text-gray-700 hover:bg-gray-100'
            )}
          >
            {pct}%
          </button>
        ))}
      </div>

      {/* Barra de margem restante */}
      <div className="mt-4">
        <div className="mb-1 flex items-center justify-between text-xs font-semibold">
          <span className="text-gray-600">Margem restante</span>
          <span className={STATUS_TEXT[status]}>
            {marginPct.toFixed(1).replace('.', ',')}%
          </span>
        </div>
        <div
          className="h-3 w-full overflow-hidden rounded-full bg-gray-200"
          role="progressbar"
          aria-label="Margem restante"
          aria-valuenow={Math.round(marginPct)}
          aria-valuemin={-100}
          aria-valuemax={MARGIN_BAR_SCALE}
        >
          <div
            className={cn('h-full rounded-full transition-all duration-200', STATUS_BAR[status])}
            style={{ width: `${Math.max(fillPct, marginPct < 0 ? 100 : 2)}%` }}
          />
        </div>
        <p className={cn('mt-1 text-xs font-semibold', STATUS_TEXT[status])}>
          {STATUS_LABEL[status]}
          {status === 'warning' && ` · abaixo de ${HEALTHY_MARGIN_PCT}%`}
        </p>
      </div>

      {/* Avisos */}
      {belowCost && (
        <div
          role="alert"
          className="mt-3 flex items-start gap-2 rounded-lg border border-red-300 bg-red-50 p-3"
        >
          <TrendingDown className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
          <p className="text-sm font-semibold text-red-800">
            Este desconto vende abaixo do custo. Não é possível cobrar.
          </p>
        </div>
      )}

      {overCeiling && !belowCost && (
        <div
          role="alert"
          className="mt-3 flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3"
        >
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
          <p className="text-sm text-amber-900">
            Acima do seu teto de <strong>{ceilingPct}%</strong>. O pedido será enviado como{' '}
            <strong>pendente de aprovação do gerente</strong>.
          </p>
        </div>
      )}

      {!overCeiling && !belowCost && discountPct > 0 && (
        <p className="mt-3 flex items-center gap-2 text-xs font-medium text-green-700">
          <ShieldCheck className="h-4 w-4" aria-hidden="true" />
          Dentro do seu limite de desconto.
        </p>
      )}
    </div>
  )
}
