import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  fetchCoupons,
  createCoupon,
  updateCoupon,
  deleteCoupon,
  type CouponInput,
} from '@/lib/adminCouponsApi'

const KEY = ['admin', 'coupons']

export function useCoupons() {
  return useQuery({ queryKey: KEY, queryFn: fetchCoupons })
}

export function useCreateCoupon() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CouponInput) => createCoupon(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

export function useUpdateCoupon() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: Partial<CouponInput> }) =>
      updateCoupon(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

export function useDeleteCoupon() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCoupon(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}
