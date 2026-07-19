import { useId, useMemo } from 'react'
import { cn } from '@/lib/cn'
import { CHART, SEVERITY_HEX } from '@/components/admin/tokens'
import type { Severity } from '@/lib/adminTypes'

/**
 * Gráficos do painel — SVG à mão, sem biblioteca.
 *
 * Recharts/Chart.js custariam 90–170 kB gzip para desenhar sparkline e barra,
 * que é tudo que este painel precisa. O bundle inteiro do app hoje é menor que
 * isso. Quando aparecer uma necessidade real de eixo duplo, zoom ou tooltip
 * complexo, aí sim vale reavaliar — não antes.
 *
 * Especificações seguidas (guia de dataviz):
 * - Linha 2px, junta/ponta arredondadas; área preenchida a ~10% da mesma cor.
 * - Barra ≤24px, topo arredondado 4px, base quadrada na linha de base.
 * - Grade recessiva, sólida, 1px — nunca tracejada.
 * - Último ponto destacado com anel na cor da superfície, para não sumir onde
 *   cruza a linha.
 * - Texto nunca usa a cor da série.
 */

// ---------------------------------------------------------------------------
// Sparkline
// ---------------------------------------------------------------------------

export interface SparklineProps {
  values: number[]
  /** Cor da linha. Padrão: azul da marca (identidade, não estado). */
  color?: string
  width?: number
  height?: number
  /** Preenche a área sob a linha. Desligue em linhas empilhadas na tabela. */
  filled?: boolean
  /** Destaca o último ponto — "onde estamos agora". */
  showLast?: boolean
  className?: string
  /** Descrição para leitor de tela. O gráfico é decorativo se ausente. */
  label?: string
}

export function Sparkline({
  values,
  color = CHART.primary,
  width = 120,
  height = 32,
  filled = true,
  showLast = true,
  className,
  label,
}: SparklineProps) {
  const gradId = useId()
  const geo = useMemo(() => {
    if (values.length === 0) return null
    const pad = 3 // deixa o anel do último ponto caber sem cortar
    const min = Math.min(...values)
    const max = Math.max(...values)
    // Série constante: uma linha reta no meio, em vez de divisão por zero.
    const span = max - min || 1
    const stepX = values.length > 1 ? (width - pad * 2) / (values.length - 1) : 0
    const pts = values.map((v, i) => {
      const x = pad + i * stepX
      const y = pad + (1 - (v - min) / span) * (height - pad * 2)
      return [x, y] as const
    })
    const line = pts.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`).join(' ')
    const area = `${line} L${pts[pts.length - 1][0].toFixed(2)},${height} L${pts[0][0].toFixed(2)},${height} Z`
    return { pts, line, area, last: pts[pts.length - 1] }
  }, [values, width, height])

  if (!geo) return null

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={cn('overflow-visible', className)}
      role={label ? 'img' : 'presentation'}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      {filled && (
        <>
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.18" />
              <stop offset="100%" stopColor={color} stopOpacity="0.02" />
            </linearGradient>
          </defs>
          <path d={geo.area} fill={`url(#${gradId})`} />
        </>
      )}
      <path
        d={geo.line}
        fill="none"
        stroke={color}
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {showLast && (
        <circle
          cx={geo.last[0]}
          cy={geo.last[1]}
          r={3.5}
          fill={color}
          stroke={CHART.surface}
          strokeWidth={2}
        />
      )}
    </svg>
  )
}

// ---------------------------------------------------------------------------
// Barras (série diária)
// ---------------------------------------------------------------------------

export interface BarPoint {
  label: string
  value: number
  /** Texto pronto para o tooltip — evita reformatar dentro do gráfico. */
  tooltip?: string
}

export interface BarSeriesProps {
  points: BarPoint[]
  height?: number
  color?: string
  className?: string
  /** Formata o rótulo do eixo Y (topo). Recebe o valor máximo. */
  formatMax?: (v: number) => string
  label?: string
}

/**
 * Colunas em CSS grid, não SVG: assim cada barra é um elemento real, ganha
 * `title` nativo (tooltip sem JS) e responde ao teclado sem reimplementar
 * hit-testing. Para até ~90 pontos é mais barato que um `<svg>` com listeners.
 */
export function BarSeries({
  points,
  height = 128,
  color = CHART.primary,
  className,
  formatMax,
  label,
}: BarSeriesProps) {
  const max = points.reduce((m, p) => Math.max(m, p.value), 0) || 1
  return (
    <figure className={cn('w-full', className)} aria-label={label}>
      <div className="relative" style={{ height }}>
        {/* Grade: 3 linhas sólidas hairline, recessivas. */}
        {[0, 0.5, 1].map((f) => (
          <div
            key={f}
            className="pointer-events-none absolute inset-x-0 border-t border-gray-200"
            style={{ top: `${f * 100}%` }}
            aria-hidden="true"
          />
        ))}
        {formatMax && (
          <span className="pointer-events-none absolute -top-1 right-0 bg-white pl-1 text-[10px] tabular-nums text-gray-400">
            {formatMax(max)}
          </span>
        )}
        {/* gap-[2px]: o espaçador de superfície que separa barras vizinhas. */}
        <div className="absolute inset-0 flex items-end gap-[2px]">
          {points.map((p) => (
            <div
              key={p.label}
              className="group relative min-w-0 flex-1"
              style={{ height: '100%' }}
            >
              <div
                className="absolute bottom-0 w-full rounded-t transition-opacity group-hover:opacity-80"
                style={{
                  height: `${Math.max((p.value / max) * 100, p.value > 0 ? 1.5 : 0)}%`,
                  backgroundColor: color,
                  maxWidth: 24,
                }}
                title={p.tooltip ?? `${p.label}: ${p.value}`}
              />
            </div>
          ))}
        </div>
      </div>
      {points.length > 1 && (
        <figcaption className="mt-1.5 flex justify-between text-[10px] tabular-nums text-gray-400">
          <span>{points[0].label}</span>
          <span>{points[points.length - 1].label}</span>
        </figcaption>
      )}
    </figure>
  )
}

// ---------------------------------------------------------------------------
// Medidor horizontal
// ---------------------------------------------------------------------------

export interface MeterProps {
  /** Fração 0..1. Valores fora do intervalo são presos nas bordas. */
  value: number
  severity?: Severity
  className?: string
  label?: string
}

/**
 * Barra de preenchimento única. O trilho é um passo claro da mesma cor, não
 * cinza — assim o estado se lê na barra inteira, não só na parte preenchida.
 */
export function Meter({ value, severity, className, label }: MeterProps) {
  const pct = Math.min(100, Math.max(0, value * 100))
  const fill = severity ? SEVERITY_HEX[severity] : CHART.primary
  return (
    <div
      className={cn('h-1.5 w-full overflow-hidden rounded-full', className)}
      style={{ backgroundColor: `${fill}22` }}
      role="meter"
      aria-valuenow={Math.round(pct)}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={label}
    >
      <div className="h-full rounded-full" style={{ width: `${pct}%`, backgroundColor: fill }} />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Barra empilhada (composição por status / método)
// ---------------------------------------------------------------------------

export interface StackSegment {
  key: string
  label: string
  value: number
  color: string
}

/**
 * Uma única barra horizontal dividida. Os segmentos são separados por 2px da
 * cor da superfície (o "surface gap") — nunca por borda desenhada, que
 * adicionaria tinta que não é dado.
 */
export function StackedBar({ segments, className }: { segments: StackSegment[]; className?: string }) {
  const total = segments.reduce((a, s) => a + s.value, 0) || 1
  return (
    <div className={cn('flex h-3 w-full gap-[2px] overflow-hidden rounded-full', className)}>
      {segments
        .filter((s) => s.value > 0)
        .map((s) => (
          <div
            key={s.key}
            style={{ width: `${(s.value / total) * 100}%`, backgroundColor: s.color }}
            title={`${s.label}: ${((s.value / total) * 100).toFixed(1)}%`}
          />
        ))}
    </div>
  )
}
