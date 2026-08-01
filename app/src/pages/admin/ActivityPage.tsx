import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import {
  EmptyState,
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { useAuditActivity } from '@/hooks/useCatalogAudit'
import {
  AUDIT_ACTION_LABEL,
  isAuditActivityEnabled,
  SOURCE_LABEL,
  type AuditQuery,
  type AuditSource,
  type FieldChange,
} from '@/lib/adminAuditApi'

const PAGE_SIZE = 30

function actionLabel(a: string): string {
  return AUDIT_ACTION_LABEL[a] ?? a
}

// Cor por serviço, pra bater o olho e saber de onde veio a linha na trilha
// unificada. Semântico, não é o laranja da marca.
const SOURCE_CHIP: Record<AuditSource, string> = {
  catalog: 'bg-blue-50 text-blue-700 ring-blue-600/20',
  staff: 'bg-purple-50 text-purple-700 ring-purple-600/20',
  operacao: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20',
}

function SourceChip({ source }: { source: AuditSource }) {
  return (
    <span
      className={cn(
        'inline-block rounded-full px-2 py-0.5 text-[11px] font-semibold ring-1 ring-inset',
        SOURCE_CHIP[source]
      )}
    >
      {SOURCE_LABEL[source]}
    </span>
  )
}

function fmtReais(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
}

function fmtValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

function formatWhen(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function ChangesCell({ changes }: { changes: Record<string, FieldChange> }) {
  const entries = Object.entries(changes ?? {})
  if (entries.length === 0) return <span className="text-xs text-gray-400">—</span>
  return (
    <div className="flex flex-col gap-0.5">
      {entries.map(([field, ch]) => (
        <span key={field} className="text-xs">
          <span className="font-medium text-gray-700">{field}</span>
          {': '}
          <span className="text-gray-400 line-through">{fmtValue(ch.old)}</span>
          {' → '}
          <span className="text-gray-800">{fmtValue(ch.new)}</span>
        </span>
      ))}
    </div>
  )
}

/**
 * Atividade / trilha CloudTrail: "quem fez o quê, quando".
 *
 * Lê a auditoria do catálogo (catalog_audit_log) — que já era gravada mas não
 * tinha tela. É a base da auditoria unificada: aqui entra o catálogo; order e
 * auth entram na sequência (mesmo formato).
 */
export default function ActivityPage() {
  const [params, setParams] = useSearchParams()
  const action = params.get('acao') ?? ''
  const actor = params.get('ator') ?? ''
  const source = (params.get('fonte') ?? '') as AuditSource | ''
  const page = Math.max(1, Number(params.get('pagina') ?? '1') || 1)

  const setParam = useCallback(
    (key: string, value: string) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (value) sp.set(key, value)
          else sp.delete(key)
          sp.delete('pagina')
          return sp
        },
        { replace: true }
      )
    },
    [setParams]
  )

  const goPage = useCallback(
    (p: number) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          sp.set('pagina', String(p))
          return sp
        },
        { replace: true }
      )
    },
    [setParams]
  )

  const query: AuditQuery = useMemo(
    () => ({ action, actor, source: source || undefined, page, perPage: PAGE_SIZE }),
    [action, actor, source, page]
  )

  const { data, isLoading, isError, error, refetch } = useAuditActivity(query)
  const rows = data?.data ?? []
  const meta = data?.meta

  const inputCls =
    'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

  return (
    <AdminShell
      title="Atividade"
      description="Quem fez o quê, quando — a trilha de auditoria da operação (imutável)."
    >
      <div className="space-y-4">
        {!isAuditActivityEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> Nenhum serviço está configurado (
            <code className="font-mono">VITE_*_URL</code> vazios): a trilha abaixo é{' '}
            <strong>inventada</strong>. Serve para conhecer a tela.
          </p>
        )}

        <Section title="Filtros">
          <div className="grid gap-3 p-3 sm:grid-cols-3 sm:p-4">
            <div>
              <label htmlFor="af-fonte" className="block text-xs font-semibold text-gray-700">
                Serviço
              </label>
              <select
                id="af-fonte"
                value={source}
                onChange={(e) => setParam('fonte', e.target.value)}
                className={cn(inputCls, 'mt-1')}
              >
                <option value="">Todos</option>
                {(Object.keys(SOURCE_LABEL) as AuditSource[]).map((s) => (
                  <option key={s} value={s}>
                    {SOURCE_LABEL[s]}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="af-acao" className="block text-xs font-semibold text-gray-700">
                Ação
              </label>
              <select
                id="af-acao"
                value={action}
                onChange={(e) => setParam('acao', e.target.value)}
                className={cn(inputCls, 'mt-1')}
              >
                <option value="">Todas</option>
                {Object.entries(AUDIT_ACTION_LABEL).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="af-ator" className="block text-xs font-semibold text-gray-700">
                Ator
              </label>
              <div className="relative mt-1">
                <Search
                  className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  aria-hidden="true"
                />
                <input
                  id="af-ator"
                  type="search"
                  value={actor}
                  onChange={(e) => setParam('ator', e.target.value)}
                  placeholder="Quem fez (e-mail ou id)"
                  className={cn(inputCls, 'pl-8')}
                />
              </div>
            </div>
          </div>
        </Section>

        <Section title="Trilha" description={meta ? `${meta.total} evento(s)` : undefined}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={8} />
          ) : rows.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nenhum evento" description="Ajuste os filtros." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Quando</Th>
                    <Th>Serviço</Th>
                    <Th>Ação</Th>
                    <Th>Ator</Th>
                    <Th>Item</Th>
                    <Th>Mudança (de → para)</Th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((e) => (
                    <tr key={`${e.source}:${e.id}`} className="hover:bg-gray-50">
                      <Td className="whitespace-nowrap text-xs text-gray-600">
                        {formatWhen(e.createdAt)}
                      </Td>
                      <Td>
                        <SourceChip source={e.source} />
                      </Td>
                      <Td className="whitespace-nowrap text-gray-800">
                        {actionLabel(e.action)}
                        {typeof e.amount === 'number' && e.amount > 0 && (
                          <span className="ml-1.5 text-xs font-semibold text-gray-500">
                            {fmtReais(e.amount)}
                          </span>
                        )}
                      </Td>
                      <Td className="max-w-[14rem] truncate text-xs text-gray-700">
                        {e.actorId ?? <span className="text-gray-400">sistema</span>}
                        {e.actorRole && <span className="ml-1 text-gray-400">· {e.actorRole}</span>}
                      </Td>
                      <Td className="font-mono text-xs text-gray-500">
                        {e.entityId ? e.entityId.slice(0, 8) : '—'}
                      </Td>
                      <Td>
                        <ChangesCell changes={e.changes} />
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </ScrollArea>
          )}

          {meta && meta.total_pages > 1 && (
            <div className="flex items-center justify-between gap-3 border-t border-gray-200 px-3 py-2 text-xs text-gray-600 sm:px-4">
              <span>
                Página {meta.page} de {meta.total_pages}
              </span>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => goPage(page - 1)}
                  disabled={page <= 1}
                  className="rounded-md border border-gray-300 px-2.5 py-1 font-semibold hover:bg-gray-50 disabled:opacity-40"
                >
                  Anterior
                </button>
                <button
                  type="button"
                  onClick={() => goPage(page + 1)}
                  disabled={page >= meta.total_pages}
                  className="rounded-md border border-gray-300 px-2.5 py-1 font-semibold hover:bg-gray-50 disabled:opacity-40"
                >
                  Próxima
                </button>
              </div>
            </div>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}
