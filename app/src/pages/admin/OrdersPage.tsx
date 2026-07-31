import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { CheckCircle2, Package, Search, Truck, XCircle } from 'lucide-react'
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
import { Reais } from '@/components/admin/products/productPrimitives'
import { cn } from '@/lib/cn'
import { useAdminOrders, useFulfillment } from '@/hooks/useAdminOrders'
import {
  isAdminOrdersEnabled,
  type AdminOrderQuery,
  type FulfillmentAction,
  type OrderStatus,
} from '@/lib/adminOrdersApi'

const PAGE_SIZE = 20

const STATUS_LABEL: Record<OrderStatus, string> = {
  pending_payment: 'Aguardando pgto',
  paid: 'Pago',
  picking: 'Em separação',
  shipped: 'Despachado',
  delivered: 'Entregue',
  cancelled: 'Cancelado',
}
const STATUS_TONE: Record<OrderStatus, string> = {
  pending_payment: 'bg-amber-100 text-amber-800',
  paid: 'bg-blue-100 text-blue-800',
  picking: 'bg-indigo-100 text-indigo-800',
  shipped: 'bg-violet-100 text-violet-800',
  delivered: 'bg-green-100 text-green-800',
  cancelled: 'bg-gray-200 text-gray-600',
}

// A próxima ação do FLUXO por status (pago→separar→despachar→entregar).
const NEXT: Partial<
  Record<OrderStatus, { action: FulfillmentAction; label: string; Icon: typeof Package }>
> = {
  paid: { action: 'picking', label: 'Separar', Icon: Package },
  picking: { action: 'shipped', label: 'Despachar', Icon: Truck },
  shipped: { action: 'delivered', label: 'Entregar', Icon: CheckCircle2 },
}
const CANCELLABLE: OrderStatus[] = ['pending_payment', 'paid', 'picking']

const STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'Todos' },
  { value: 'active', label: 'Em aberto' },
  { value: 'done', label: 'Finalizados' },
  { value: 'pending_payment', label: STATUS_LABEL.pending_payment },
  { value: 'paid', label: STATUS_LABEL.paid },
  { value: 'picking', label: STATUS_LABEL.picking },
  { value: 'shipped', label: STATUS_LABEL.shipped },
  { value: 'delivered', label: STATUS_LABEL.delivered },
  { value: 'cancelled', label: STATUS_LABEL.cancelled },
]

function OrderStatusPill({ status }: { status: OrderStatus }) {
  return (
    <span
      className={cn(
        'inline-flex rounded-full px-2 py-0.5 text-xs font-semibold',
        STATUS_TONE[status]
      )}
    >
      {STATUS_LABEL[status]}
    </span>
  )
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime())
    ? '—'
    : d.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: '2-digit' })
}

/**
 * Fila de operação de pedidos.
 *
 * PORQUÊ existe: os PATCH de separar/despachar/entregar/cancelar já existiam no
 * order-service, mas NADA listava os pedidos pra agir neles — a loja não
 * processava pedido online pelo painel. Esta é a porta dessas ações.
 */
export default function OrdersPage() {
  const [params, setParams] = useSearchParams()
  const statusParam = params.get('situacao') ?? ''
  const channel = params.get('canal') ?? ''
  const q = params.get('busca') ?? ''
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

  const query: AdminOrderQuery = useMemo(
    () => ({
      status: statusParam as AdminOrderQuery['status'],
      channel: channel as AdminOrderQuery['channel'],
      q,
      page,
      perPage: PAGE_SIZE,
    }),
    [statusParam, channel, q, page]
  )

  const { data, isLoading, isError, error, refetch } = useAdminOrders(query)
  const fulfill = useFulfillment()
  const rows = data?.data ?? []
  const meta = data?.meta

  const act = (id: string, action: FulfillmentAction) => {
    if (
      action === 'cancel' &&
      !window.confirm('Cancelar este pedido? A reserva de estoque é estornada.')
    ) {
      return
    }
    fulfill.mutate({ id, action })
  }

  const inputCls =
    'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

  return (
    <AdminShell
      title="Pedidos"
      description="A fila de operação da loja: pago → separar → despachar → entregar."
    >
      <div className="space-y-4">
        {!isAdminOrdersEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O order-service não está configurado (
            <code className="font-mono">VITE_ORDER_URL</code> vazio): os pedidos abaixo são{' '}
            <strong>inventados</strong> e as ações não gravam. Serve para conhecer a tela.
          </p>
        )}

        <Section title="Filtros">
          <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
            <div className="lg:col-span-2">
              <label htmlFor="of-q" className="block text-xs font-semibold text-gray-700">
                Buscar
              </label>
              <div className="relative mt-1">
                <Search
                  className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  aria-hidden="true"
                />
                <input
                  id="of-q"
                  type="search"
                  value={q}
                  onChange={(e) => setParam('busca', e.target.value)}
                  placeholder="Cliente, documento ou nº do pedido"
                  className={cn(inputCls, 'pl-8')}
                />
              </div>
            </div>

            <div>
              <label htmlFor="of-status" className="block text-xs font-semibold text-gray-700">
                Situação
              </label>
              <select
                id="of-status"
                value={statusParam}
                onChange={(e) => setParam('situacao', e.target.value)}
                className={cn(inputCls, 'mt-1')}
              >
                {STATUS_OPTIONS.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label htmlFor="of-canal" className="block text-xs font-semibold text-gray-700">
                Canal
              </label>
              <select
                id="of-canal"
                value={channel}
                onChange={(e) => setParam('canal', e.target.value)}
                className={cn(inputCls, 'mt-1')}
              >
                <option value="">Todos</option>
                <option value="web">Loja (web)</option>
                <option value="balcao">Balcão</option>
              </select>
            </div>
          </div>
        </Section>

        <Section title="Pedidos" description={meta ? `${meta.total} pedido(s)` : undefined}>
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
              <EmptyState
                title="Nenhum pedido"
                description="Ajuste os filtros ou aguarde uma nova venda."
              />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Nº</Th>
                    <Th>Situação</Th>
                    <Th>Canal</Th>
                    <Th>Cliente</Th>
                    <Th numeric>Total</Th>
                    <Th>Data</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((o) => {
                    const next = NEXT[o.status]
                    const canCancel = CANCELLABLE.includes(o.status)
                    return (
                      <tr key={o.id} className="hover:bg-gray-50">
                        <Td className="font-mono text-xs text-gray-500">{o.id.slice(0, 8)}</Td>
                        <Td>
                          <OrderStatusPill status={o.status} />
                        </Td>
                        <Td className="text-xs text-gray-600">
                          {o.channel === 'balcao' ? 'Balcão' : 'Loja'}
                        </Td>
                        <Td className="max-w-[16rem] truncate text-gray-800">
                          {o.customerName ?? '—'}
                        </Td>
                        <Td numeric>
                          <Reais value={o.total} className="font-semibold text-gray-900" />
                        </Td>
                        <Td className="whitespace-nowrap text-xs text-gray-600">
                          {formatDate(o.createdAt)}
                        </Td>
                        <Td className="text-right">
                          <div className="inline-flex items-center gap-1.5">
                            {next && (
                              <button
                                type="button"
                                onClick={() => act(o.id, next.action)}
                                disabled={fulfill.isPending}
                                className="inline-flex items-center gap-1 rounded-md bg-brand-blue px-2 py-1 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:opacity-50"
                              >
                                <next.Icon className="h-3.5 w-3.5" aria-hidden="true" />
                                {next.label}
                              </button>
                            )}
                            {canCancel && (
                              <button
                                type="button"
                                onClick={() => act(o.id, 'cancel')}
                                disabled={fulfill.isPending}
                                className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-2 py-1 text-xs font-semibold text-gray-600 hover:bg-gray-50 disabled:opacity-50"
                              >
                                <XCircle className="h-3.5 w-3.5" aria-hidden="true" />
                                Cancelar
                              </button>
                            )}
                            {!next && !canCancel && (
                              <span className="text-xs text-gray-400">—</span>
                            )}
                          </div>
                        </Td>
                      </tr>
                    )
                  })}
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
