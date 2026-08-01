import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import { actOnReturn, fetchReturns, type ReturnItem } from '@/lib/adminReturnsApi'

export function useAdminReturns(status: string): UseQueryResult<ReturnItem[]> {
  return useQuery({
    queryKey: ['admin-returns', status],
    queryFn: () => fetchReturns(status),
    staleTime: 10_000,
  })
}

export function useReturnAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      action,
      note,
    }: {
      id: string
      action: 'approve' | 'reject' | 'receive' | 'refund'
      note?: string
    }) => actOnReturn(id, action, note),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-returns'] })
    },
  })
}
