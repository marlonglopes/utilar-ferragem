import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, Check, Lock, RefreshCw, ShieldAlert, WifiOff, X } from 'lucide-react'
import { Modal, Button, Input } from '@/components/ui'
import { formatCurrency, formatCNPJ, formatCPF, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/cn'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'
import { useBalcaoOperator } from '@/hooks/useBalcaoOperator'
import {
  useBalcaoApprovals,
  BLOCKED_MESSAGE,
  type BalcaoApprovalOrder,
} from '@/hooks/useBalcaoApprovals'
import { isCNPJ } from '@/store/balcaoStore'

/**
 * Fila de homologação de desconto — o outro lado do `requiresApproval` do PDV.
 *
 * A regra que dá forma a esta tela é a REGRA 3 do backend: ninguém aprova o
 * próprio desconto. Os pedidos que o próprio usuário vendeu APARECEM (ele
 * precisa ver que estão parados, e por quê), mas com os botões desabilitados e
 * o motivo escrito. Esconder o pedido faria o gerente-vendedor achar que a
 * venda sumiu; mostrar o botão e deixar o 403 explicar seria pior ainda.
 */

function maskDoc(value?: string): string {
  if (!value) return ''
  return isCNPJ(value) ? formatCNPJ(value) : formatCPF(value)
}

interface CardProps {
  order: BalcaoApprovalOrder
  blocked: ReturnType<ReturnType<typeof useBalcaoApprovals>['blockedReason']>
  busy: boolean
  onApprove: () => void
  onReject: () => void
}

function ApprovalCard({ order, blocked, busy, onApprove, onReject }: CardProps) {
  return (
    <li className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="font-display text-base font-bold text-gray-900">
            {order.number || order.id}
          </p>
          <p className="truncate text-sm text-gray-700">{order.customerName ?? 'Sem cliente'}</p>
          {order.customerDocument && (
            <p className="font-mono text-xs text-gray-500">{maskDoc(order.customerDocument)}</p>
          )}
          <p className="mt-0.5 text-xs text-gray-400">{formatDateTime(order.createdAt)}</p>
        </div>

        <div className="text-right">
          <p className="font-display text-xl font-bold text-brand-blue">
            {formatCurrency(order.total)}
          </p>
          <p className="text-xs text-gray-500 line-through">{formatCurrency(order.subtotal)}</p>
          <p className="mt-1 inline-block rounded-full bg-orange-100 px-2 py-0.5 text-xs font-bold text-brand-orange">
            −{order.discountPct.toFixed(1).replace('.', ',')}% ·{' '}
            {formatCurrency(order.discountAmount)}
          </p>
        </div>
      </div>

      {order.items.length > 0 && (
        <p className="mt-2 truncate text-xs text-gray-500">
          {order.items.length} {order.items.length === 1 ? 'item' : 'itens'} ·{' '}
          {order.items.map((i) => `${i.quantity}× ${i.name}`).join(', ')}
        </p>
      )}

      {blocked ? (
        <p
          role="note"
          className="mt-3 flex items-start gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm font-semibold text-gray-600"
        >
          <Lock className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          {BLOCKED_MESSAGE[blocked]}
        </p>
      ) : (
        <div className="mt-3 flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={onApprove}
            className="flex h-12 flex-1 items-center justify-center gap-2 rounded-lg bg-green-600 font-semibold text-white hover:bg-green-700 disabled:opacity-50"
          >
            <Check className="h-5 w-5" aria-hidden="true" />
            Aprovar
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={onReject}
            className="flex h-12 flex-1 items-center justify-center gap-2 rounded-lg border border-red-300 font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
          >
            <X className="h-5 w-5" aria-hidden="true" />
            Recusar
          </button>
        </div>
      )}

      {/* Os botões desabilitados já contam a regra; este bloco é o reforço para
          quem usa leitor de tela e não vê o estado do botão. */}
      {blocked === 'self_approval' && (
        <p className="sr-only">
          Aprovação bloqueada: separação de funções — quem vende não homologa.
        </p>
      )}
    </li>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen flex-col bg-gray-50">
      <BalcaoTopBar />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl p-4">
          <Link
            to="/balcao"
            className="mb-4 inline-flex h-12 items-center gap-2 text-sm font-semibold text-brand-blue hover:underline"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            Voltar ao PDV
          </Link>
          {children}
        </div>
      </div>
    </div>
  )
}

export default function BalcaoApprovalsPage() {
  const { operator, isLoading: loadingOperator } = useBalcaoOperator()
  const approvals = useBalcaoApprovals()
  const [rejecting, setRejecting] = useState<BalcaoApprovalOrder | null>(null)
  const [note, setNote] = useState('')
  const [noteError, setNoteError] = useState('')

  if (loadingOperator) {
    return (
      <Shell>
        <p className="text-sm text-gray-500">Carregando seu cargo…</p>
      </Shell>
    )
  }

  // Sem poder de homologação a tela inteira não faz sentido — e o backend
  // devolveria 403 em cada clique.
  if (!operator.canApproveDiscount) {
    return (
      <Shell>
        <div className="rounded-xl border border-gray-200 bg-white p-6 text-center">
          <ShieldAlert className="mx-auto h-10 w-10 text-brand-orange" aria-hidden="true" />
          <h1 className="mt-3 font-display text-xl font-bold text-gray-900">
            Fila de aprovação restrita
          </h1>
          <p className="mt-2 text-sm text-gray-600">
            Seu cargo não homologa descontos. Peça ao gerente da loja para aprovar as vendas
            pendentes.
          </p>
        </div>
      </Shell>
    )
  }

  async function confirmReject() {
    if (!rejecting) return
    // O backend exige justificativa na recusa (e está certo: "recusado" sem
    // motivo obriga o vendedor a voltar ao gerente para saber o que fazer).
    if (!note.trim()) {
      setNoteError('Informe o motivo da recusa.')
      return
    }
    await approvals.reject(rejecting.id, note.trim())
    setRejecting(null)
    setNote('')
    setNoteError('')
  }

  return (
    <Shell>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h1 className="font-display text-2xl font-bold text-gray-900">Aprovações pendentes</h1>
          <p className="text-sm text-gray-500">
            {operator.storeName || 'Sua loja'} · {approvals.orders.length}{' '}
            {approvals.orders.length === 1 ? 'pedido' : 'pedidos'}
          </p>
        </div>
        <button
          type="button"
          onClick={approvals.refetch}
          aria-label="Atualizar fila"
          className="flex h-12 w-12 items-center justify-center rounded-lg border border-gray-300 bg-white text-gray-600 hover:bg-gray-100"
        >
          <RefreshCw className={cn('h-5 w-5', approvals.isLoading && 'animate-spin')} aria-hidden="true" />
        </button>
      </div>

      {approvals.actionError && (
        <p role="alert" className="mb-3 rounded-lg bg-red-50 p-3 text-sm font-semibold text-red-700">
          {approvals.actionError}
        </p>
      )}

      {approvals.isLoading && <p className="text-sm text-gray-500">Carregando fila…</p>}

      {approvals.isError && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4"
        >
          <WifiOff className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
          <div>
            <p className="text-sm font-semibold text-red-800">Não foi possível carregar a fila.</p>
            <p className="text-xs text-red-700">
              {approvals.errorMessage || 'Verifique a conexão da loja.'}
            </p>
            <Button size="sm" className="mt-2 h-12" onClick={approvals.refetch}>
              Tentar de novo
            </Button>
          </div>
        </div>
      )}

      {!approvals.isLoading && !approvals.isError && approvals.orders.length === 0 && (
        <div className="rounded-xl border border-dashed border-gray-300 bg-white p-8 text-center">
          <Check className="mx-auto h-8 w-8 text-green-600" aria-hidden="true" />
          <p className="mt-2 font-semibold text-gray-800">Nenhum desconto pendente</p>
          <p className="text-sm text-gray-500">As vendas da loja estão todas dentro do teto.</p>
        </div>
      )}

      <ul className="flex flex-col gap-3">
        {approvals.orders.map((order) => (
          <ApprovalCard
            key={order.id}
            order={order}
            blocked={approvals.blockedReason(order)}
            busy={approvals.decidingId === order.id}
            onApprove={() => void approvals.approve(order.id)}
            onReject={() => {
              setRejecting(order)
              setNote('')
              setNoteError('')
            }}
          />
        ))}
      </ul>

      <Modal
        open={rejecting !== null}
        onClose={() => setRejecting(null)}
        title={`Recusar ${rejecting?.number ?? ''}`}
        size="sm"
      >
        <div className="flex flex-col gap-3">
          <Input
            label="Motivo da recusa"
            value={note}
            autoFocus
            onChange={(e) => setNote(e.target.value.slice(0, 500))}
            placeholder="Ex: desconto acima da política para este cliente"
            className="h-12 text-base"
          />
          {noteError && (
            <p role="alert" className="text-sm font-semibold text-red-600">
              {noteError}
            </p>
          )}
          <Button size="lg" fullWidth className="h-12" onClick={() => void confirmReject()}>
            Confirmar recusa
          </Button>
        </div>
      </Modal>
    </Shell>
  )
}
