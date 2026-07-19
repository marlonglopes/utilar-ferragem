import { useCallback, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isOrderEnabled, orderGetWithJWT, orderPatchWithJWT } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import { useBalcaoStore } from '@/store/balcaoStore'

/**
 * Fila de aprovação de desconto — `/api/v1/balcao/*` (order-service).
 *
 * A regra que este hook existe para respeitar é a REGRA 3 do
 * `internal/balcao/authz.go`: **ninguém aprova o próprio desconto**, nem o
 * gerente que fez a venda, nem o admin. O backend recusa com 403, mas depender
 * só do erro do servidor é uma UI ruim — o vendedor clicaria num botão que
 * nunca poderia funcionar. Por isso {@link canDecide} espelha a regra aqui, e a
 * tela desabilita o botão com o motivo à vista.
 *
 * O espelho é conveniência, não autorização: o servidor continua sendo quem
 * decide, e um 403 ainda é tratado.
 */

export interface BalcaoApprovalOrder {
  id: string
  number: string
  storeId?: string
  operatorId?: string
  customerName?: string
  customerDocument?: string
  customerPhone?: string
  discountPct: number
  discountAmount: number
  subtotal: number
  total: number
  approvalStatus: string
  createdAt: string
  items: Array<{ productId: string; name: string; quantity: number; unitPrice: number }>
}

interface ApprovalsResponse {
  data: BalcaoApprovalOrder[]
}

/** Motivo pelo qual o usuário atual NÃO pode decidir sobre este pedido. */
export type BlockedReason = 'self_approval' | 'not_approver' | null

/**
 * Espelho de `balcao.CanApproveOrder`. A ordem importa e é a mesma do backend:
 * a checagem de auto-aprovação vem ANTES da de cargo, porque o caso perigoso é
 * justamente o gerente que vendeu — ele TEM poder de aprovar e passaria pela
 * checagem de cargo sem problema.
 */
export function blockedReasonFor(
  order: Pick<BalcaoApprovalOrder, 'operatorId'>,
  viewer: { userId: string; canApprove: boolean }
): BlockedReason {
  if (order.operatorId && order.operatorId === viewer.userId) return 'self_approval'
  if (!viewer.canApprove) return 'not_approver'
  return null
}

export const BLOCKED_MESSAGE: Record<Exclude<BlockedReason, null>, string> = {
  self_approval: 'Você fez esta venda — outra pessoa precisa homologar o desconto.',
  not_approver: 'Seu cargo não homologa descontos.',
}

/** Fila de demonstração (modo mock), com os dois casos que importam na tela. */
function mockQueue(viewerId: string): BalcaoApprovalOrder[] {
  const base = {
    storeId: 'mock-store',
    subtotal: 1000,
    approvalStatus: 'pending',
    createdAt: new Date().toISOString(),
    items: [{ productId: 'p1', name: 'Cimento CP-II 50kg', quantity: 20, unitPrice: 50 }],
  }
  return [
    {
      ...base,
      id: 'mock-ord-1',
      number: 'BAL-0001',
      // Vendido por OUTRA pessoa: decidível.
      operatorId: 'outro-vendedor',
      customerName: 'Construtora Vale (demonstração)',
      customerDocument: '12345678000190',
      discountPct: 18,
      discountAmount: 180,
      total: 820,
    },
    {
      ...base,
      id: 'mock-ord-2',
      number: 'BAL-0002',
      // Vendido pelo PRÓPRIO usuário: demonstra o bloqueio de auto-aprovação.
      operatorId: viewerId,
      customerName: 'Cliente balcão (demonstração)',
      discountPct: 25,
      discountAmount: 250,
      total: 750,
    },
  ]
}

export interface UseBalcaoApprovalsResult {
  orders: BalcaoApprovalOrder[]
  isLoading: boolean
  isError: boolean
  errorMessage: string
  /** `null` = pode decidir. Caso contrário, o porquê. */
  blockedReason: (order: BalcaoApprovalOrder) => BlockedReason
  approve: (orderId: string) => Promise<void>
  reject: (orderId: string, note: string) => Promise<void>
  decidingId: string | null
  actionError: string
  refetch: () => void
}

export function useBalcaoApprovals(): UseBalcaoApprovalsResult {
  const token = useAuthStore((s) => s.user?.token ?? null)
  const authUserId = useAuthStore((s) => s.user?.id ?? '')
  const operator = useBalcaoStore((s) => s.operator)
  const queryClient = useQueryClient()
  const [decidingId, setDecidingId] = useState<string | null>(null)
  const [actionError, setActionError] = useState('')

  const live = isOrderEnabled && !!token
  // O id do vínculo é o que o backend compara com `operator_id`; o do authStore
  // é o fallback de quando o contexto de loja ainda não chegou.
  const viewerId = operator.userId || authUserId

  const query = useQuery({
    queryKey: ['balcao', 'approvals', token],
    // Fila de gerente: dado que envelhece rápido e vale pouco em cache.
    staleTime: 10_000,
    retry: 1,
    queryFn: async (): Promise<BalcaoApprovalOrder[]> => {
      if (!live) return mockQueue(viewerId)
      const res = await orderGetWithJWT<ApprovalsResponse>('/api/v1/balcao/approvals', token!)
      return res.data ?? []
    },
  })

  const decide = useMutation({
    mutationFn: async (input: { orderId: string; decision: 'approve' | 'reject'; note?: string }) => {
      if (!live) return
      await orderPatchWithJWT(
        `/api/v1/balcao/orders/${input.orderId}/${input.decision}`,
        token!,
        input.note ? { note: input.note } : {}
      )
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['balcao', 'approvals'] })
    },
  })

  const run = useCallback(
    async (orderId: string, decision: 'approve' | 'reject', note?: string) => {
      setActionError('')
      setDecidingId(orderId)
      try {
        await decide.mutateAsync({ orderId, decision, note })
        if (!live) {
          // Modo demonstração: some da fila localmente, para o fluxo ter fim.
          queryClient.setQueryData<BalcaoApprovalOrder[]>(['balcao', 'approvals', token], (prev) =>
            (prev ?? []).filter((o) => o.id !== orderId)
          )
        }
      } catch (err) {
        setActionError(err instanceof Error ? err.message : 'Não foi possível registrar a decisão.')
      } finally {
        setDecidingId(null)
      }
    },
    [decide, live, queryClient, token]
  )

  const approve = useCallback((orderId: string) => run(orderId, 'approve'), [run])
  const reject = useCallback(
    (orderId: string, note: string) => run(orderId, 'reject', note),
    [run]
  )

  const blockedReason = useCallback(
    (order: BalcaoApprovalOrder) =>
      blockedReasonFor(order, { userId: viewerId, canApprove: operator.canApproveDiscount }),
    [viewerId, operator.canApproveDiscount]
  )

  return {
    orders: query.data ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    errorMessage: query.error instanceof Error ? query.error.message : '',
    blockedReason,
    approve,
    reject,
    decidingId,
    actionError,
    refetch: () => void query.refetch(),
  }
}
