import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  adjustStock,
  fetchMovements,
  fetchStock,
  type AdjustInput,
  type StockMovement,
  type StockPage,
  type StockQuery,
} from '@/lib/adminStockApi'

export function useAdminStock(q: StockQuery): UseQueryResult<StockPage> {
  return useQuery({
    queryKey: ['admin-stock', q],
    queryFn: () => fetchStock(q),
    staleTime: 10_000,
  })
}

export function useStockMovements(id: string | null): UseQueryResult<StockMovement[]> {
  return useQuery({
    queryKey: ['admin-stock', 'movements', id],
    queryFn: () => fetchMovements(id as string),
    enabled: !!id,
    staleTime: 5_000,
  })
}

export function useAdjustStock() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: AdjustInput }) => adjustStock(id, input),
    onSuccess: (_data, { id }) => {
      // Invalida a lista (o número mudou) e o histórico do produto ajustado.
      void qc.invalidateQueries({ queryKey: ['admin-stock'] })
      void qc.invalidateQueries({ queryKey: ['admin-stock', 'movements', id] })
    },
  })
}
