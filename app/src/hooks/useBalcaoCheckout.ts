import { useCallback, useState } from 'react'
import { usePayment, type PaymentMethod, type PaymentResult } from '@/hooks/usePayment'
import { isOrderEnabled, orderPostWithJWT } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import type { BalcaoCustomer, BalcaoItem, BalcaoPricing } from '@/store/balcaoStore'

/**
 * Cobrança do PDV de balcão.
 *
 * NÃO reimplementa pagamento: Pix, cartão e boleto passam inteiros pelo
 * `usePayment` (mesma PSP, mesmo polling, mesmo parsing). O que este hook
 * adiciona é (a) a criação do pedido com os dados do balcão e (b) o método
 * `external` (maquininha), que não existe no payment-service.
 */

/**
 * `external` = maquininha de cartão / dinheiro — a transação acontece FORA do
 * sistema e o PDV só registra que foi paga, com o NSU do comprovante.
 *
 * TODO(backend): o enum de métodos do payment-service é `pix | boleto | card`.
 * Não existe `external`, nem campo para NSU/autorização, nem um endpoint para
 * marcar um pedido como pago por meio externo. Hoje isso é registrado APENAS no
 * frontend — nada é persistido. Ver relatório: precisa de
 * `POST /api/v1/payments` aceitando `method=external` + `external_nsu`, com
 * status já `confirmed` (sem PSP, sem webhook), ou um endpoint dedicado
 * `POST /api/v1/orders/:id/settle-external`.
 */
export type BalcaoPaymentMethod = PaymentMethod | 'external'

export interface ExternalPaymentRecord {
  method: 'external'
  nsu: string
  orderId: string
  amount: number
  recordedAt: string
  /** Sempre true: nada foi enviado ao backend. */
  localOnly: true
}

export interface BalcaoChargeInput {
  items: BalcaoItem[]
  pricing: BalcaoPricing
  customer: BalcaoCustomer
  method: BalcaoPaymentMethod
  /** Obrigatório quando `method === 'external'`. */
  nsu?: string
}

export interface BalcaoChargeOutcome {
  orderId: string
  orderNumber?: string
  method: BalcaoPaymentMethod
  payment?: PaymentResult
  external?: ExternalPaymentRecord
  /** Desconto acima do teto do cargo — pedido nasce pendente de aprovação. */
  requiresApproval: boolean
}

export function useBalcaoCheckout() {
  const accessToken = useAuthStore((s) => s.user?.token ?? null)
  const payment = usePayment()
  const [submitting, setSubmitting] = useState(false)
  const [orderError, setOrderError] = useState('')
  const [outcome, setOutcome] = useState<BalcaoChargeOutcome | null>(null)

  /**
   * Cria o pedido no order-service.
   *
   * TODO(backend): o payload de `POST /api/v1/orders` exige `address` (entrega).
   * Venda de balcão é retirada no ato — não há endereço. Enviamos um
   * pseudo-endereço "retirada na loja" para não quebrar a validação. O
   * order-service precisa de um `channel: 'balcao' | 'web'` e tornar `address`
   * opcional quando o canal é balcão, além de guardar
   * `operator_id`, `discount_pct` e `approval_status`.
   */
  const createOrder = useCallback(
    async (input: BalcaoChargeInput): Promise<{ id: string; number?: string }> => {
      if (!isOrderEnabled || !accessToken) {
        // Modo mock/demo: pedido local, sem backend.
        return { id: `balcao-${Date.now().toString(36)}`, number: undefined }
      }
      const payload = {
        items: input.items.map((i) => ({
          productId: i.productId,
          name: i.name,
          icon: i.icon,
          sellerId: 'balcao',
          sellerName: 'Loja física',
          quantity: i.quantity,
          unitPrice: i.unitPrice,
        })),
        // TODO(backend): substituir por `channel: 'balcao'` quando existir.
        address: {
          street: 'Retirada no balcão',
          number: 'S/N',
          neighborhood: 'Loja física',
          city: 'São Paulo',
          state: 'SP',
          cep: '00000-000',
        },
      }
      const order = await orderPostWithJWT<{ id: string; number: string }>(
        '/api/v1/orders',
        accessToken,
        payload
      )
      return { id: order.id, number: order.number }
    },
    [accessToken]
  )

  const charge = useCallback(
    async (input: BalcaoChargeInput): Promise<BalcaoChargeOutcome | null> => {
      if (input.pricing.blocked) {
        setOrderError('Desconto abaixo do custo — cobrança bloqueada.')
        return null
      }
      if (input.method === 'external' && !input.nsu?.trim()) {
        setOrderError('Informe o NSU do comprovante da maquininha.')
        return null
      }
      if (!input.customer.phone.replace(/\D/g, '')) {
        // A Appmax rejeita a cobrança sem celular do pagador (403 confirmado).
        setOrderError('Telefone do cliente é obrigatório para cobrar.')
        return null
      }

      setOrderError('')
      setSubmitting(true)
      try {
        const order = await createOrder(input)

        if (input.method === 'external') {
          const external: ExternalPaymentRecord = {
            method: 'external',
            nsu: input.nsu!.trim(),
            orderId: order.id,
            amount: input.pricing.total,
            recordedAt: new Date().toISOString(),
            localOnly: true,
          }
          const done: BalcaoChargeOutcome = {
            orderId: order.id,
            orderNumber: order.number,
            method: 'external',
            external,
            requiresApproval: input.pricing.requiresApproval,
          }
          setOutcome(done)
          return done
        }

        const result = await payment.createPayment(
          order.id,
          input.method,
          input.pricing.total,
          {
            payer_name: input.customer.name,
            payer_cpf: input.customer.document.replace(/\D/g, ''),
            payer_phone: input.customer.phone.replace(/\D/g, ''),
          }
        )
        if (!result) return null

        const done: BalcaoChargeOutcome = {
          orderId: order.id,
          orderNumber: order.number,
          method: input.method,
          payment: result,
          requiresApproval: input.pricing.requiresApproval,
        }
        setOutcome(done)
        return done
      } catch (err) {
        setOrderError(err instanceof Error ? err.message : 'Erro ao registrar o pedido')
        return null
      } finally {
        setSubmitting(false)
      }
    },
    [createOrder, payment]
  )

  const reset = useCallback(() => {
    setOutcome(null)
    setOrderError('')
    payment.stopPolling()
  }, [payment])

  return {
    charge,
    reset,
    submitting,
    outcome,
    error: orderError || payment.error,
    paymentResult: payment.result,
    markConfirmed: payment.markConfirmed,
    simulateConfirm: payment.simulateConfirm,
  }
}
