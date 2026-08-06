import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, CheckCircle2, QrCode, RefreshCw, Terminal, WifiOff } from 'lucide-react'
import { Button, Input } from '@/components/ui'
import { formatCurrency, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/cn'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'
import { isApiEnabled } from '@/lib/api'
import { usePayment } from '@/hooks/usePayment'
import {
  useBalcaoPendingCompletion,
  type UseBalcaoPendingCompletionResult,
} from '@/hooks/useBalcaoPendingCompletion'
import type { BalcaoApprovalOrder } from '@/hooks/useBalcaoApprovals'

/**
 * Tela de CONCLUSÃO da venda de balcão aprovada.
 *
 * Fecha o ciclo da venda acima do teto: o gerente aprovou, agora o operador
 * cobra e finaliza. Dois caminhos:
 *   - MAQUININHA — informa o NSU do comprovante (settle-external): o backend
 *     marca pago, baixa estoque e lança no livro.
 *   - PIX — gera o QR (mesmo motor de pagamento do resto); a confirmação cai
 *     sozinha e a venda sai da fila.
 */

type Mode = 'choose' | 'maquininha' | 'pix'

function CompletionCard({
  order,
  settling,
  onSettle,
  onPix,
  pix,
}: {
  order: BalcaoApprovalOrder
  settling: boolean
  onSettle: (nsu: string) => void
  onPix: () => void
  /** Estado do Pix quando ESTA venda é a que está sendo paga por Pix. */
  pix: {
    active: boolean
    qrCodeBase64?: string
    status?: string
    onSimulateConfirm?: () => void
  }
}) {
  const [mode, setMode] = useState<Mode>('choose')
  const [nsu, setNsu] = useState('')

  const pixPending = pix.active && pix.status === 'pending'
  const pixConfirmed = pix.active && pix.status === 'confirmed'

  return (
    <li className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="font-display text-base font-bold text-gray-900">
            {order.number || order.id}
          </p>
          <p className="truncate text-sm text-gray-700">{order.customerName ?? 'Sem cliente'}</p>
          <p className="mt-0.5 text-xs text-gray-400">{formatDateTime(order.createdAt)}</p>
        </div>
        <div className="text-right">
          <p className="font-display text-xl font-bold text-brand-blue">
            {formatCurrency(order.total)}
          </p>
          {order.discountAmount > 0 && (
            <p className="text-xs text-emerald-700">
              desconto {order.discountPct.toFixed(1).replace('.', ',')}% (
              {formatCurrency(order.discountAmount)}) — aprovado
            </p>
          )}
        </div>
      </div>

      <div className="mt-3 border-t border-gray-100 pt-3">
        {/* Pix em andamento nesta venda: QR + status. */}
        {pix.active ? (
          <div className="flex flex-col items-center gap-2">
            {pixConfirmed ? (
              <p className="flex items-center gap-2 font-semibold text-emerald-700">
                <CheckCircle2 className="h-5 w-5" aria-hidden="true" /> Pagamento confirmado
              </p>
            ) : (
              <>
                {pixPending && pix.qrCodeBase64 && (
                  <img
                    src={
                      pix.qrCodeBase64.startsWith('http')
                        ? pix.qrCodeBase64
                        : `data:image/png;base64,${pix.qrCodeBase64}`
                    }
                    alt="QR Code do Pix"
                    className="h-44 w-44 rounded-lg border border-gray-200"
                  />
                )}
                <p className="text-center text-sm text-gray-600">
                  Mostre o QR ao cliente. A confirmação aparece sozinha.
                </p>
                {pix.onSimulateConfirm && (
                  <button
                    type="button"
                    onClick={pix.onSimulateConfirm}
                    className="text-xs font-semibold text-brand-blue hover:underline"
                  >
                    (demo) simular confirmação
                  </button>
                )}
              </>
            )}
          </div>
        ) : mode === 'maquininha' ? (
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-[10rem] flex-1">
              <Input
                label="NSU do comprovante"
                value={nsu}
                inputMode="numeric"
                onChange={(e) => setNsu(e.target.value.replace(/\D/g, '').slice(0, 12))}
                placeholder="Ex: 004512890"
                className="h-11 text-base"
              />
            </div>
            <Button
              size="lg"
              loading={settling}
              disabled={!nsu.trim()}
              onClick={() => onSettle(nsu)}
              className="h-11"
            >
              Concluir
            </Button>
            <button
              type="button"
              onClick={() => setMode('choose')}
              className="pb-2 text-sm text-gray-500 hover:underline"
            >
              voltar
            </button>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2">
            <Button size="lg" onClick={() => setMode('maquininha')} className="h-11 flex-1">
              <Terminal className="h-5 w-5" aria-hidden="true" />
              Maquininha
            </Button>
            <Button size="lg" variant="secondary" onClick={onPix} className={cn('h-11 flex-1')}>
              <QrCode className="h-5 w-5" aria-hidden="true" />
              Pix
            </Button>
          </div>
        )}
      </div>
    </li>
  )
}

export default function BalcaoCompletionPage() {
  const completion: UseBalcaoPendingCompletionResult = useBalcaoPendingCompletion()
  const payment = usePayment()
  const [pixOrderId, setPixOrderId] = useState<string | null>(null)

  // Pix confirmado → a venda saiu de "pendente de pagamento": recarrega a fila.
  const pixStatus = payment.result?.status
  useEffect(() => {
    if (pixOrderId && pixStatus === 'confirmed') {
      completion.refetch()
      const t = setTimeout(() => {
        setPixOrderId(null)
        payment.stopPolling()
      }, 1500)
      return () => clearTimeout(t)
    }
    // completion/payment são estáveis o bastante; evitamos re-disparar por eles.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pixOrderId, pixStatus])

  const payPix = (order: BalcaoApprovalOrder) => {
    setPixOrderId(order.id)
    void payment.createPayment(order.id, 'pix', order.total, {
      payer_name: order.customerName,
      payer_cpf: (order.customerDocument ?? '').replace(/\D/g, ''),
      payer_phone: (order.customerPhone ?? '').replace(/\D/g, ''),
    })
  }

  return (
    <div className="flex h-screen flex-col bg-gray-50">
      <BalcaoTopBar />

      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl p-4 sm:p-6">
          <div className="mb-4 flex items-center justify-between">
            <Link
              to="/balcao"
              className="inline-flex items-center gap-1 text-sm font-semibold text-brand-blue hover:underline"
            >
              <ArrowLeft className="h-4 w-4" aria-hidden="true" />
              Voltar ao balcão
            </Link>
            <button
              type="button"
              onClick={completion.refetch}
              className="inline-flex items-center gap-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-xs font-semibold text-gray-600 hover:bg-gray-50"
            >
              <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
              Atualizar
            </button>
          </div>

          <h1 className="font-display text-2xl font-bold text-gray-900">Vendas aprovadas</h1>
          <p className="mt-1 text-sm text-gray-600">
            Descontos que o gerente homologou. Conclua a cobrança na maquininha ou no Pix.
          </p>

          {(completion.actionError || payment.error) && (
            <p role="alert" className="mt-3 text-sm font-semibold text-red-600">
              {completion.actionError || payment.error}
            </p>
          )}

          <div className="mt-5">
            {completion.isLoading ? (
              <p className="text-sm text-gray-500">Carregando…</p>
            ) : completion.isError ? (
              <p className="flex items-center gap-2 text-sm text-red-600">
                <WifiOff className="h-4 w-4" aria-hidden="true" />
                {completion.errorMessage || 'Não foi possível carregar as vendas aprovadas.'}
              </p>
            ) : completion.orders.length === 0 ? (
              <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-gray-300 bg-white p-8 text-center">
                <CheckCircle2 className="h-10 w-10 text-emerald-400" aria-hidden="true" />
                <p className="text-sm text-gray-600">Nenhuma venda aprovada aguardando cobrança.</p>
              </div>
            ) : (
              <ul className="flex flex-col gap-3">
                {completion.orders.map((order) => (
                  <CompletionCard
                    key={order.id}
                    order={order}
                    settling={completion.settlingId === order.id}
                    onSettle={(nsu) => void completion.settle(order.id, nsu)}
                    onPix={() => payPix(order)}
                    pix={{
                      active: pixOrderId === order.id,
                      qrCodeBase64: payment.result?.qrCodeBase64,
                      status: payment.result?.status,
                      onSimulateConfirm: !isApiEnabled ? payment.simulateConfirm : undefined,
                    }}
                  />
                ))}
              </ul>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
