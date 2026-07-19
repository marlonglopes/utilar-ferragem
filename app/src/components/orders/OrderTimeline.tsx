import { useTranslation } from 'react-i18next'
import { Check, X, Truck, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/cn'
import type { Order, OrderStatus } from '@/lib/mockOrders'

/**
 * Linha do tempo do pedido — "cadê meu pedido?" respondido sem abrir chamado.
 *
 * Antes, esta tela mostrava só o status como uma palavra ("Enviado") e uma
 * régua de bolinhas derivada do índice do status. Quem quisesse saber QUANDO o
 * pedido foi separado, ou se já saiu pra entrega ontem ou há uma semana, não
 * tinha onde olhar.
 *
 * Vertical em qualquer largura, de propósito: as datas não cabem embaixo das
 * bolinhas num celular de 360px sem virar sopa de letrinhas, e a leitura de
 * cima pra baixo é a mesma ordem em que o leitor de tela vai narrar.
 */

type StepKey = 'placed' | 'paid' | 'picking' | 'shipped' | 'delivered'

interface Step {
  key: StepKey
  /** ISO da data em que a etapa aconteceu, se o backend informou. */
  at?: string
}

/** Ordem canônica das etapas e o status do pedido a partir do qual cada uma já aconteceu. */
const STEP_ORDER: { key: StepKey; reachedAt: OrderStatus[] }[] = [
  { key: 'placed', reachedAt: ['pending_payment', 'paid', 'picking', 'shipped', 'delivered'] },
  { key: 'paid', reachedAt: ['paid', 'picking', 'shipped', 'delivered'] },
  { key: 'picking', reachedAt: ['picking', 'shipped', 'delivered'] },
  { key: 'shipped', reachedAt: ['shipped', 'delivered'] },
  { key: 'delivered', reachedAt: ['delivered'] },
]

function stepsOf(order: Order): Step[] {
  const at: Record<StepKey, string | undefined> = {
    placed: order.createdAt,
    paid: order.paidAt,
    picking: order.pickedAt,
    shipped: order.shippedAt,
    delivered: order.deliveredAt,
  }
  return STEP_ORDER.map(({ key }) => ({ key, at: at[key] }))
}

function formatStamp(iso: string): string {
  return new Date(iso).toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

type StepState = 'done' | 'current' | 'pending'

export function OrderTimeline({ order, className }: { order: Order; className?: string }) {
  const { t } = useTranslation()

  if (order.status === 'cancelled') {
    const when = order.cancelledAt ?? order.updatedAt
    return (
      <div
        className={cn(
          'flex items-start gap-2 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-700',
          className
        )}
      >
        <X className="mt-0.5 h-4 w-4 flex-shrink-0" aria-hidden />
        <div>
          <p className="font-semibold">{t('orders.timeline.cancelled')}</p>
          {when && <p className="mt-0.5 text-xs text-red-600">{formatStamp(when)}</p>}
        </div>
      </div>
    )
  }

  const steps = stepsOf(order)
  const currentIdx = STEP_ORDER.reduce(
    (acc, s, idx) => (s.reachedAt.includes(order.status) ? idx : acc),
    0
  )

  function stateOf(idx: number): StepState {
    if (idx < currentIdx) return 'done'
    if (idx === currentIdx) return 'current'
    return 'pending'
  }

  return (
    <div className={className}>
      {/*
        <ol> e não <div>: é uma sequência ordenada de fatos, e o leitor de tela
        anuncia "lista com 5 itens, item 3 de 5" de graça. Cada item carrega o
        estado em TEXTO (concluído / etapa atual / ainda não iniciado) num span
        sr-only — a cor da bolinha não existe pra quem não enxerga.
      */}
      <ol aria-label={t('orders.timeline.label')} className="flex flex-col">
        {steps.map((step, idx) => {
          const state = stateOf(idx)
          const isLast = idx === steps.length - 1
          const stateLabel = t(`orders.timeline.${state}`)

          return (
            <li
              key={step.key}
              // aria-current="step" marca onde o pedido está AGORA — é como o
              // leitor de tela responde "em que pé está" sem ler a lista toda.
              aria-current={state === 'current' ? 'step' : undefined}
              className="flex gap-3"
            >
              {/* Trilho: bolinha + linha de ligação */}
              <div className="flex flex-col items-center" aria-hidden>
                <div
                  className={cn(
                    'flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full',
                    state === 'done' && 'bg-green-500',
                    state === 'current' && 'bg-brand-orange ring-4 ring-brand-orange/15',
                    state === 'pending' && 'border-2 border-gray-200 bg-white'
                  )}
                >
                  {state === 'done' ? (
                    <Check className="h-4 w-4 text-white" />
                  ) : state === 'current' ? (
                    <div className="h-2.5 w-2.5 rounded-full bg-white" />
                  ) : (
                    <div className="h-2 w-2 rounded-full bg-gray-200" />
                  )}
                </div>
                {!isLast && (
                  <div
                    className={cn(
                      'w-0.5 flex-1 min-h-[28px]',
                      idx < currentIdx ? 'bg-green-400' : 'bg-gray-200'
                    )}
                  />
                )}
              </div>

              {/* Conteúdo da etapa */}
              <div className={cn('min-w-0 flex-1', isLast ? 'pb-0' : 'pb-5')}>
                <p
                  className={cn(
                    'text-sm leading-tight',
                    state === 'current' && 'font-semibold text-brand-orange',
                    state === 'done' && 'font-medium text-gray-900',
                    state === 'pending' && 'text-gray-400'
                  )}
                >
                  {t(`orders.timeline.${step.key}`)}
                  <span className="sr-only"> — {stateLabel}</span>
                </p>

                {/*
                  Data por etapa. Um pedido pode chegar sem o carimbo de uma
                  etapa que comprovadamente já passou (importação de pedido
                  antigo, webhook perdido, migração). Nesse caso dizemos "data
                  não informada" em vez de esconder a etapa: sumir com o passo
                  "Separando" de um pedido já enviado faria parecer que ele
                  pulou a separação.
                */}
                {state !== 'pending' && (
                  <p className="mt-0.5 text-xs text-gray-400">
                    {step.at ? formatStamp(step.at) : t('orders.timeline.noDate')}
                  </p>
                )}

                {/* Rastreio ancorado no passo "enviado", que é onde ele importa. */}
                {step.key === 'shipped' && state !== 'pending' && order.trackingCode && (
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <code className="rounded bg-gray-50 px-2 py-1 font-mono text-xs text-gray-700">
                      {order.trackingCode}
                    </code>
                    <a
                      href={`https://rastreamento.correios.com.br/app/index.php?objeto=${encodeURIComponent(order.trackingCode)}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-xs font-medium text-brand-orange hover:underline"
                    >
                      <Truck className="h-3.5 w-3.5" aria-hidden />
                      {t('orders.timeline.trackShipment')}
                      <ExternalLink className="h-3 w-3" aria-hidden />
                    </a>
                  </div>
                )}
              </div>
            </li>
          )
        })}
      </ol>

      {order.status === 'delivered' && (
        <p className="mt-3 rounded-lg bg-green-50 px-3 py-2 text-xs font-medium text-green-700">
          {t('orders.timeline.deliveredHint')}
        </p>
      )}
    </div>
  )
}
