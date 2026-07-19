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
 *
 * Consequência no pedido: como `paymentMethod` é obrigatório e só aceita
 * `pix|boleto|card`, a venda na maquininha é gravada como `card`. O pedido fica
 * correto em valor e desconto, mas o MEIO de pagamento registrado não
 * corresponde ao que aconteceu no caixa.
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
  /**
   * Pedido nasceu pendente de homologação do gerente. Quem manda é o
   * `approvalStatus` da RESPOSTA do servidor — o `requiresApproval` calculado no
   * front é só previsão, e diverge se o teto mudou no banco entre o login e a
   * venda.
   */
  requiresApproval: boolean
  /** `not_required | pending | approved | rejected`, direto do backend. */
  approvalStatus: string
  /** Desconto que o servidor de fato aplicou (pode diferir do pedido). */
  discountPct: number
  discountAmount: number
}

/** Subset de `model.Order` que o PDV lê de volta ao criar a venda. */
interface CreateOrderResponse {
  id: string
  number: string
  approvalStatus?: string
  discountPct?: number
  discountAmount?: number
}

interface CreatedOrder {
  id: string
  number?: string
  approvalStatus: string
  discountPct: number
  discountAmount: number
}

/**
 * Traduz o erro do order-service em algo acionável no caixa.
 *
 * LIMITAÇÃO CONHECIDA: `handleResponse` em `lib/api.ts` (arquivo de outro dono)
 * descarta `code` e `details` do envelope e propaga só `error` como mensagem.
 * Então `insufficient_stock` — que chega com o item e o saldo em `details` — só
 * pode ser reconhecido pelo texto. Funciona, mas é frágil: ver o relatório.
 */
export function describeOrderError(err: unknown): string {
  const raw = err instanceof Error ? err.message : ''
  if (!raw) return 'Erro ao registrar o pedido.'

  if (/estoque insuficiente/i.test(raw)) {
    return `Estoque insuficiente — ajuste a quantidade. ${raw}`
  }
  if (/must not carry a shipping address/i.test(raw)) {
    return 'Pedido de balcão não aceita endereço de entrega (retirada no ato).'
  }
  if (/store operator role required|operador sem loja/i.test(raw)) {
    return 'Sua conta não está vinculada a uma loja. Fale com o gerente.'
  }
  if (/telefone/i.test(raw)) {
    return 'Telefone do cliente é obrigatório para cobrar.'
  }
  if (/identifique o cliente/i.test(raw)) {
    return 'Identifique o cliente antes de cobrar.'
  }
  if (/failed to fetch|networkerror|load failed/i.test(raw)) {
    return 'Sem conexão com o servidor. Verifique a rede da loja e tente de novo.'
  }
  return raw
}

export function useBalcaoCheckout() {
  const accessToken = useAuthStore((s) => s.user?.token ?? null)
  const payment = usePayment()
  const [submitting, setSubmitting] = useState(false)
  const [orderError, setOrderError] = useState('')
  const [outcome, setOutcome] = useState<BalcaoChargeOutcome | null>(null)

  /**
   * Cria o pedido no order-service com o contrato de balcão.
   *
   * SEM `address`, de propósito: o backend agora RECUSA (400) endereço em pedido
   * de balcão. O pseudo-endereço que este hook mandava antes ("Retirada no
   * balcão", CEP 00000-000) existia só para passar num `binding:"required"` que
   * já não existe — e ia parar na etiqueta de entrega e na cotação de frete.
   *
   * `discountPct` é INTENÇÃO: o servidor recalcula o valor em reais sobre o
   * subtotal que ele mesmo apurou e compara com o teto do vínculo. Por isso a
   * resposta é lida de volta em vez de confiar no cálculo local.
   */
  const createOrder = useCallback(
    async (input: BalcaoChargeInput): Promise<CreatedOrder> => {
      if (!isOrderEnabled || !accessToken) {
        // Modo mock/demo: pedido local, sem backend. A previsão local do
        // `requiresApproval` é o melhor disponível aqui.
        return {
          id: `balcao-${Date.now().toString(36)}`,
          approvalStatus: input.pricing.requiresApproval ? 'pending' : 'not_required',
          discountPct: input.pricing.discountPct,
          discountAmount: input.pricing.discountAmount,
        }
      }
      const payload = {
        channel: 'balcao',
        paymentMethod: input.method === 'external' ? 'card' : input.method,
        items: input.items.map((i) => ({
          productId: i.productId,
          name: i.name,
          icon: i.icon,
          sellerId: 'balcao',
          sellerName: 'Loja física',
          quantity: i.quantity,
          unitPrice: i.unitPrice,
        })),
        discountPct: input.pricing.discountPct,
        // `customerId` só vai quando o cliente veio do cadastro leve
        // (`/api/v1/store/customers`); avulso identifica-se pelo snapshot.
        ...(input.customer.id ? { customerId: input.customer.id } : {}),
        customerName: input.customer.name,
        customerDocument: input.customer.document.replace(/\D/g, ''),
        customerPhone: input.customer.phone.replace(/\D/g, ''),
      }
      const order = await orderPostWithJWT<CreateOrderResponse>(
        '/api/v1/orders',
        accessToken,
        payload
      )
      return {
        id: order.id,
        number: order.number,
        approvalStatus: order.approvalStatus ?? 'not_required',
        discountPct: order.discountPct ?? input.pricing.discountPct,
        discountAmount: order.discountAmount ?? input.pricing.discountAmount,
      }
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
        // A verdade sobre a aprovação é a do servidor: ele reavaliou o desconto
        // contra o teto atual do vínculo, que pode ter mudado desde o login.
        const requiresApproval = order.approvalStatus === 'pending'
        const settled = {
          approvalStatus: order.approvalStatus,
          discountPct: order.discountPct,
          discountAmount: order.discountAmount,
          requiresApproval,
        }

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
            ...settled,
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
          ...settled,
        }
        setOutcome(done)
        return done
      } catch (err) {
        setOrderError(describeOrderError(err))
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
