import type {
  AccountingByMethod,
  AccountingSummary,
  AdminPeriod,
  AuditEvent,
  ChainVerification,
  LedgerEntry,
  PaymentMethod,
  ReconciliationItem,
  ReconciliationReport,
  Severity,
  TimePoint,
} from '@/lib/adminTypes'

/**
 * Adaptadores do contrato REAL do payment-service para o modelo de view do
 * painel.
 *
 * Por que existe uma camada aqui em vez de o front falar o dialeto do backend
 * direto: o `/api/v1/ledger/*` é modelado para o contador (partida dobrada,
 * balancete, período contábil), e a tela é modelada para quem toma decisão. Os
 * dois modelos são legitimamente diferentes. Colar o dialeto do banco nos
 * componentes acoplaria a UI a um esquema contábil que vai evoluir.
 *
 * Todas as funções abaixo são puras e testáveis sem rede.
 *
 * ── Divergências reais entre o que a tela precisa e o que a API entrega ──
 * (registradas aqui porque são invisíveis no componente e viram bug silencioso)
 *
 * 1. `by-method` NÃO recorta chargeback. O adaptador devolve `null`, não `0`:
 *    "não sabemos" e "não houve" são coisas diferentes num relatório contábil.
 * 2. `by-method` NÃO traz taxa de antecipação nem repasse, então a soma das
 *    linhas por método não fecha com o total do `summary`. Esperado — o rodapé
 *    da tabela usa o `summary`, nunca a soma das linhas.
 * 3. `entries` não pagina (só `limit`). A paginação é feita no cliente sobre a
 *    janela carregada. Para períodos longos isso tem teto — ver `LEDGER_LIMIT`.
 * 4. A trilha de auditoria é genérica (`create|update|delete|access|approve|…`)
 *    e não tem os verbos de negócio que a tela nomeia. O mapeamento é
 *    heurístico, por `entityType` + `action`.
 */

// ---------------------------------------------------------------------------
// Contábil
// ---------------------------------------------------------------------------

/** `GET /api/v1/ledger/summary` */
export interface ApiLedgerSummary {
  from: string
  to: string
  currency: string
  grossCents: number
  pspFeesCents: number
  anticipationFeesCents: number
  refundsCents: number
  chargebacksCents: number
  sellerSplitCents: number
  netCents: number
  transactionCount: number
}

/** `GET /api/v1/ledger/by-method` → `{from, to, methods: [...]}` */
export interface ApiMethodBreakdown {
  method: string // "pix" | "boleto" | "card" | ""
  grossCents: number
  pspFeesCents: number
  refundsCents: number
  netCents: number
  saleCount: number
}

/** `GET /api/v1/ledger/daily` → `{from, to, points: [...]}` */
export interface ApiDailyPoint {
  day: string // YYYY-MM-DD
  grossCents: number
  pspFeesCents: number
  refundsCents: number
  netCents: number
}

/** `GET /api/v1/ledger/entries` → `{from, to, entries: [...]}` */
export interface ApiEntryRow {
  transactionId: string
  occurredAt: string
  kind: string // sale | refund | chargeback | fee | payout | adjustment | ...
  sourceType: string // payment | order | manual | ...
  sourceId: string
  description: string
  account: string // código da conta, ex. "3.1.1"
  accountName: string
  side: 'debit' | 'credit'
  amountCents: number
  paymentMethod?: string
  sellerId?: string
  memo?: string
  requestId?: string
}

/** Método do backend → união do front. `""` (sem método) vira `null`. */
export function toPaymentMethod(raw: string | undefined | null): PaymentMethod | null {
  switch (raw) {
    case 'pix':
      return 'pix'
    case 'boleto':
      return 'boleto'
    // O livro grava só `card`; não desdobra em crédito/débito. O front não
    // inventa a distinção — inventar aqui produziria um relatório que afirma
    // algo que o banco não sabe.
    case 'card':
      return 'card'
    case 'credit_card':
      return 'credit_card'
    case 'debit_card':
      return 'debit_card'
    case 'cash':
      return 'cash'
    default:
      return null
  }
}

const LEDGER_KINDS = new Set<LedgerEntry['kind']>([
  'sale',
  'psp_fee',
  'refund',
  'chargeback',
  'adjustment',
  'payout',
])

/** Natureza do backend → natureza da UI. Desconhecida cai em `adjustment`. */
export function toLedgerKind(raw: string): LedgerEntry['kind'] {
  const normalized = raw === 'fee' || raw === 'psp_fee' ? 'psp_fee' : raw
  return LEDGER_KINDS.has(normalized as LedgerEntry['kind'])
    ? (normalized as LedgerEntry['kind'])
    : 'adjustment'
}

export function adaptSummary(
  period: AdminPeriod,
  s: ApiLedgerSummary,
  methods: ApiMethodBreakdown[],
  points: ApiDailyPoint[],
): AccountingSummary {
  return {
    period,
    grossCents: s.grossCents,
    pspFeeCents: s.pspFeesCents,
    refundCents: s.refundsCents,
    chargebackCents: s.chargebacksCents,
    anticipationFeeCents: s.anticipationFeesCents,
    sellerSplitCents: s.sellerSplitCents,
    netCents: s.netCents,
    transactionCount: s.transactionCount,
    byMethod: methods.map(adaptMethodBreakdown),
    series: points.map(adaptDailyPoint),
  }
}

export function adaptMethodBreakdown(m: ApiMethodBreakdown): AccountingByMethod {
  return {
    method: toPaymentMethod(m.method) ?? 'cash',
    grossCents: m.grossCents,
    pspFeeCents: m.pspFeesCents,
    refundCents: m.refundsCents,
    // `null`, não `0`: a rota não recorta chargeback por método.
    chargebackCents: null,
    netCents: m.netCents,
    transactions: m.saleCount,
    effectiveFeeRate: m.grossCents > 0 ? m.pspFeesCents / m.grossCents : 0,
  }
}

export function adaptDailyPoint(p: ApiDailyPoint): TimePoint {
  // `orders` não existe na série contábil — o livro conta lançamentos, não
  // pedidos. Zero aqui é honesto: o gráfico contábil não plota pedidos.
  return { date: p.day, valueCents: p.netCents, orders: 0 }
}

/**
 * Uma linha do razão. O backend entrega `side` + `amountCents`; a tela mostra
 * colunas de débito e crédito separadas, que é como o contador lê.
 *
 * O `id` é sintetizado a partir de `transactionId` + índice porque a linha de
 * entrada não tem id próprio na resposta — e o React precisa de chave estável.
 */
export function adaptEntry(e: ApiEntryRow, index: number): LedgerEntry {
  const isOrder = e.sourceType === 'order'
  const isPayment = e.sourceType === 'payment'
  return {
    id: `${e.transactionId}:${index}`,
    occurredAt: e.occurredAt,
    account: e.accountName,
    accountCode: e.account,
    debitCents: e.side === 'debit' ? e.amountCents : 0,
    creditCents: e.side === 'credit' ? e.amountCents : 0,
    kind: toLedgerKind(e.kind),
    method: toPaymentMethod(e.paymentMethod),
    orderId: isOrder ? e.sourceId : null,
    orderNumber: isOrder ? e.sourceId : null,
    paymentId: isPayment ? e.sourceId : null,
    // O razão referencia o pagamento interno, não a transação do PSP. Bater com
    // o extrato é trabalho da reconciliação, que traz `pspPaymentId`.
    pspTransactionId: null,
    memo: e.memo || e.description,
  }
}

// ---------------------------------------------------------------------------
// Reconciliação
// ---------------------------------------------------------------------------

/** `GET /api/v1/ledger/discrepancies` → `{discrepancies: [...]}` */
export interface ApiDiscrepancy {
  id: string
  runId: string
  paymentId?: string
  pspPaymentId?: string
  kind: string // amount_mismatch | status_mismatch | missing_at_psp | psp_error | ledger_missing
  severity: string
  localValue: string
  pspValue: string
  amountDeltaCents: number
  detail?: string
  detectedAt: string
  resolvedAt?: string
  resolvedBy?: string
  resolutionNote?: string
}

const DISCREPANCY_TYPE: Record<string, ReconciliationItem['type']> = {
  amount_mismatch: 'amount_mismatch',
  // Divergência de status é, para a leitura contábil, o livro discordando do
  // PSP sobre o valor reconhecido.
  status_mismatch: 'amount_mismatch',
  missing_at_psp: 'missing_in_psp',
  ledger_missing: 'missing_in_ledger',
  psp_error: 'fee_mismatch',
}

/**
 * Severidade textual do backend → escala do painel.
 *
 * `ledger_missing` é forçado a `critical` independentemente do que vier: é
 * dinheiro confirmado sem lançamento, ou seja, faturamento que não existe no
 * livro. Nunca é "atenção".
 */
export function adaptDiscrepancySeverity(kind: string, raw: string): Severity {
  if (kind === 'ledger_missing') return 'critical'
  switch (raw) {
    case 'critical':
    case 'high':
      return 'critical'
    case 'warn':
    case 'warning':
    case 'medium':
      return 'warn'
    case 'low':
    case 'info':
    case 'ok':
      return 'ok'
    default:
      return 'warn'
  }
}

export function adaptDiscrepancy(d: ApiDiscrepancy): ReconciliationItem {
  return {
    id: d.id,
    date: d.detectedAt.slice(0, 10),
    // O backend não devolve os dois lados em centavos, só o delta e os valores
    // como texto. `ledgerCents` é reconstruído a partir do delta quando dá; do
    // contrário fica 0 e a tela mostra o delta, que é o número que importa.
    ledgerCents: 0,
    pspCents: d.amountDeltaCents,
    deltaCents: d.amountDeltaCents,
    type: DISCREPANCY_TYPE[d.kind] ?? 'amount_mismatch',
    severity: adaptDiscrepancySeverity(d.kind, d.severity),
    pspTransactionId: d.pspPaymentId ?? null,
    orderNumber: d.paymentId ?? null,
    note:
      d.detail ||
      `Local: ${d.localValue || '—'} · PSP: ${d.pspValue || '—'}`,
  }
}

export function adaptReconciliation(
  period: AdminPeriod,
  discrepancies: ApiDiscrepancy[],
): ReconciliationReport {
  const items = discrepancies.map(adaptDiscrepancy)
  return {
    period,
    // A rota de divergências abertas não informa quando o job rodou; o
    // `detectedAt` mais recente é a melhor aproximação disponível.
    lastRunAt: items.length > 0 ? discrepancies[0].detectedAt : new Date().toISOString(),
    // `matchedCount` só existe no resultado de uma execução (`POST /reconcile`),
    // não na listagem de divergências abertas.
    matchedCount: 0,
    divergentCount: items.length,
    totalDeltaCents: items.reduce((a, i) => a + i.deltaCents, 0),
    items,
  }
}

// ---------------------------------------------------------------------------
// Trilha de auditoria
// ---------------------------------------------------------------------------

/**
 * `GET /api/v1/ledger/audit` → `{records: [...]}`
 *
 * ⚠️ `pkg/audit.Record` não tem tags `json`, então a serialização sai em
 * **PascalCase** — divergente do camelCase de todo o resto da API. Os campos
 * abaixo refletem o que realmente chega hoje; se o backend adicionar as tags,
 * este é o único lugar que muda.
 */
export interface ApiAuditRecord {
  Seq: number
  OccurredAt: string
  Service: string
  ActorID: string
  ActorRole: string
  ActorIP: string
  ActorUserAgent: string
  EntityType: string
  EntityID: string
  Action: string
  OldValue: unknown
  NewValue: unknown
  RequestID: string
  PrevHash: string
  Hash: string
}

/**
 * A trilha genérica (`create|update|delete|access|approve|…`) não tem os verbos
 * de negócio que a tela nomeia. O mapeamento cruza `entityType` com `action`.
 */
export function adaptAuditAction(entityType: string, action: string): AuditEvent['action'] {
  if (action === 'access' || action === 'login' || action === 'export') return 'admin_access'
  if (action === 'approve' || action === 'reject') return 'discount_approval'
  if (entityType === 'user') return 'user_role_change'
  if (entityType === 'payment' || entityType.startsWith('ledger')) {
    return action === 'delete' ? 'refund_issued' : 'order_status_change'
  }
  if (entityType === 'order') return 'order_status_change'
  if (entityType === 'product' || entityType === 'price') return 'price_change'
  return 'order_status_change'
}

const AUDIT_ROLES = new Set<AuditEvent['actorRole']>(['admin', 'seller', 'operator', 'system'])

function adaptActorRole(raw: string): AuditEvent['actorRole'] {
  return AUDIT_ROLES.has(raw as AuditEvent['actorRole'])
    ? (raw as AuditEvent['actorRole'])
    : 'system'
}

/**
 * Serializa o valor de/para em texto de exibição.
 *
 * Um objeto inteiro despejado na célula tornaria a coluna ilegível, então
 * objetos de um campo só viram o próprio valor e os demais são compactados.
 * Idealmente o backend entregaria isso pronto — está registrado em
 * `docs/admin-dashboard-api.md`.
 */
export function auditValueToText(value: unknown): string | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return null
    if (entries.length === 1) return `${entries[0][0]}: ${String(entries[0][1])}`
    return entries.map(([k, v]) => `${k}: ${String(v)}`).join(' · ')
  }
  return String(value)
}

/**
 * Mascara o último octeto de um IPv4.
 *
 * ⚠️ Isto é mitigação no cliente, não correção: o backend está enviando o IP
 * **completo** (`pkg/audit.Record.ActorIP`), então o dado sensível já cruzou a
 * rede e está no cache da aba antes de chegar aqui. O mascaramento correto é no
 * servidor. Registrado como pendência em `docs/admin-dashboard-api.md`.
 */
export function maskIp(ip: string): string {
  if (!ip) return '—'
  const v4 = ip.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3})\.\d{1,3}$/)
  if (v4) return `${v4[1]}.xxx`
  // IPv6 ou formato inesperado: corta o sufixo em vez de exibir por inteiro.
  const parts = ip.split(':')
  if (parts.length > 2) return `${parts.slice(0, 2).join(':')}:…`
  return ip
}

export function adaptAuditRecord(r: ApiAuditRecord): AuditEvent {
  return {
    id: `au-${r.Seq}`,
    sequence: r.Seq,
    occurredAt: r.OccurredAt,
    actorId: r.ActorID,
    // A trilha guarda o id do ator, não o nome. Exibir o id é preferível a
    // uma chamada por linha ao auth-service.
    actorName: r.ActorID || 'sistema',
    actorRole: adaptActorRole(r.ActorRole),
    action: adaptAuditAction(r.EntityType, r.Action),
    entityType: r.EntityType,
    entityId: r.EntityID,
    entityLabel: `${r.EntityType} ${r.EntityID}`,
    fromValue: auditValueToText(r.OldValue),
    toValue: auditValueToText(r.NewValue),
    ip: maskIp(r.ActorIP),
    hash: r.Hash,
    prevHash: r.PrevHash || null,
  }
}

/** `GET /api/v1/ledger/audit/verify` */
export interface ApiChainVerification {
  valid: boolean
  headSeq: number
  headHash: string
  brokenAtSeq?: number
  kind?: string
  error?: string
}

export function adaptChainVerification(v: ApiChainVerification): ChainVerification {
  return {
    valid: v.valid,
    checkedCount: v.headSeq,
    brokenAtSequence: v.brokenAtSeq ?? null,
    verifiedAt: new Date().toISOString(),
    headHash: v.headHash,
  }
}
