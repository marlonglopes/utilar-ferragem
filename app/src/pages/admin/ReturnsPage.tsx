import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AdminShell } from '@/components/admin/AdminShell'
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
import { cn } from '@/lib/cn'
import { useAuthStore } from '@/store/authStore'
import { useAdminReturns, useReturnAction } from '@/hooks/useAdminReturns'
import {
  isReturnsAdminEnabled,
  RETURN_STATUS_LABEL,
  type ReturnItem,
  type ReturnStatus,
} from '@/lib/adminReturnsApi'

const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

function fmtReais(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
}

const STATUS_CHIP: Record<ReturnStatus, string> = {
  requested: 'bg-amber-50 text-amber-800 ring-amber-600/20',
  approved: 'bg-blue-50 text-blue-700 ring-blue-600/20',
  in_transit: 'bg-blue-50 text-blue-700 ring-blue-600/20',
  received: 'bg-purple-50 text-purple-700 ring-purple-600/20',
  refunded: 'bg-emerald-50 text-emerald-700 ring-emerald-600/20',
  rejected: 'bg-gray-100 text-gray-600 ring-gray-500/20',
}

const FILTERS: { value: string; label: string }[] = [
  { value: '', label: 'Fila aberta' },
  { value: 'requested', label: 'Solicitadas' },
  { value: 'approved', label: 'Aprovadas' },
  { value: 'received', label: 'Recebidas' },
  { value: 'refunded', label: 'Estornadas' },
  { value: 'rejected', label: 'Recusadas' },
]

/**
 * Devoluções — a fila da loja (CDC). Endpoints já existiam; faltava a tela.
 *
 * O fluxo casa com as personas: quem confere a mercadoria física (receber) é o
 * almoxarife/vendas; quem ESTORNA (dinheiro saindo) é só o admin. O contador vê
 * a fila mas não age (read-only). A fronteira de verdade é o 403 do servidor;
 * aqui só escondemos o botão que ele não pode clicar.
 */
export default function ReturnsPage() {
  const [params, setParams] = useSearchParams()
  const status = params.get('status') ?? ''
  const role = useAuthStore((s) => s.user?.role)

  // Em dev/mock sem login, mostramos tudo (demo). Com persona logada, respeita.
  const readOnly = role === 'contador'
  const canRefund = !role || role === 'admin'

  const setStatus = (value: string) =>
    setParams(
      (prev) => {
        const sp = new URLSearchParams(prev)
        if (value) sp.set('status', value)
        else sp.delete('status')
        return sp
      },
      { replace: true }
    )

  const { data: rows = [], isLoading, isError, error, refetch } = useAdminReturns(status)

  return (
    <AdminShell
      title="Devoluções"
      description="Aprovar, receber a mercadoria e estornar — a fila de devolução da loja (CDC)."
    >
      <div className="space-y-4">
        {!isReturnsAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O order-service não está configurado (
            <code className="font-mono">VITE_ORDER_URL</code> vazio): a fila abaixo é inventada.
          </p>
        )}

        <div className="flex flex-wrap gap-1.5">
          {FILTERS.map((f) => (
            <button
              key={f.value}
              type="button"
              onClick={() => setStatus(f.value)}
              className={cn(
                'rounded-full px-3 py-1 text-xs font-semibold transition-colors',
                status === f.value
                  ? 'bg-brand-blue text-white'
                  : 'bg-white text-gray-600 ring-1 ring-inset ring-gray-300 hover:bg-gray-50'
              )}
            >
              {f.label}
            </button>
          ))}
        </div>

        <Section title="Fila" description={`${rows.length} devolução(ões)`}>
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={6} />
          ) : rows.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nada na fila" description="Nenhuma devolução neste filtro." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Pedido</Th>
                    <Th>Tipo</Th>
                    <Th>Situação</Th>
                    <Th numeric>Estorno</Th>
                    <Th>Pagamento</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <ReturnRow key={r.id} item={r} readOnly={readOnly} canRefund={canRefund} />
                  ))}
                </tbody>
              </Table>
            </ScrollArea>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}

function ReturnRow({
  item,
  readOnly,
  canRefund,
}: {
  item: ReturnItem
  readOnly: boolean
  canRefund: boolean
}) {
  const act = useReturnAction()
  const [rejecting, setRejecting] = useState(false)
  const [note, setNote] = useState('')
  const [noteError, setNoteError] = useState('')

  const run = (action: 'approve' | 'reject' | 'receive' | 'refund', reason?: string) =>
    act.mutate({ id: item.id, action, note: reason })

  const confirmReject = () => {
    if (!note.trim()) {
      setNoteError('Informe o motivo da recusa.')
      return
    }
    run('reject', note.trim())
    setRejecting(false)
    setNote('')
    setNoteError('')
  }

  return (
    <>
      <tr className="hover:bg-gray-50">
        <Td className="font-mono text-xs text-gray-600">{item.orderId.slice(0, 12)}</Td>
        <Td className="text-xs text-gray-700">
          {item.kind === 'defect' ? 'Defeito' : 'Arrependimento'}
        </Td>
        <Td>
          <Chip className={cn('ring-1 ring-inset', STATUS_CHIP[item.status])}>
            {RETURN_STATUS_LABEL[item.status]}
          </Chip>
        </Td>
        <Td numeric className="tabular-nums font-semibold text-gray-800">
          {fmtReais(item.refundTotal)}
        </Td>
        <Td className="text-xs text-gray-500">{item.paymentMethod}</Td>
        <Td className="text-right">
          {readOnly ? (
            <span className="text-xs text-gray-400">somente leitura</span>
          ) : (
            <div className="inline-flex flex-wrap justify-end gap-1.5">
              {item.status === 'requested' && (
                <>
                  <ActBtn onClick={() => run('approve')} disabled={act.isPending} tone="blue">
                    Aprovar
                  </ActBtn>
                  <ActBtn
                    onClick={() => setRejecting((v) => !v)}
                    disabled={act.isPending}
                    tone="ghost"
                  >
                    Recusar
                  </ActBtn>
                </>
              )}
              {(item.status === 'approved' || item.status === 'in_transit') && (
                <ActBtn onClick={() => run('receive')} disabled={act.isPending} tone="purple">
                  Receber mercadoria
                </ActBtn>
              )}
              {item.status === 'received' &&
                (canRefund ? (
                  <ActBtn onClick={() => run('refund')} disabled={act.isPending} tone="emerald">
                    Estornar
                  </ActBtn>
                ) : (
                  <span className="text-xs text-gray-400">estorno é do admin</span>
                ))}
              {['refunded', 'rejected'].includes(item.status) && (
                <span className="text-xs text-gray-400">encerrada</span>
              )}
            </div>
          )}
        </Td>
      </tr>
      {rejecting && (
        <tr className="bg-amber-50/40">
          <td colSpan={6} className="px-3 py-2 sm:px-4">
            <div className="flex flex-wrap items-center gap-2">
              <input
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder="Motivo da recusa (obrigatório)"
                className={cn(inputCls, 'max-w-md')}
              />
              <ActBtn onClick={confirmReject} disabled={act.isPending} tone="ghost">
                Confirmar recusa
              </ActBtn>
              {noteError && <span className="text-xs text-red-700">{noteError}</span>}
            </div>
          </td>
        </tr>
      )}
      {act.isError && (
        <tr>
          <td colSpan={6} className="px-3 py-1 text-xs text-red-700 sm:px-4">
            {act.error instanceof Error ? act.error.message : 'Falha na ação'}
          </td>
        </tr>
      )}
    </>
  )
}

function ActBtn({
  children,
  onClick,
  disabled,
  tone,
}: {
  children: React.ReactNode
  onClick: () => void
  disabled?: boolean
  tone: 'blue' | 'purple' | 'emerald' | 'ghost'
}) {
  const tones = {
    blue: 'bg-brand-blue text-white hover:bg-brand-blue/90',
    purple: 'bg-purple-600 text-white hover:bg-purple-700',
    emerald: 'bg-emerald-600 text-white hover:bg-emerald-700',
    ghost: 'border border-gray-300 text-gray-700 hover:bg-gray-50',
  }
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'rounded-md px-2.5 py-1 text-xs font-semibold transition-colors disabled:opacity-50',
        tones[tone]
      )}
    >
      {children}
    </button>
  )
}
