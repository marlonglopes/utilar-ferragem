import { cn } from '@/lib/cn'
import { PERIOD_PRESETS, type UseAdminPeriod } from '@/hooks/useAdminPeriod'

/**
 * Seletor de período. Fica numa linha única acima do conteúdo, conforme o guia
 * de interação — filtros nunca ficam espalhados entre os gráficos.
 *
 * Presets antes das datas soltas: 95% das aberturas do painel são "hoje" ou
 * "30 dias", e obrigar a preencher duas datas para isso é atrito puro.
 */
export function PeriodPicker({
  period,
  preset,
  setPreset,
  setPeriod,
  className,
}: UseAdminPeriod & { className?: string }) {
  return (
    <div className={cn('flex flex-wrap items-center gap-2', className)}>
      <div
        className="inline-flex overflow-hidden rounded-md border border-gray-300 bg-white"
        role="group"
        aria-label="Período"
      >
        {PERIOD_PRESETS.map((p) => (
          <button
            key={p.key}
            type="button"
            onClick={() => setPreset(p.key)}
            aria-pressed={preset === p.key}
            className={cn(
              'px-2.5 py-1.5 text-xs font-semibold transition-colors',
              'border-r border-gray-200 last:border-r-0',
              preset === p.key
                ? 'bg-brand-blue text-white'
                : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
            )}
          >
            {p.label}
          </button>
        ))}
      </div>

      <div className="flex items-center gap-1.5">
        <label className="sr-only" htmlFor="admin-de">
          Data inicial
        </label>
        <input
          id="admin-de"
          type="date"
          value={period.from}
          max={period.to}
          onChange={(e) => e.target.value && setPeriod({ ...period, from: e.target.value })}
          className="rounded-md border border-gray-300 px-2 py-1.5 text-xs tabular-nums text-gray-700 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
        />
        <span className="text-xs text-gray-400">até</span>
        <label className="sr-only" htmlFor="admin-ate">
          Data final
        </label>
        <input
          id="admin-ate"
          type="date"
          value={period.to}
          min={period.from}
          onChange={(e) => e.target.value && setPeriod({ ...period, to: e.target.value })}
          className="rounded-md border border-gray-300 px-2 py-1.5 text-xs tabular-nums text-gray-700 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
        />
      </div>
    </div>
  )
}
