import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { ArrowRight, Link2, Lock, ShieldCheck, ShieldX } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { PeriodPicker } from '@/components/admin/PeriodPicker'
import {
  Chip,
  EmptyState,
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { useAdminPeriod } from '@/hooks/useAdminPeriod'
import { useAdminAudit, useAdminChainVerification } from '@/hooks/useAdminQueries'
import { formatCount, shortHash } from '@/lib/adminFormat'
import { formatDateTime, formatRelativeTime } from '@/lib/format'
import type { AuditAction, AuditEvent } from '@/lib/adminTypes'

/**
 * Trilha de auditoria — quem fez o quê e quando.
 *
 * A tela toda gira em torno de uma promessa que precisa ficar explícita: o
 * registro é **imutável e encadeado por hash**. Cada evento carrega o hash do
 * anterior, então alterar uma linha antiga invalida toda a cadeia dali para a
 * frente — não dá para "corrigir" um preço no histórico sem que apareça.
 *
 * A verificação é feita NO SERVIDOR e a UI só mostra o veredito. Recalcular
 * SHA-256 no cliente seria teatro: o mesmo servidor que poderia mentir sobre a
 * validade mandaria os dados usados no cálculo.
 */

const ACTION_LABEL: Record<AuditAction, string> = {
  price_change: 'Mudança de preço',
  discount_approval: 'Aprovação de desconto',
  order_status_change: 'Status de pedido',
  admin_access: 'Acesso administrativo',
  refund_issued: 'Estorno emitido',
  user_role_change: 'Mudança de papel',
}

const ACTION_OPTIONS = [
  { value: '', label: 'Todas as ações' },
  ...Object.entries(ACTION_LABEL).map(([value, label]) => ({ value, label })),
]

const ENTITY_OPTIONS = [
  { value: '', label: 'Todas as entidades' },
  { value: 'produto', label: 'Produto' },
  { value: 'pedido', label: 'Pedido' },
  { value: 'pagamento', label: 'Pagamento' },
  { value: 'usuário', label: 'Usuário' },
  { value: 'painel', label: 'Painel' },
]

const PAGE_SIZE = 25

const selectCls =
  'rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-700 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

/** De → para. Sem valor (ex. acesso ao painel) mostra o traço, não some. */
function ValueDiff({ event }: { event: AuditEvent }) {
  if (event.fromValue === null && event.toValue === null) {
    return <span className="text-gray-300">—</span>
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5 text-xs">
      <span className="rounded bg-gray-100 px-1.5 py-0.5 tabular-nums text-gray-600 line-through decoration-gray-400">
        {event.fromValue ?? '—'}
      </span>
      <ArrowRight className="h-3 w-3 shrink-0 text-gray-400" aria-hidden="true" />
      <span className="rounded bg-brand-blue-light px-1.5 py-0.5 font-semibold tabular-nums text-brand-blue">
        {event.toValue ?? '—'}
      </span>
    </span>
  )
}

export default function AdminAuditTrailPage() {
  const periodCtl = useAdminPeriod('month')
  const { period } = periodCtl
  const [params, setParams] = useSearchParams()

  const action = params.get('acao') ?? ''
  const entityType = params.get('entidade') ?? ''
  const actorId = params.get('usuario') ?? ''
  const page = Math.max(1, Number(params.get('pagina') ?? '1') || 1)

  const setParam = useCallback(
    (key: string, value: string) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (value) sp.set(key, value)
          else sp.delete(key)
          if (key !== 'pagina') sp.delete('pagina')
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const chain = useAdminChainVerification(period)
  const events = useAdminAudit({
    ...period,
    action: action || undefined,
    entityType: entityType || undefined,
    actorId: actorId || undefined,
    page,
    pageSize: PAGE_SIZE,
  })

  const totalPages = events.data ? Math.max(1, Math.ceil(events.data.total / PAGE_SIZE)) : 1

  /** Lista de atores do recorte carregado — filtro sem uma chamada extra. */
  const actors = Array.from(
    new Map((events.data?.items ?? []).map((e) => [e.actorId, e.actorName])).entries(),
  )

  return (
    <AdminShell
      title="Trilha de auditoria"
      description="Registro imutável de quem alterou o quê, encadeado por hash."
      toolbar={<PeriodPicker {...periodCtl} />}
    >
      <div className="space-y-4">
        {/* Estado da cadeia — o bloco mais importante da tela. */}
        {chain.data && (
          <div
            className={
              chain.data.valid
                ? 'rounded-lg border border-gray-200 border-l-4 border-l-emerald-500 bg-white p-3 sm:p-4'
                : 'rounded-lg border border-gray-200 border-l-4 border-l-red-600 bg-red-50 p-3 sm:p-4'
            }
          >
            <div className="flex flex-wrap items-start gap-3">
              {chain.data.valid ? (
                <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-emerald-600" aria-hidden="true" />
              ) : (
                <ShieldX className="mt-0.5 h-5 w-5 shrink-0 text-red-700" aria-hidden="true" />
              )}
              <div className="min-w-0 flex-1">
                <p
                  className={
                    chain.data.valid
                      ? 'text-sm font-bold text-emerald-800'
                      : 'text-sm font-bold text-red-800'
                  }
                >
                  {chain.data.valid
                    ? 'Cadeia íntegra'
                    : `Cadeia rompida no registro #${chain.data.brokenAtSequence}`}
                </p>
                <p className="mt-0.5 text-xs leading-relaxed text-gray-600">
                  {formatCount(chain.data.checkedCount)} registros verificados pelo servidor{' '}
                  <time dateTime={chain.data.verifiedAt}>
                    {formatRelativeTime(chain.data.verifiedAt)}
                  </time>
                  . Cada registro guarda o hash SHA-256 do anterior: alterar ou remover qualquer
                  linha invalida todos os registros seguintes.{' '}
                  {chain.data.valid
                    ? 'Nenhuma adulteração detectada no período.'
                    : 'Trate como incidente de segurança e preserve o banco antes de qualquer escrita.'}
                </p>
                <p className="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-gray-500">
                  <Lock className="h-3 w-3" aria-hidden="true" />
                  <span>Hash âncora:</span>
                  <code className="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-gray-700">
                    {shortHash(chain.data.headHash)}
                  </code>
                </p>
              </div>
            </div>
          </div>
        )}

        {chain.isError && (
          <div className="rounded-lg border border-gray-200 border-l-4 border-l-amber-500 bg-white p-3">
            <p className="text-sm font-semibold text-amber-900">Verificação de integridade indisponível</p>
            <p className="mt-0.5 text-xs text-gray-600">
              Os eventos abaixo continuam sendo exibidos, mas sem confirmação de que a cadeia não foi
              adulterada. {(chain.error as Error)?.message}
            </p>
          </div>
        )}

        <Section
          title="Eventos"
          description="Mais recentes primeiro. A numeração da cadeia é contígua — um salto indica remoção."
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <label className="sr-only" htmlFor="audit-actor">Usuário</label>
              <select
                id="audit-actor"
                className={selectCls}
                value={actorId}
                onChange={(e) => setParam('usuario', e.target.value)}
              >
                <option value="">Todos os usuários</option>
                {actors.map(([id, name]) => (
                  <option key={id} value={id}>{name}</option>
                ))}
              </select>
              <label className="sr-only" htmlFor="audit-entity">Entidade</label>
              <select
                id="audit-entity"
                className={selectCls}
                value={entityType}
                onChange={(e) => setParam('entidade', e.target.value)}
              >
                {ENTITY_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              <label className="sr-only" htmlFor="audit-action">Ação</label>
              <select
                id="audit-action"
                className={selectCls}
                value={action}
                onChange={(e) => setParam('acao', e.target.value)}
              >
                {ACTION_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
            </div>
          }
        >
          {events.isError && (
            <ErrorState
              message={(events.error as Error)?.message ?? 'Erro desconhecido'}
              onRetry={() => void events.refetch()}
            />
          )}
          {events.isLoading && !events.data && <LoadingRows rows={10} />}
          {events.data && events.data.items.length === 0 && (
            <EmptyState title="Nenhum evento" description="Nenhum registro bate com os filtros." />
          )}
          {events.data && events.data.items.length > 0 && (
            <>
              <ScrollArea>
                <Table>
                  <thead>
                    <tr>
                      <Th numeric className="w-16">#</Th>
                      <Th>Quando</Th>
                      <Th>Quem</Th>
                      <Th>Ação</Th>
                      <Th>Entidade</Th>
                      <Th>De → para</Th>
                      <Th>Origem</Th>
                      <Th>Elo</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {events.data.items.map((e) => (
                      <tr key={e.id} className="hover:bg-gray-50">
                        <Td numeric className="font-mono text-xs text-gray-400">{e.sequence}</Td>
                        <Td className="whitespace-nowrap text-xs tabular-nums text-gray-600">
                          {formatDateTime(e.occurredAt)}
                        </Td>
                        <Td className="whitespace-nowrap">
                          <span className="font-medium text-gray-900">{e.actorName}</span>
                          <span className="ml-1.5 text-[11px] text-gray-400">{e.actorRole}</span>
                        </Td>
                        <Td><Chip>{ACTION_LABEL[e.action]}</Chip></Td>
                        <Td className="max-w-[16rem]">
                          <span className="block truncate text-gray-800">{e.entityLabel}</span>
                          <span className="block truncate font-mono text-[11px] text-gray-400">
                            {e.entityType} · {e.entityId}
                          </span>
                        </Td>
                        <Td><ValueDiff event={e} /></Td>
                        <Td className="whitespace-nowrap font-mono text-[11px] text-gray-500">{e.ip}</Td>
                        <Td>
                          {/* O elo é o que torna a promessa verificável — mostrá-lo
                              é o que separa "confie em nós" de "confira você". */}
                          <span
                            className="inline-flex items-center gap-1 font-mono text-[11px] text-gray-500"
                            title={`hash: ${e.hash}\nanterior: ${e.prevHash ?? '(gênese)'}`}
                          >
                            <Link2 className="h-3 w-3 text-gray-400" aria-hidden="true" />
                            {shortHash(e.hash)}
                          </span>
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </ScrollArea>

              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 px-3 py-2 text-xs">
                <span className="text-gray-500">
                  Página {page} de {totalPages} · {formatCount(events.data.total)} eventos
                </span>
                <div className="flex gap-1.5">
                  <button
                    type="button"
                    onClick={() => setParam('pagina', String(page - 1))}
                    disabled={page <= 1}
                    className="rounded border border-gray-300 px-2.5 py-1 font-semibold text-gray-700 disabled:opacity-40 enabled:hover:bg-gray-50"
                  >
                    Anterior
                  </button>
                  <button
                    type="button"
                    onClick={() => setParam('pagina', String(page + 1))}
                    disabled={page >= totalPages}
                    className="rounded border border-gray-300 px-2.5 py-1 font-semibold text-gray-700 disabled:opacity-40 enabled:hover:bg-gray-50"
                  >
                    Próxima
                  </button>
                </div>
              </div>
            </>
          )}
        </Section>

        <p className="px-1 text-[11px] leading-relaxed text-gray-500">
          O endereço IP é entregue com o último octeto mascarado pelo backend. Nada desta tela é
          gravado no navegador nem enviado para telemetria.
        </p>
      </div>
    </AdminShell>
  )
}
