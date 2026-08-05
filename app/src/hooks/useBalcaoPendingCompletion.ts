import { useCallback, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { isOrderEnabled, orderGetWithJWT, orderPostWithJWT } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import type { BalcaoApprovalOrder } from '@/hooks/useBalcaoApprovals'

/**
 * Fila de CONCLUSÃO do operador: vendas de balcão que o gerente já aprovou
 * (desconto acima do teto) mas ainda não foram PAGAS. É o que fecha o ciclo da
 * venda acima do teto — sem esta tela, a venda ficava aprovada e sem onde
 * cobrar.
 *
 * A conclusão aqui é a MAQUININHA (o caminho dominante de venda grande no
 * balcão): registra o NSU do comprovante e o backend marca pago, baixa estoque
 * e lança no livro (settle-external). Pix/boleto/cartão como conclusão ficam
 * como extensão futura.
 */

interface CompletionResponse {
  data: BalcaoApprovalOrder[]
}

// Modo demo: uma venda aprovada de exemplo, pra a tela ser navegável sem backend.
function mockQueue(): BalcaoApprovalOrder[] {
  return [
    {
      id: 'mock-aprovada-1',
      number: 'BAL-0042',
      customerName: 'Construtora Aurora',
      customerPhone: '(11) 98888-7777',
      discountPct: 22,
      discountAmount: 264,
      subtotal: 1200,
      total: 936,
      approvalStatus: 'approved',
      createdAt: new Date().toISOString(),
      items: [{ productId: 'p1', name: 'Cimento CP-II 50kg', quantity: 40, unitPrice: 30 }],
    },
  ]
}

export interface UseBalcaoPendingCompletionResult {
  orders: BalcaoApprovalOrder[]
  isLoading: boolean
  isError: boolean
  errorMessage: string
  /** Liquida a venda na maquininha (NSU do comprovante). */
  settle: (orderId: string, nsu: string) => Promise<void>
  settlingId: string | null
  actionError: string
  refetch: () => void
}

export function useBalcaoPendingCompletion(): UseBalcaoPendingCompletionResult {
  const token = useAuthStore((s) => s.user?.token ?? null)
  const queryClient = useQueryClient()
  const [settlingId, setSettlingId] = useState<string | null>(null)
  const [actionError, setActionError] = useState('')

  const live = isOrderEnabled && !!token
  const queryKey = ['balcao', 'pending-completion', token]

  const query = useQuery({
    queryKey,
    staleTime: 10_000,
    retry: 1,
    queryFn: async (): Promise<BalcaoApprovalOrder[]> => {
      if (!live) return mockQueue()
      const res = await orderGetWithJWT<CompletionResponse>(
        '/api/v1/balcao/pending-completion',
        token!
      )
      return res.data ?? []
    },
  })

  const settleMutation = useMutation({
    mutationFn: async (input: { orderId: string; nsu: string }) => {
      if (!live) return
      // Idempotente por NSU do lado do backend (retry seguro).
      await orderPostWithJWT(`/api/v1/balcao/orders/${input.orderId}/settle-external`, token!, {
        nsu: input.nsu,
      })
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['balcao', 'pending-completion'] })
    },
  })

  const settle = useCallback(
    async (orderId: string, nsu: string) => {
      setActionError('')
      if (!nsu.trim()) {
        setActionError('Informe o NSU do comprovante da maquininha.')
        return
      }
      setSettlingId(orderId)
      try {
        await settleMutation.mutateAsync({ orderId, nsu: nsu.trim() })
        if (!live) {
          // Demo: some da fila localmente, pro fluxo ter fim.
          queryClient.setQueryData<BalcaoApprovalOrder[]>(queryKey, (prev) =>
            (prev ?? []).filter((o) => o.id !== orderId)
          )
        }
      } catch (err) {
        setActionError(err instanceof Error ? err.message : 'Não foi possível concluir a venda.')
      } finally {
        setSettlingId(null)
      }
    },
    // queryKey é derivado de token; live também. Estável entre renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [settleMutation, live, queryClient, token]
  )

  return {
    orders: query.data ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    errorMessage: query.error instanceof Error ? query.error.message : '',
    settle,
    settlingId,
    actionError,
    refetch: () => void query.refetch(),
  }
}
