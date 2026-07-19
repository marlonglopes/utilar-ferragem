import type { ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { Sparkline } from '@/components/admin/charts'
import {
  SEVERITY_DOT,
  SEVERITY_LABEL,
  SEVERITY_PILL,
  SEVERITY_TEXT,
  deltaSeverity,
} from '@/components/admin/tokens'
import { formatCents, formatDelta } from '@/lib/adminFormat'
import type { Severity } from '@/lib/adminTypes'

/**
 * Primitivas do painel admin.
 *
 * Não moram em `components/ui/` porque aquela pasta é de outro dono e serve o
 * e-commerce (componentes espaçosos, de conversão). Estas são o oposto:
 * densas, tabulares, feitas para leitura de varredura.
 */

// ---------------------------------------------------------------------------
// Dinheiro
// ---------------------------------------------------------------------------

/**
 * Todo valor monetário passa por aqui. `tabular-nums` alinha os dígitos em
 * coluna — sem isso, uma coluna de reais fica serrilhada e a comparação visual
 * entre linhas, que é o motivo de existir a tabela, deixa de funcionar.
 */
export function Money({
  cents,
  className,
  emphasis = false,
}: {
  cents: number
  className?: string
  emphasis?: boolean
}) {
  return (
    <span
      className={cn(
        'tabular-nums',
        emphasis ? 'font-semibold text-gray-900' : 'text-gray-700',
        cents < 0 && 'text-red-700',
        className,
      )}
    >
      {formatCents(cents)}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Severidade
// ---------------------------------------------------------------------------

/**
 * Chip de estado. O ponto colorido é o canal redundante: em preto e branco (o
 * contador vai imprimir) ou para daltônicos, a legenda textual continua ali.
 */
export function SeverityPill({
  severity,
  children,
  className,
}: {
  severity: Severity
  children?: ReactNode
  className?: string
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium',
        SEVERITY_PILL[severity],
        className,
      )}
    >
      <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', SEVERITY_DOT[severity])} aria-hidden="true" />
      {children ?? SEVERITY_LABEL[severity]}
    </span>
  )
}

/** Chip neutro, sem carga semântica — categoria, método, natureza. */
export function Chip({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium text-gray-600 ring-1 ring-inset ring-gray-200',
        className,
      )}
    >
      {children}
    </span>
  )
}

// ---------------------------------------------------------------------------
// Stat tile
// ---------------------------------------------------------------------------

export interface StatTileProps {
  label: string
  /** Valor já formatado. Números grandes usam figuras proporcionais, não tabulares. */
  value: string
  /** Variação relativa (fração) contra o período nomeado em `deltaLabel`. */
  delta?: number | null
  deltaLabel?: string
  /** Para taxa de erro e similares, subir é ruim. */
  higherIsBetter?: boolean
  /** Série de contexto — 12 a 30 pontos. */
  series?: number[]
  seriesColor?: string
  /** Severidade explícita, quando a métrica já traz o próprio julgamento. */
  severity?: Severity
  hint?: string
  className?: string
}

export function StatTile({
  label,
  value,
  delta,
  deltaLabel,
  higherIsBetter = true,
  series,
  seriesColor,
  severity,
  hint,
  className,
}: StatTileProps) {
  const ds = severity ?? deltaSeverity(delta ?? null, higherIsBetter)
  return (
    <div
      className={cn(
        'rounded-lg border border-gray-200 bg-white p-3 sm:p-4',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
        {severity && <SeverityPill severity={severity} />}
      </div>
      {/* Figuras proporcionais: `tabular-nums` num número de 30px fica solto. */}
      <p className="mt-1.5 font-display text-2xl font-bold leading-tight text-gray-900 sm:text-[1.75rem]">
        {value}
      </p>
      <div className="mt-1.5 flex items-end justify-between gap-3">
        <div className="min-w-0">
          {delta !== undefined && (
            <p className="truncate text-xs">
              {delta === null ? (
                <span className="text-gray-400">sem base de comparação</span>
              ) : (
                <>
                  <span className={cn('font-semibold tabular-nums', ds ? SEVERITY_TEXT[ds] : 'text-gray-500')}>
                    {formatDelta(delta)}
                  </span>{' '}
                  <span className="text-gray-500">{deltaLabel ?? 'vs. período anterior'}</span>
                </>
              )}
            </p>
          )}
          {hint && <p className="mt-0.5 truncate text-xs text-gray-500">{hint}</p>}
        </div>
        {series && series.length > 1 && (
          <Sparkline values={series} width={88} height={28} color={seriesColor} className="shrink-0" />
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Estrutura de seção
// ---------------------------------------------------------------------------

export function Section({
  title,
  description,
  actions,
  children,
  className,
}: {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('rounded-lg border border-gray-200 bg-white', className)}>
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 px-3 py-3 sm:px-4">
        <div className="min-w-0">
          <h2 className="font-display text-sm font-bold text-gray-900">{title}</h2>
          {description && <p className="mt-0.5 text-xs text-gray-500">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </header>
      {children}
    </section>
  )
}

/**
 * Contêiner de rolagem horizontal para tabela larga.
 *
 * A rolagem tem que ficar AQUI e nunca no `body`: se a página inteira rolar
 * para o lado, o cabeçalho e a navegação saem da tela e o painel fica
 * inutilizável no celular — que é justamente onde o dono vai abrir.
 */
export function ScrollArea({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn('w-full overflow-x-auto overscroll-x-contain', className)}>
      <div className="min-w-full">{children}</div>
    </div>
  )
}

export function EmptyState({ title, description }: { title: string; description?: string }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm font-medium text-gray-700">{title}</p>
      {description && <p className="mt-1 text-xs text-gray-500">{description}</p>}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm font-medium text-red-700">Não foi possível carregar</p>
      <p className="mt-1 text-xs text-gray-600">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 rounded border border-gray-300 px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
        >
          Tentar de novo
        </button>
      )}
    </div>
  )
}

/** Esqueleto de carregamento em linhas — mesma altura da tabela real. */
export function LoadingRows({ rows = 6 }: { rows?: number }) {
  return (
    <div className="divide-y divide-gray-100" aria-busy="true" aria-live="polite">
      <span className="sr-only">Carregando…</span>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 px-4 py-3">
          <div className="h-3 flex-1 animate-pulse rounded bg-gray-100" />
          <div className="h-3 w-16 animate-pulse rounded bg-gray-100" />
          <div className="h-3 w-24 animate-pulse rounded bg-gray-100" />
        </div>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Tabela
// ---------------------------------------------------------------------------

/** `<table>` densa padrão do painel. Numérico sempre à direita. */
export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return <table className={cn('w-full border-collapse text-sm', className)}>{children}</table>
}

export function Th({
  children,
  numeric = false,
  className,
  ...rest
}: { children: ReactNode; numeric?: boolean } & React.ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      scope="col"
      className={cn(
        'whitespace-nowrap border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-semibold uppercase tracking-wide text-gray-500',
        numeric ? 'text-right' : 'text-left',
        className,
      )}
      {...rest}
    >
      {children}
    </th>
  )
}

export function Td({
  children,
  numeric = false,
  className,
  ...rest
}: { children: ReactNode; numeric?: boolean } & React.TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td
      className={cn(
        'border-b border-gray-100 px-3 py-2 align-middle',
        numeric ? 'text-right tabular-nums' : 'text-left',
        className,
      )}
      {...rest}
    >
      {children}
    </td>
  )
}
