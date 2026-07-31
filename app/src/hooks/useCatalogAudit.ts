import { useQuery, type UseQueryResult } from '@tanstack/react-query'
import { fetchAuditActivity, type AuditPage, type AuditQuery } from '@/lib/adminAuditApi'

export function useAuditActivity(q: AuditQuery): UseQueryResult<AuditPage> {
  return useQuery({
    queryKey: ['admin-audit', 'activity', q],
    queryFn: () => fetchAuditActivity(q),
    staleTime: 15_000,
  })
}
