import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  fetchAdminOrders,
  runFulfillment,
  type AdminOrderQuery,
  type AdminOrdersPage,
  type FulfillmentAction,
} from '@/lib/adminOrdersApi'

const orderKeys = {
  all: ['admin-orders'] as const,
  list: (q: AdminOrderQuery) => ['admin-orders', 'list', q] as const,
}

export function useAdminOrders(q: AdminOrderQuery): UseQueryResult<AdminOrdersPage> {
  return useQuery({
    queryKey: orderKeys.list(q),
    queryFn: () => fetchAdminOrders(q),
    staleTime: 15_000,
  })
}

/** Ação de fulfillment (separar/despachar/entregar/cancelar) + invalida a lista. */
export function useFulfillment() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: FulfillmentAction }) =>
      runFulfillment(id, action),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: orderKeys.all })
    },
  })
}
