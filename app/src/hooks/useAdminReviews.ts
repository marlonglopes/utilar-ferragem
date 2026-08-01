import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  fetchReviews,
  moderateReview,
  type AdminReview,
  type ReviewStatus,
} from '@/lib/adminReviewsApi'

export function useAdminReviews(status: ReviewStatus): UseQueryResult<AdminReview[]> {
  return useQuery({
    queryKey: ['admin-reviews', status],
    queryFn: () => fetchReviews(status),
    staleTime: 10_000,
  })
}

export function useModerateReview() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      action,
      note,
    }: {
      id: string
      action: 'approve' | 'reject'
      note?: string
    }) => moderateReview(id, action, note),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-reviews'] })
    },
  })
}
