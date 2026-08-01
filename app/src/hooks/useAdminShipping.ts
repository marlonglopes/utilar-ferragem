import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  createRate,
  deleteRate,
  fetchRates,
  updateRate,
  type ShippingRate,
  type ShippingRateInput,
} from '@/lib/adminShippingApi'

export function useShippingRates(): UseQueryResult<ShippingRate[]> {
  return useQuery({
    queryKey: ['admin-shipping'],
    queryFn: fetchRates,
    staleTime: 15_000,
  })
}

export function useCreateRate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ShippingRateInput) => createRate(input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin-shipping'] }),
  })
}

export function useUpdateRate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: ShippingRateInput }) => updateRate(id, input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin-shipping'] }),
  })
}

export function useDeleteRate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteRate(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin-shipping'] }),
  })
}
