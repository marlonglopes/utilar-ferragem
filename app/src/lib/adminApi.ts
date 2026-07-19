import { useAuthStore } from '@/store/authStore'
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
import {
  adaptChainVerification,
  adaptEntry,
  adaptAuditRecord,
  adaptReconciliation,
  adaptSummary,
  type ApiAuditRecord,
  type ApiChainVerification,
  type ApiDailyPoint,
  type ApiDiscrepancy,
  type ApiEntryRow,
  type ApiLedgerSummary,
  type ApiMethodBreakdown,
} from '@/lib/adminAdapters'
import {
  mockAccountingSummary,
  mockAuditEvents,
  mockAuditPage,
  mockChainVerification,
  mockLedgerEntries,
  mockLedgerPage,
  mockObservability,
  mockOverview,
  mockReconciliation,
  mockSellerPerformance,
} from '@/lib/adminMock'

/**
 * Camada de rede do dashboard admin.
 *
 * Vive fora de `lib/api.ts` de propósito — aquele arquivo é de outro dono e as
 * rotas de admin têm uma política própria que não deve vazar para o e-commerce:
 *
 * 1. **`cache: 'no-store'` em tudo.** Livro-razão e trilha de auditoria não
 *    podem ficar no cache HTTP do disco. O usuário fecha a aba, a máquina é da
 *    loja, e o próximo operador não tem que conseguir ler o faturamento no
 *    cache do navegador.
 * 2. **Nada é persistido.** Nenhuma resposta daqui vai para `localStorage`,
 *    `sessionStorage`, IndexedDB ou telemetria. O único estado que sobrevive ao
 *    reload é o filtro de período (na URL, via query string) — que não é dado.
 * 3. **O token vem do store a cada chamada**, nunca capturado em closure — o
 *    refresh-on-401 do app pode ter trocado o access token no meio da sessão.
 *
 * ⚠️ O guard de rota NÃO é fronteira de segurança. Toda rota abaixo tem que ser
 * autorizada no servidor por papel `admin`. Ver `docs/admin-dashboard-api.md`.
 */

const PAYMENT_URL = import.meta.env.VITE_API_URL ?? ''
const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
const AUTH_URL = import.meta.env.VITE_AUTH_URL ?? ''
const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''

/**
 * Modo mock: sem backend configurado, o painel roda com dados de demonstração.
 * Mesmo critério do resto do app (`VITE_*_URL` vazio) — ver CLAUDE.md.
 */
export const isAdminApiEnabled = PAYMENT_URL !== '' && ORDER_URL !== '' && AUTH_URL !== ''

export class AdminApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message)
    this.name = 'AdminApiError'
  }
}

function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

async function adminGet<T>(base: string, path: string): Promise<T> {
  const token = useAuthStore.getState().user?.token ?? null
  const res = await fetch(`${base}${path}`, {
    cache: 'no-store',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string; code?: string }
    if (res.status === 403) {
      throw new AdminApiError('Sua conta não tem permissão de administrador.', 403, 'forbidden')
    }
    throw new AdminApiError(body.error ?? `HTTP ${res.status}`, res.status, body.code)
  }
  return (await res.json()) as T
}

// ---------------------------------------------------------------------------
// 1. Visão geral
// ---------------------------------------------------------------------------

export async function fetchOverview(period: AdminPeriod): Promise<AdminOverview> {
  if (!isAdminApiEnabled) return mockOverview(period)
  return adminGet<AdminOverview>(ORDER_URL, `/api/v1/admin/overview${qs({ ...period })}`)
}

// ---------------------------------------------------------------------------
// 2. Contábil
// ---------------------------------------------------------------------------

/**
 * O resumo da tela é a junção de TRÊS rotas do payment-service: o total
 * (`/summary`), o recorte por método (`/by-method`) e a série diária
 * (`/daily`). Disparadas em paralelo — encadeá-las triplicaria o tempo até o
 * primeiro pixel sem nenhum ganho, já que nenhuma depende da outra.
 */
export async function fetchAccountingSummary(period: AdminPeriod): Promise<AccountingSummary> {
  if (!isAdminApiEnabled) return mockAccountingSummary(period)
  const w = qs({ ...period })
  const [summary, methods, daily] = await Promise.all([
    adminGet<ApiLedgerSummary>(PAYMENT_URL, `/api/v1/ledger/summary${w}`),
    adminGet<{ methods: ApiMethodBreakdown[] }>(PAYMENT_URL, `/api/v1/ledger/by-method${w}`),
    adminGet<{ points: ApiDailyPoint[] }>(PAYMENT_URL, `/api/v1/ledger/daily${w}`),
  ])
  return adaptSummary(period, summary, methods.methods ?? [], daily.points ?? [])
}

export interface LedgerQuery extends AdminPeriod {
  kind?: string
  method?: string
  page: number
  pageSize: number
}

/**
 * Teto de linhas trazidas do razão numa consulta.
 *
 * `/api/v1/ledger/entries` não pagina — aceita só `limit` (máx. 50 000 no
 * servidor, 5 000 por padrão). A paginação da tabela é feita no cliente sobre
 * esta janela. 5 000 linhas cobrem um mês típico com folga; acima disso o
 * caminho certo é o export CSV, não rolar a tabela, e a UI já oferece o botão.
 */
export const LEDGER_LIMIT = 5000

export async function fetchLedger(q: LedgerQuery): Promise<LedgerPage> {
  if (!isAdminApiEnabled) {
    const all = mockLedgerEntries().filter(
      (e) => (!q.kind || e.kind === q.kind) && (!q.method || e.method === q.method),
    )
    return mockLedgerPage(all, q.page, q.pageSize)
  }
  const res = await adminGet<{ entries: ApiEntryRow[] }>(
    PAYMENT_URL,
    `/api/v1/ledger/entries${qs({ ...q, limit: LEDGER_LIMIT })}`,
  )
  const all = (res.entries ?? [])
    .map(adaptEntry)
    .filter((e) => (!q.kind || e.kind === q.kind) && (!q.method || e.method === q.method))
    // O backend devolve em ordem cronológica crescente; a tela lê do mais
    // recente para o mais antigo.
    .sort((a, b) => b.occurredAt.localeCompare(a.occurredAt))
  return mockLedgerPage(all, q.page, q.pageSize)
}

export async function fetchReconciliation(period: AdminPeriod): Promise<ReconciliationReport> {
  if (!isAdminApiEnabled) return mockReconciliation(period)
  const res = await adminGet<{ discrepancies: ApiDiscrepancy[] }>(
    PAYMENT_URL,
    '/api/v1/ledger/discrepancies?limit=200',
  )
  return adaptReconciliation(period, res.discrepancies ?? [])
}

/**
 * Baixa o export contábil do servidor, autenticado.
 *
 * **Não pode ser uma navegação simples** (`window.location.href = url`): o
 * `/api/v1/ledger/export` exige `Authorization: Bearer` com papel admin, e o
 * navegador não anexa header nenhum numa navegação de topo. O resultado seria
 * um 401 renderizado como página em vez de um arquivo — falha silenciosa e
 * confusa justamente no botão que o contador usa.
 *
 * Então: `fetch` com o token, resposta em blob, download por object URL.
 * Devolve `null` quando não há backend — o chamador cai no CSV gerado
 * localmente (`adminExport.ts`), com o mesmo cabeçalho.
 */
export async function fetchAccountingExport(
  period: AdminPeriod,
  format: 'csv' | 'ofx' | 'balancete' = 'csv',
): Promise<{ blob: Blob; filename: string } | null> {
  if (!isAdminApiEnabled) return null
  const token = useAuthStore.getState().user?.token ?? null
  const res = await fetch(`${PAYMENT_URL}/api/v1/ledger/export${qs({ ...period, format })}`, {
    cache: 'no-store',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!res.ok) {
    throw new AdminApiError(
      res.status === 403
        ? 'Sua conta não tem permissão de administrador.'
        : `Falha ao gerar o export (HTTP ${res.status})`,
      res.status,
    )
  }
  // O nome vem do `Content-Disposition` do servidor quando disponível — é ele
  // que sabe o padrão de nomenclatura que o contador espera.
  const disposition = res.headers.get('Content-Disposition') ?? ''
  const match = disposition.match(/filename="?([^";]+)"?/i)
  const ext = format === 'ofx' ? 'ofx' : 'csv'
  return {
    blob: await res.blob(),
    filename: match?.[1] ?? `utilar-razao-${period.from}-a-${period.to}.${ext}`,
  }
}

// ---------------------------------------------------------------------------
// 3. Vendedores
// ---------------------------------------------------------------------------

export async function fetchSellerPerformance(
  period: AdminPeriod,
  storeId?: string,
): Promise<SellerPerformanceReport> {
  if (!isAdminApiEnabled) return mockSellerPerformance(period)
  return adminGet<SellerPerformanceReport>(
    ORDER_URL,
    `/api/v1/admin/sellers/performance${qs({ ...period, storeId })}`,
  )
}

// ---------------------------------------------------------------------------
// 4. Trilha de auditoria
// ---------------------------------------------------------------------------

/** Mesmo teto e mesmo motivo do razão — a rota de auditoria não pagina. */
export const AUDIT_LIMIT = 2000

export interface AuditQuery extends AdminPeriod {
  actorId?: string
  entityType?: string
  action?: string
  page: number
  pageSize: number
}

export async function fetchAuditEvents(q: AuditQuery): Promise<AuditEventPage> {
  if (!isAdminApiEnabled) {
    const all = mockAuditEvents().filter(
      (e) =>
        (!q.actorId || e.actorId === q.actorId) &&
        (!q.entityType || e.entityType === q.entityType) &&
        (!q.action || e.action === q.action),
    )
    return mockAuditPage(all, q.page, q.pageSize)
  }
  // A trilha vive no payment-service (`pkg/audit` + LedgerHandler), não no
  // auth-service. Também não pagina: filtra por `fromSeq` + `limit`.
  const res = await adminGet<{ records: ApiAuditRecord[] }>(
    PAYMENT_URL,
    `/api/v1/ledger/audit${qs({
      entityType: q.entityType,
      actorId: q.actorId,
      limit: AUDIT_LIMIT,
    })}`,
  )
  const all = (res.records ?? [])
    .map(adaptAuditRecord)
    .filter((e) => !q.action || e.action === q.action)
    .sort((a, b) => b.sequence - a.sequence)
  return mockAuditPage(all, q.page, q.pageSize)
}

export async function fetchChainVerification(period: AdminPeriod): Promise<ChainVerification> {
  if (!isAdminApiEnabled) return mockChainVerification(mockAuditEvents())
  // `period` não é usado: a verificação é da cadeia INTEIRA, por definição —
  // verificar um recorte não provaria nada sobre os elos fora dele.
  void period
  const v = await adminGet<ApiChainVerification>(PAYMENT_URL, '/api/v1/ledger/audit/verify')
  return adaptChainVerification(v)
}

// ---------------------------------------------------------------------------
// 5. Observabilidade
// ---------------------------------------------------------------------------

export async function fetchObservability(): Promise<ObservabilitySnapshot> {
  if (!isAdminApiEnabled) return mockObservability()
  // CATALOG_URL, não PAYMENT_URL: o agregador de saúde mora no catalog
  // porque o payment é o serviço mais sensível do sistema e não deve virar
  // cliente HTTP dos outros três só para montar um painel.
  return adminGet<ObservabilitySnapshot>(CATALOG_URL, '/api/v1/admin/observability')
}
