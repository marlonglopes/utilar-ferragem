import { useState } from 'react'
import { QrCode, Terminal, Check, CreditCard } from 'lucide-react'
import { Modal, Button, Input } from '@/components/ui'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import CardPayment from '@/pages/checkout/CardPayment'
import type { BalcaoPaymentMethod } from '@/hooks/useBalcaoCheckout'
import type { BalcaoPricing } from '@/store/balcaoStore'
import type { PaymentResult } from '@/hooks/usePayment'

// "Cartão" = cartão DIGITADO. O vendedor anda pelo corredor com o tablet, LONGE
// da maquininha, e digita o cartão — igual ao checkout web. Reaproveita o MESMO
// componente da web (CardPayment) e o mesmo pipeline usePayment→order→payment;
// nada de pagamento é reimplementado aqui. Em dev/mock já funciona (Stripe test /
// "simular confirmação"). Em produção com Appmax, o passo que falta é a
// tokenização no browser (Appmax JS) — o MESMO bloqueio do checkout web; quando
// entrar lá, o balcão herda sem mudança. "Maquininha" (external) segue existindo
// para dinheiro / POS físico, que não passa pela Appmax.
//
// SEM boleto: venda de balcão é presencial e à vista — boleto é "pagar depois",
// não faz sentido no caixa (entregaria a mercadoria antes de o pagamento cair).
// O tipo BalcaoPaymentMethod ainda aceita 'boleto' (herdado da web), mas o PDV
// simplesmente não o oferece.
const METHODS: Array<{
  id: BalcaoPaymentMethod
  label: string
  hint: string
  icon: typeof QrCode
}> = [
  { id: 'pix', label: 'Pix', hint: 'QR na tela', icon: QrCode },
  { id: 'card', label: 'Cartão', hint: 'Crédito — digitado', icon: CreditCard },
  { id: 'external', label: 'Maquininha', hint: 'Dinheiro / POS', icon: Terminal },
]

export interface ChargeModalProps {
  open: boolean
  onClose: () => void
  pricing: BalcaoPricing
  submitting: boolean
  error: string
  paymentResult: PaymentResult | null
  /** Nome pra pré-preencher "Nome no cartão" — o do CLIENTE da comanda (o titular
   * do cartão, não o operador). Editável no form. */
  cardholderName?: string
  onConfirm: (method: BalcaoPaymentMethod, nsu?: string) => void
  onDone: () => void
  /** Marca o pagamento de cartão como confirmado (chamado pelo CardPayment). */
  onCardConfirmed?: () => void
  /** Só em mock/demo: simula a confirmação do cartão (sem PSP real). */
  onSimulateConfirm?: () => void
}

/**
 * Seleção da forma de cobrança.
 *
 * Pix/cartão/boleto vão inteiros para o `usePayment` — nada de pagamento é
 * reimplementado aqui. "Maquininha" é o caso do balcão: a transação roda numa
 * POS física e o PDV só registra o NSU do comprovante.
 */
export function ChargeModal({
  open,
  onClose,
  pricing,
  submitting,
  error,
  paymentResult,
  cardholderName,
  onConfirm,
  onDone,
  onCardConfirmed,
  onSimulateConfirm,
}: ChargeModalProps) {
  const [method, setMethod] = useState<BalcaoPaymentMethod>('pix')
  const [nsu, setNsu] = useState('')
  const [cardError, setCardError] = useState('')

  const confirmed = paymentResult?.status === 'confirmed'
  const pixPending = paymentResult?.method === 'pix' && paymentResult.status === 'pending'
  // Cartão digitado: enquanto não confirma, mostramos o formulário (CardPayment).
  const cardForm = paymentResult?.method === 'card' && !confirmed

  return (
    <Modal open={open} onClose={onClose} title="Cobrar" size="md">
      <div className="flex flex-col gap-4">
        <div className="rounded-xl bg-brand-blue-light p-4 text-center">
          <p className="text-xs font-semibold uppercase tracking-wide text-brand-blue">
            Total a cobrar
          </p>
          <p className="font-display text-3xl font-bold text-brand-blue">
            {formatCurrency(pricing.total)}
          </p>
          {pricing.requiresApproval && (
            <p className="mt-1 text-xs font-semibold text-amber-700">
              Pendente de aprovação do gerente
            </p>
          )}
        </div>

        {!paymentResult && (
          <>
            <div className="grid grid-cols-2 gap-2">
              {METHODS.map((m) => {
                const Icon = m.icon
                const active = method === m.id
                return (
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => setMethod(m.id)}
                    aria-pressed={active}
                    className={cn(
                      'flex min-h-[72px] flex-col items-center justify-center gap-1 rounded-xl border-2 px-3 transition-colors',
                      active
                        ? 'border-brand-orange bg-orange-50'
                        : 'border-gray-200 bg-white hover:bg-gray-50'
                    )}
                  >
                    <Icon
                      className={cn('h-5 w-5', active ? 'text-brand-orange' : 'text-gray-500')}
                      aria-hidden="true"
                    />
                    <span className="text-sm font-semibold text-gray-900">{m.label}</span>
                    <span className="text-[11px] text-gray-500">{m.hint}</span>
                  </button>
                )
              })}
            </div>

            {method === 'external' && (
              <div>
                <Input
                  label="NSU do comprovante"
                  value={nsu}
                  inputMode="numeric"
                  onChange={(e) => setNsu(e.target.value.replace(/\D/g, '').slice(0, 12))}
                  placeholder="Ex: 004512890"
                  className="h-12 text-base"
                />
                <p className="mt-1 text-xs text-gray-500">
                  Passe o cartão/receba o dinheiro na maquininha e informe o NSU do comprovante. A
                  venda é marcada como paga, baixa o estoque e entra no livro.
                </p>
              </div>
            )}

            {error && (
              <p role="alert" className="text-sm font-semibold text-red-600">
                {error}
              </p>
            )}

            <Button
              size="lg"
              fullWidth
              loading={submitting}
              onClick={() => onConfirm(method, nsu)}
              className="h-14"
            >
              Confirmar cobrança
            </Button>
          </>
        )}

        {paymentResult && (
          <div className="flex flex-col gap-3">
            {cardForm ? (
              // Cartão digitado — MESMO componente da web (Stripe Elements em dev;
              // "simular" em mock). O PAN nunca passa pelo nosso backend.
              <CardPayment
                result={paymentResult}
                amount={pricing.total}
                defaultCardholderName={cardholderName}
                onConfirmed={() => onCardConfirmed?.()}
                onFailed={setCardError}
                onSimulateConfirm={onSimulateConfirm}
              />
            ) : (
              <>
                {pixPending && paymentResult.qrCodeBase64 && (
                  <img
                    src={
                      paymentResult.qrCodeBase64.startsWith('http')
                        ? paymentResult.qrCodeBase64
                        : `data:image/png;base64,${paymentResult.qrCodeBase64}`
                    }
                    alt="QR Code do Pix"
                    className="mx-auto h-48 w-48 rounded-lg border border-gray-200"
                  />
                )}
                {pixPending && (
                  <p className="text-center text-sm text-gray-600">
                    Mostre o QR ao cliente. A confirmação aparece sozinha.
                  </p>
                )}
                {paymentResult.barCode && (
                  <p className="break-all rounded-lg bg-gray-50 p-3 font-mono text-xs text-gray-700">
                    {paymentResult.barCode}
                  </p>
                )}
              </>
            )}

            {cardError && (
              <p role="alert" className="text-sm font-semibold text-red-600">
                {cardError}
              </p>
            )}

            {confirmed && (
              <p className="flex items-center justify-center gap-2 font-semibold text-green-700">
                <Check className="h-5 w-5" aria-hidden="true" />
                Pagamento confirmado
              </p>
            )}

            {/* Cartão em aberto NÃO oferece "concluir mesmo assim" — evita fechar a
                venda sem o cartão ter sido aprovado. Pix/maquininha seguem podendo. */}
            {(confirmed || !cardForm) && (
              <Button size="lg" fullWidth onClick={onDone} className="h-14">
                {confirmed ? 'Concluir venda' : 'Concluir mesmo assim'}
              </Button>
            )}
          </div>
        )}
      </div>
    </Modal>
  )
}
