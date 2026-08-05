import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft, CheckCircle2, RefreshCw, Terminal, WifiOff } from 'lucide-react'
import { Button, Input } from '@/components/ui'
import { formatCurrency, formatDateTime } from '@/lib/format'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'
import {
  useBalcaoPendingCompletion,
  type UseBalcaoPendingCompletionResult,
} from '@/hooks/useBalcaoPendingCompletion'
import type { BalcaoApprovalOrder } from '@/hooks/useBalcaoApprovals'

/**
 * Tela de CONCLUSÃO da venda de balcão aprovada.
 *
 * Fecha o ciclo da venda acima do teto: o gerente aprovou, agora o operador
 * cobra na maquininha e informa o NSU — o backend marca pago, baixa estoque e
 * lança no livro. Sem esta tela, a venda ficava aprovada e sem onde finalizar.
 */
function CompletionCard({
  order,
  settling,
  onSettle,
}: {
  order: BalcaoApprovalOrder
  settling: boolean
  onSettle: (nsu: string) => void
}) {
  const [nsu, setNsu] = useState('')
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

      <div className="mt-3 flex flex-wrap items-end gap-3 border-t border-gray-100 pt-3">
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
          <Terminal className="h-5 w-5" aria-hidden="true" />
          Concluir na maquininha
        </Button>
      </div>
      <p className="mt-2 text-xs text-gray-500">
        Passe o cartão / receba na maquininha e informe o NSU. A venda é marcada como paga.
      </p>
    </li>
  )
}

export default function BalcaoCompletionPage() {
  const completion: UseBalcaoPendingCompletionResult = useBalcaoPendingCompletion()

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
            Descontos que o gerente homologou. Conclua a cobrança na maquininha.
          </p>

          {completion.actionError && (
            <p role="alert" className="mt-3 text-sm font-semibold text-red-600">
              {completion.actionError}
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
