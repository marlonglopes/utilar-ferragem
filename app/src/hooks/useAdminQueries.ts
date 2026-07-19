import { useQuery, type UseQueryResult } from '@tanstack/react-query'
import {
  fetchAccountingSummary,
  fetchAuditEvents,
  fetchChainVerification,
  fetchLedger,
  fetchObservability,
  fetchOverview,
  fetchReconciliation,
  fetchSellerPerformance,
  type AuditQuery,
  type LedgerQuery,
} from '@/lib/adminApi'
import type {
  AccountingSummary,
  AdminOverview,
  AdminPeriod,
  AuditEventPage,
  ChainVerification,
  LedgerPage,
  ObservabilitySnapshot,
  ReconciliationReport,
  SellerPerformanceReport,
} from '@/lib/adminTypes'

/**
 * Queries do painel admin.
 *
 * Duas políticas específicas daqui:
 *
 * - **`gcTime` curto (2 min).** O cache do TanStack Query é memória da aba, mas
 *   dado contábil não precisa sobreviver 5 minutos depois que o admin saiu da
 *   tela. Sair do painel e o cache evaporar é o comportamento desejado.
 * - **Observabilidade tem `refetchInterval`; contábil não.** Saúde de serviço
 *   sem atualização automática é inútil (o incidente acontece enquanto você
 *   olha). Livro-razão que se mexe sozinho embaixo do cursor, ao contrário,
 *   atrapalha quem está conferindo linha a linha.
 */

const ADMIN_GC_TIME = 2 * 60 * 1000

const base = { gcTime: ADMIN_GC_TIME, retry: 1 } as const

export const adminKeys = {
  overview: (p: AdminPeriod) => ['admin', 'overview', p.from, p.to] as const,
  accounting: (p: AdminPeriod) => ['admin', 'accounting', p.from, p.to] as const,
  ledger: (q: LedgerQuery) =>
    ['admin', 'ledger', q.from, q.to, q.kind ?? '', q.method ?? '', q.page, q.pageSize] as const,
  reconciliation: (p: AdminPeriod) => ['admin', 'reconciliation', p.from, p.to] as const,
  sellers: (p: AdminPeriod, storeId?: string) =>
    ['admin', 'sellers', p.from, p.to, storeId ?? ''] as const,
  audit: (q: AuditQuery) =>
    [
      'admin',
      'audit',
      q.from,
      q.to,
      q.actorId ?? '',
      q.entityType ?? '',
      q.action ?? '',
      q.page,
      q.pageSize,
    ] as const,
  chain: (p: AdminPeriod) => ['admin', 'chain', p.from, p.to] as const,
  observability: () => ['admin', 'observability'] as const,
}

export function useAdminOverview(period: AdminPeriod): UseQueryResult<AdminOverview> {
  return useQuery({
    queryKey: adminKeys.overview(period),
    queryFn: () => fetchOverview(period),
    staleTime: 60_000,
    ...base,
  })
}

export function useAdminAccounting(period: AdminPeriod): UseQueryResult<AccountingSummary> {
  return useQuery({
    queryKey: adminKeys.accounting(period),
    queryFn: () => fetchAccountingSummary(period),
    staleTime: 5 * 60_000,
    ...base,
  })
}

export function useAdminLedger(query: LedgerQuery): UseQueryResult<LedgerPage> {
  return useQuery({
    queryKey: adminKeys.ledger(query),
    queryFn: () => fetchLedger(query),
    staleTime: 5 * 60_000,
    // Sem isto a tabela pisca para o esqueleto a cada troca de página e o olho
    // perde a linha que estava conferindo.
    placeholderData: (prev) => prev,
    ...base,
  })
}

export function useAdminReconciliation(period: AdminPeriod): UseQueryResult<ReconciliationReport> {
  return useQuery({
    queryKey: adminKeys.reconciliation(period),
    queryFn: () => fetchReconciliation(period),
    staleTime: 5 * 60_000,
    ...base,
  })
}

export function useAdminSellers(
  period: AdminPeriod,
  storeId?: string,
): UseQueryResult<SellerPerformanceReport> {
  return useQuery({
    queryKey: adminKeys.sellers(period, storeId),
    queryFn: () => fetchSellerPerformance(period, storeId),
    staleTime: 5 * 60_000,
    ...base,
  })
}

export function useAdminAudit(query: AuditQuery): UseQueryResult<AuditEventPage> {
  return useQuery({
    queryKey: adminKeys.audit(query),
    queryFn: () => fetchAuditEvents(query),
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    ...base,
  })
}

export function useAdminChainVerification(period: AdminPeriod): UseQueryResult<ChainVerification> {
  return useQuery({
    queryKey: adminKeys.chain(period),
    queryFn: () => fetchChainVerification(period),
    staleTime: 60_000,
    ...base,
  })
}

export function useAdminObservability(): UseQueryResult<ObservabilitySnapshot> {
  return useQuery({
    queryKey: adminKeys.observability(),
    queryFn: fetchObservability,
    staleTime: 15_000,
    // Saúde de serviço tem que se mover sozinha — é o ponto da tela.
    refetchInterval: 30_000,
    ...base,
  })
}
