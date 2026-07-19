import { useMemo, useState } from 'react'
import { AlertTriangle, ArrowRight, Info } from 'lucide-react'
import { cn } from '@/lib/cn'
import { ScrollArea, Table, Td, Th } from '@/components/admin/primitives'
import { SEVERITY_TEXT } from '@/components/admin/tokens'
import { formatCount } from '@/lib/adminFormat'
import {
  ACTION_CHIP,
  ACTION_LABEL,
  formatPriceDelta,
  formatReais,
  priceChangeSeverity,
  sortRowsByAttention,
  summaryCounters,
} from '@/lib/adminImportFormat'
import type { ImportAction, ImportPlan, ImportRow } from '@/lib/adminImportTypes'

/**
 * Passo 3 — o diff. É a tela mais importante do fluxo.
 *
 * Ela existe para pegar UM erro específico antes que ele chegue ao catálogo: o
 * separador decimal. `1.234,56` lido como `1,23` é o modo de falha mais caro da
 * ingestão, aparece como uma queda de ~99,9% e é invisível em qualquer resumo
 * agregado — só a linha, com de → para lado a lado, denuncia.
 *
 * Por isso o de/para de preço não é uma coluna qualquer: variação grande recebe
 * cor, ícone e o percentual explícito, e a linha inteira ganha fundo. E o número
 * da linha vem primeiro em fonte monoespaçada, porque é o que a pessoa digita no
 * "ir para" do Excel para achar a célula errada.
 */

const FILTERS: { id: ImportAction | 'all'; label: string }[] = [
  { id: 'all', label: 'Todas' },
  { id: 'reject', label: 'Rejeitadas' },
  { id: 'review', label: 'Retidas' },
  { id: 'update', label: 'Atualizações' },
  { id: 'create', label: 'Novas' },
]

function PriceDiff({ row }: { row: ImportRow }) {
  if (row.oldPrice === undefined && row.newPrice === undefined) {
    return <span className="text-gray-300">—</span>
  }
  if (row.oldPrice === undefined) {
    return <span className="tabular-nums text-gray-700">{formatReais(row.newPrice)}</span>
  }
  const severity = priceChangeSeverity(row.oldPrice, row.newPrice)
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs tabular-nums text-gray-600 line-through decoration-gray-400">
        {formatReais(row.oldPrice)}
      </span>
      <ArrowRight className="h-3 w-3 shrink-0 text-gray-400" aria-hidden="true" />
      <span
        className={cn(
          'rounded px-1.5 py-0.5 text-xs font-semibold tabular-nums',
          severity === 'critical' && 'bg-red-100 text-red-800',
          severity === 'warn' && 'bg-amber-100 text-amber-900',
          !severity && 'bg-gray-100 text-gray-800',
        )}
      >
        {formatReais(row.newPrice)}
      </span>
      {severity && (
        <span
          data-testid="price-alert"
          className={cn('inline-flex items-center gap-0.5 text-xs font-bold', SEVERITY_TEXT[severity])}
        >
          <AlertTriangle className="h-3 w-3" aria-hidden="true" />
          {formatPriceDelta(row.oldPrice, row.newPrice)}
        </span>
      )}
    </span>
  )
}

export function ImportDiff({ plan }: { plan: ImportPlan }) {
  const [filter, setFilter] = useState<ImportAction | 'all'>('all')

  const counters = summaryCounters(plan.summary)
  const rows = useMemo(() => {
    const sorted = sortRowsByAttention(plan.rows ?? [])
    return filter === 'all' ? sorted : sorted.filter((r) => r.action === filter)
  }, [plan.rows, filter])

  const truncated = plan.summary.total > (plan.rows?.length ?? 0)

  return (
    <div className="space-y-4">
      {/* Contadores. O que vai acontecer, em números, antes de qualquer escrita. */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
        {counters.map((c) => (
          <div
            key={c.key}
            data-testid={`counter-${c.key}`}
            className={cn(
              'rounded-lg border border-gray-200 bg-white p-3',
              c.severity === 'critical' && 'border-l-4 border-l-red-600',
              c.severity === 'warn' && 'border-l-4 border-l-amber-500',
              c.severity === 'ok' && 'border-l-4 border-l-emerald-500',
            )}
          >
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{c.label}</p>
            <p
              className={cn(
                'mt-0.5 font-display text-2xl font-bold tabular-nums leading-tight',
                c.value === 0 ? 'text-gray-300' : c.severity ? SEVERITY_TEXT[c.severity] : 'text-gray-900',
              )}
            >
              {formatCount(c.value)}
            </p>
            <p className="mt-0.5 text-[11px] leading-tight text-gray-500">{c.hint}</p>
          </div>
        ))}
      </div>

      {/* Nada foi escrito ainda — dito com todas as letras, não implícito. */}
      <div className="flex items-start gap-2 rounded-md border border-gray-200 border-l-4 border-l-brand-blue bg-white p-3">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-brand-blue" aria-hidden="true" />
        <p className="text-xs leading-relaxed text-gray-700">
          <strong>Isto é uma simulação.</strong> Nenhum produto foi criado ou alterado até aqui —{' '}
          {formatCount(plan.summary.total)} linhas foram lidas e conferidas contra o catálogo. A
          escrita só acontece quando você aprovar no passo seguinte.
        </p>
      </div>

      {plan.warnings && plan.warnings.length > 0 && (
        <ul className="space-y-1.5">
          {plan.warnings.map((w, i) => (
            <li
              key={i}
              className="flex items-start gap-2 rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-2.5 text-xs leading-relaxed text-gray-700"
            >
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-700" aria-hidden="true" />
              {w}
            </li>
          ))}
        </ul>
      )}

      <div className="rounded-lg border border-gray-200 bg-white">
        <div className="flex flex-wrap items-center gap-1.5 border-b border-gray-200 px-3 py-2">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              onClick={() => setFilter(f.id)}
              aria-pressed={filter === f.id}
              className={cn(
                'rounded-md px-2.5 py-1 text-xs font-semibold transition-colors',
                filter === f.id
                  ? 'bg-brand-blue text-white'
                  : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900',
              )}
            >
              {f.label}
            </button>
          ))}
          <span className="ml-auto text-xs text-gray-500">
            {formatCount(rows.length)} linhas exibidas
          </span>
        </div>

        {rows.length === 0 ? (
          <p className="px-4 py-10 text-center text-sm text-gray-500">
            Nenhuma linha com esta classificação.
          </p>
        ) : (
          <ScrollArea>
            <Table>
              <thead>
                <tr>
                  <Th numeric className="w-20">Linha</Th>
                  <Th className="w-28">Ação</Th>
                  <Th>SKU</Th>
                  <Th>Produto</Th>
                  <Th>Preço (de → para)</Th>
                  <Th>Motivo</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => {
                  const issues = [...(r.errors ?? []), ...(r.warnings ?? [])]
                  return (
                    <tr
                      key={r.rowNumber}
                      data-testid={`diff-row-${r.rowNumber}`}
                      className={cn(
                        r.action === 'reject' && 'bg-red-50/60',
                        r.action === 'review' && 'bg-amber-50/50',
                      )}
                    >
                      {/* O número DA PLANILHA — é assim que se acha o erro no Excel. */}
                      <Td numeric className="font-mono text-xs font-semibold text-gray-500">
                        {r.rowNumber}
                      </Td>
                      <Td>
                        <span
                          className={cn(
                            'inline-flex rounded-full px-2 py-0.5 text-[11px] font-semibold',
                            ACTION_CHIP[r.action],
                          )}
                        >
                          {ACTION_LABEL[r.action]}
                        </span>
                      </Td>
                      <Td className="whitespace-nowrap font-mono text-xs text-gray-700">
                        {r.sku || <span className="text-red-600">(vazio)</span>}
                      </Td>
                      <Td className="max-w-[18rem]">
                        <span className="block truncate text-gray-800">
                          {String(r.mapped?.name ?? r.raw?.name ?? '—')}
                        </span>
                      </Td>
                      <Td><PriceDiff row={r} /></Td>
                      <Td className="max-w-[22rem]">
                        {issues.length === 0 ? (
                          <span className="text-gray-300">—</span>
                        ) : (
                          <ul className="space-y-0.5">
                            {issues.map((e, i) => (
                              <li
                                key={i}
                                className={cn(
                                  'text-xs leading-snug',
                                  r.action === 'reject' ? 'text-red-800' : 'text-amber-900',
                                )}
                              >
                                {e.field && (
                                  <span className="font-mono font-semibold">{e.field}: </span>
                                )}
                                {e.message}
                              </li>
                            ))}
                          </ul>
                        )}
                      </Td>
                    </tr>
                  )
                })}
              </tbody>
            </Table>
          </ScrollArea>
        )}

        {truncated && (
          <p className="border-t border-gray-200 px-3 py-2 text-xs text-gray-500">
            Mostrando as {formatCount(plan.rows.length)} linhas que exigem atenção, de{' '}
            {formatCount(plan.summary.total)} no total. As demais foram conferidas e seguem sem
            pendência.
          </p>
        )}
      </div>
    </div>
  )
}

export default ImportDiff
