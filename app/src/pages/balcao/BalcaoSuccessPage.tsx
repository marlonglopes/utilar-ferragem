import { useLocation, useNavigate } from 'react-router-dom'
import { CheckCircle2, Printer, Plus, AlertTriangle } from 'lucide-react'
import { Button } from '@/components/ui'
import { formatCurrency } from '@/lib/format'
import { BalcaoTopBar } from '@/components/balcao/BalcaoTopBar'

interface SaleSummary {
  orderId?: string
  orderNumber?: string
  method?: string
  total?: number
  requiresApproval?: boolean
  customerName?: string
  nsu?: string
}

const METHOD_LABEL: Record<string, string> = {
  pix: 'Pix',
  card: 'Cartão online',
  boleto: 'Boleto',
  external: 'Maquininha / externo',
}

/** Tela de venda concluída: resumo + caminho de volta para a próxima venda. */
export default function BalcaoSuccessPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const sale = (location.state ?? {}) as SaleSummary

  return (
    <div className="flex h-screen flex-col bg-gray-50">
      <BalcaoTopBar />

      <main className="flex flex-1 items-center justify-center overflow-y-auto p-6">
        <div className="w-full max-w-md rounded-2xl border border-gray-200 bg-white p-6 text-center shadow-sm">
          <CheckCircle2 className="mx-auto h-16 w-16 text-green-500" aria-hidden="true" />
          <h1 className="mt-3 font-display text-2xl font-bold text-gray-900">Venda concluída</h1>

          {sale.total != null && (
            <p className="mt-1 font-display text-3xl font-bold text-brand-blue">
              {formatCurrency(sale.total)}
            </p>
          )}

          <dl className="mt-5 space-y-2 text-left text-sm">
            {sale.customerName && (
              <div className="flex justify-between border-b border-gray-100 pb-2">
                <dt className="text-gray-500">Cliente</dt>
                <dd className="font-semibold text-gray-900">{sale.customerName}</dd>
              </div>
            )}
            {sale.method && (
              <div className="flex justify-between border-b border-gray-100 pb-2">
                <dt className="text-gray-500">Forma de pagamento</dt>
                <dd className="font-semibold text-gray-900">
                  {METHOD_LABEL[sale.method] ?? sale.method}
                </dd>
              </div>
            )}
            {sale.nsu && (
              <div className="flex justify-between border-b border-gray-100 pb-2">
                <dt className="text-gray-500">NSU</dt>
                <dd className="font-mono font-semibold text-gray-900">{sale.nsu}</dd>
              </div>
            )}
            {(sale.orderNumber || sale.orderId) && (
              <div className="flex justify-between border-b border-gray-100 pb-2">
                <dt className="text-gray-500">Pedido</dt>
                <dd className="font-mono text-xs font-semibold text-gray-900">
                  {sale.orderNumber ?? sale.orderId}
                </dd>
              </div>
            )}
          </dl>

          {sale.requiresApproval && (
            <div
              role="alert"
              className="mt-4 flex items-start gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-left"
            >
              <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" aria-hidden="true" />
              <p className="text-sm text-amber-900">
                Desconto acima do seu teto — o pedido ficou{' '}
                <strong>pendente de aprovação do gerente</strong>.
              </p>
            </div>
          )}

          <div className="mt-6 flex flex-col gap-2">
            <Button size="lg" fullWidth className="h-14" onClick={() => navigate('/balcao')}>
              <Plus className="h-5 w-5" aria-hidden="true" />
              Nova venda
            </Button>
            {/*
              TODO(backend/hardware): impressão de cupom não está implementada.
              window.print() imprime a página, não um cupom 80mm formatado, e não
              há cupom fiscal (NFC-e) — isso depende de integração fiscal.
            */}
            <Button
              size="lg"
              variant="secondary"
              fullWidth
              className="h-14"
              onClick={() => window.print()}
            >
              <Printer className="h-5 w-5" aria-hidden="true" />
              Imprimir comprovante
            </Button>
          </div>
        </div>
      </main>
    </div>
  )
}
