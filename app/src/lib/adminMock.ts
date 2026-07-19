import type {
  AccountingByMethod,
  AccountingSummary,
  AdminOverview,
  AdminPeriod,
  Alert,
  AuditEvent,
  AuditEventPage,
  ChainVerification,
  LedgerEntry,
  LedgerPage,
  ObservabilitySnapshot,
  OrderStatus,
  PaymentMethod,
  ReconciliationReport,
  SellerPerformance,
  SellerPerformanceReport,
  ServiceHealth,
  TimePoint,
} from '@/lib/adminTypes'
import { avgTicketCents, outboxSeverity } from '@/lib/adminFormat'

/**
 * Dados de demonstração do dashboard admin (modo mock).
 *
 * Determinístico de propósito: um PRNG com semente fixa, não `Math.random()`.
 * Assim o teste consegue asserir números concretos e o dono vê o mesmo painel
 * a cada refresh — um dashboard que muda sozinho a cada F5 parece quebrado.
 *
 * As datas são relativas a `now` para o painel nunca parecer congelado no
 * passado, mas os VALORES vêm da semente e não do relógio.
 */

// PRNG mulberry32 — 4 linhas, sem dependência, distribuição boa o bastante.
function makeRng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function isoDate(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function daysAgo(n: number, base = new Date()): Date {
  const d = new Date(base)
  d.setDate(d.getDate() - n)
  return d
}

/** Período padrão do painel: últimos 30 dias, terminando hoje. */
export function defaultPeriod(base = new Date()): AdminPeriod {
  return { from: isoDate(daysAgo(29, base)), to: isoDate(base) }
}

const SELLERS = [
  { id: 'v-01', name: 'Cleiton Ramos', storeId: 'l-centro' },
  { id: 'v-02', name: 'Marinalva Souza', storeId: 'l-centro' },
  { id: 'v-03', name: 'Jorge Bittencourt', storeId: 'l-centro' },
  { id: 'v-04', name: 'Patrícia Nogueira', storeId: 'l-industrial' },
  { id: 'v-05', name: 'Wanderson Lima', storeId: 'l-industrial' },
  { id: 'v-06', name: 'Rosana Peixoto', storeId: 'l-industrial' },
  { id: 'v-07', name: 'Edilson Tavares', storeId: 'l-rodovia' },
  { id: 'v-08', name: 'Simone Caldeira', storeId: 'l-rodovia' },
]

const STORES = [
  { id: 'l-centro', name: 'Loja Centro' },
  { id: 'l-industrial', name: 'Distrito Industrial' },
  { id: 'l-rodovia', name: 'Rodovia BR-060' },
]

const METHODS: PaymentMethod[] = ['pix', 'credit_card', 'boleto', 'debit_card', 'cash']

/** Série diária com sazonalidade de ferragem: sábado forte, domingo fechado. */
function buildSeries(days: number, seed: number, baseCents: number, base = new Date()): TimePoint[] {
  const rng = makeRng(seed)
  const out: TimePoint[] = []
  for (let i = days - 1; i >= 0; i--) {
    const d = daysAgo(i, base)
    const dow = d.getDay()
    // 0=dom (fechado), 6=sáb (pico da obra do fim de semana)
    const weekday = dow === 0 ? 0.08 : dow === 6 ? 1.45 : 1
    const noise = 0.75 + rng() * 0.5
    const valueCents = Math.round(baseCents * weekday * noise)
    const orders = Math.max(0, Math.round((valueCents / 18_500) * (0.85 + rng() * 0.3)))
    out.push({ date: isoDate(d), valueCents, orders })
  }
  return out
}

// ---------------------------------------------------------------------------
// Alertas — compartilhados entre visão geral e observabilidade
// ---------------------------------------------------------------------------

function buildAlerts(base = new Date()): Alert[] {
  const t = (minutes: number) => new Date(base.getTime() - minutes * 60_000).toISOString()
  return [
    {
      id: 'al-outbox',
      severity: 'critical',
      title: 'Outbox do payment-service acumulando',
      detail:
        '184 eventos pendentes, o mais antigo há 22 minutos. Pagamento confirmado pode não estar virando pedido pago.',
      source: 'payment-service · outbox relay',
      firedAt: t(22),
      href: '/admin/observabilidade',
    },
    {
      id: 'al-recon',
      severity: 'critical',
      title: 'Divergência de reconciliação acima do limite',
      detail: 'R$ 1.284,40 de diferença acumulada contra o extrato do Appmax em 3 transações.',
      source: 'payment-service · reconciliação',
      firedAt: t(95),
      href: '/admin/contabil',
    },
    {
      id: 'al-stuck',
      severity: 'warn',
      title: '6 pedidos pagos sem separação',
      detail: 'Pagos há mais de 4 horas e ainda em "pago". Verificar o fluxo de separação da loja.',
      source: 'order-service',
      firedAt: t(140),
      href: '/admin',
    },
    {
      id: 'al-latency',
      severity: 'warn',
      title: 'Latência p95 do catalog-service em 412ms',
      detail: 'Acima do orçamento de 300ms nos últimos 15 minutos. Busca facetada é a suspeita.',
      source: 'catalog-service',
      firedAt: t(11),
      href: '/admin/observabilidade',
    },
    {
      id: 'al-discount',
      severity: 'warn',
      title: 'Desconto médio do Distrito Industrial em 14,8%',
      detail: 'Quase no teto de 15%. Margem da loja caiu 3,1 p.p. no mês.',
      source: 'order-service · desempenho',
      firedAt: t(300),
      href: '/admin/vendedores',
    },
  ]
}

// ---------------------------------------------------------------------------
// 1. Visão geral
// ---------------------------------------------------------------------------

const STATUSES: Array<{ status: OrderStatus; weight: number }> = [
  { status: 'pending', weight: 0.11 },
  { status: 'paid', weight: 0.18 },
  { status: 'picking', weight: 0.14 },
  { status: 'shipped', weight: 0.16 },
  { status: 'delivered', weight: 0.34 },
  { status: 'canceled', weight: 0.05 },
  { status: 'refunded', weight: 0.02 },
]

export function mockOverview(period: AdminPeriod, base = new Date()): AdminOverview {
  const series = buildSeries(30, 1337, 41_800_00 / 30, base)
  const monthCents = series.reduce((a, p) => a + p.valueCents, 0)
  const weekCents = series.slice(-7).reduce((a, p) => a + p.valueCents, 0)
  const todayCents = series[series.length - 1]?.valueCents ?? 0
  const orderCount = series.reduce((a, p) => a + p.orders, 0)

  const totalOrders = orderCount
  const byStatus = STATUSES.map(({ status, weight }) => ({
    status,
    count: Math.round(totalOrders * weight),
    valueCents: Math.round(monthCents * weight),
  }))

  const created = 1482
  const funnel = {
    created,
    confirmed: 1291,
    failed: 118,
    expired: 73,
    byMethod: [
      { method: 'pix' as const, created: 712, confirmed: 689 },
      { method: 'credit_card' as const, created: 508, confirmed: 431 },
      { method: 'boleto' as const, created: 174, confirmed: 96 },
      { method: 'debit_card' as const, created: 88, confirmed: 75 },
    ],
  }

  const stuck = [
    { n: '2026-8841', h: 31, c: 4_890_00, who: 'Construtora Vale Verde' },
    { n: '2026-8829', h: 19, c: 1_247_50, who: 'Rogério Anhaia' },
    { n: '2026-8817', h: 12, c: 12_380_00, who: 'Prefeitura de Anápolis' },
    { n: '2026-8802', h: 8, c: 638_90, who: 'Marcenaria Dois Irmãos' },
    { n: '2026-8795', h: 6, c: 2_115_00, who: 'Elaine Fortunato' },
    { n: '2026-8788', h: 5, c: 397_40, who: 'Serralheria Novo Tempo' },
  ]

  return {
    period,
    kpis: {
      todayCents,
      weekCents,
      monthCents,
      todayPrevCents: Math.round(todayCents * 0.88),
      weekPrevCents: Math.round(weekCents * 1.06),
      monthPrevCents: Math.round(monthCents * 0.91),
      avgTicketCents: avgTicketCents(monthCents, orderCount),
      avgTicketPrevCents: Math.round(avgTicketCents(monthCents, orderCount) * 0.96),
      orderCount,
      orderCountPrev: Math.round(orderCount * 0.93),
    },
    series,
    byStatus,
    funnel,
    stuckOrders: stuck.map((s) => ({
      orderId: `ord-${s.n}`,
      orderNumber: s.n,
      status: 'paid' as OrderStatus,
      paidAt: new Date(base.getTime() - s.h * 3600_000).toISOString(),
      stuckForHours: s.h,
      totalCents: s.c,
      customerName: s.who,
    })),
    alerts: buildAlerts(base),
  }
}

// ---------------------------------------------------------------------------
// 2. Contábil
// ---------------------------------------------------------------------------

/** Taxas efetivas por método — Pix é barato, crédito é caro, dinheiro é zero. */
const FEE_RATE: Record<PaymentMethod, number> = {
  pix: 0.0099,
  card: 0.0439,
  credit_card: 0.0439,
  boleto: 0.0289,
  debit_card: 0.0219,
  cash: 0,
}

const METHOD_SHARE: Record<PaymentMethod, number> = {
  // `card` fica fora da demonstração: o mock usa a granularidade
  // crédito/débito, que é a do funil. Somar os dois contaria em dobro.
  card: 0,
  pix: 0.44,
  credit_card: 0.31,
  boleto: 0.11,
  debit_card: 0.09,
  cash: 0.05,
}

export function mockAccountingSummary(period: AdminPeriod, base = new Date()): AccountingSummary {
  const series = buildSeries(30, 4242, 41_800_00 / 30, base)
  const grossCents = series.reduce((a, p) => a + p.valueCents, 0)

  const byMethod: AccountingByMethod[] = METHODS.map((method) => {
    const g = Math.round(grossCents * METHOD_SHARE[method])
    const fee = Math.round(g * FEE_RATE[method])
    // Estorno e chargeback só existem onde há disputa: cartão e boleto.
    const refund = method === 'credit_card' ? Math.round(g * 0.021) : method === 'pix' ? Math.round(g * 0.004) : 0
    const chargeback = method === 'credit_card' ? Math.round(g * 0.0068) : 0
    return {
      method,
      grossCents: g,
      pspFeeCents: fee,
      refundCents: refund,
      chargebackCents: chargeback,
      netCents: g - fee - refund - chargeback,
      transactions: Math.round((g / 21_400) * (method === 'cash' ? 1.8 : 1)),
      effectiveFeeRate: g > 0 ? fee / g : 0,
    }
  })

  type MoneyKey = 'grossCents' | 'pspFeeCents' | 'refundCents' | 'chargebackCents' | 'netCents'
  const sum = (k: MoneyKey) => byMethod.reduce((a, m) => a + (m[k] ?? 0), 0)

  return {
    period,
    grossCents: sum('grossCents'),
    pspFeeCents: sum('pspFeeCents'),
    refundCents: sum('refundCents'),
    chargebackCents: sum('chargebackCents'),
    // O mock não simula antecipação nem repasse — a identidade contábil
    // continua fechando porque ambos entram como zero.
    anticipationFeeCents: 0,
    sellerSplitCents: 0,
    netCents: sum('netCents'),
    transactionCount: byMethod.reduce((a, m) => a + m.transactions, 0),
    byMethod,
    series,
  }
}

const ACCOUNTS: Record<LedgerEntry['kind'], { code: string; name: string }> = {
  sale: { code: '3.1.01', name: 'Receita de vendas' },
  psp_fee: { code: '4.1.03', name: 'Despesas financeiras — taxas PSP' },
  refund: { code: '3.1.09', name: 'Devoluções e estornos' },
  chargeback: { code: '4.1.07', name: 'Perdas com chargeback' },
  adjustment: { code: '3.1.99', name: 'Ajustes de receita' },
  payout: { code: '1.1.02', name: 'Banco — conta movimento' },
}

const CASH_ACCOUNT = { code: '1.1.01', name: 'Caixa e equivalentes' }

const LEDGER_KINDS: Array<LedgerEntry['kind']> = [
  'sale',
  'sale',
  'sale',
  'sale',
  'psp_fee',
  'psp_fee',
  'refund',
  'payout',
  'chargeback',
  'adjustment',
]

/**
 * Livro-razão de demonstração em partida dobrada: cada evento gera o par
 * débito/crédito, então `ledgerBalanceCents` fecha em zero — como tem que
 * fechar de verdade.
 */
export function mockLedgerEntries(count = 140, base = new Date()): LedgerEntry[] {
  const rng = makeRng(90210)
  const out: LedgerEntry[] = []
  for (let i = 0; i < count / 2; i++) {
    const kind = LEDGER_KINDS[Math.floor(rng() * LEDGER_KINDS.length)]
    const method = METHODS[Math.floor(rng() * METHODS.length)]
    const amount = Math.round((80 + rng() * 9200) * 100)
    const dayOffset = Math.floor(rng() * 30)
    const occurredAt = new Date(
      daysAgo(dayOffset, base).setHours(8 + Math.floor(rng() * 11), Math.floor(rng() * 60), 0, 0),
    ).toISOString()
    const orderNumber = `2026-${8000 + Math.floor(rng() * 900)}`
    const acct = ACCOUNTS[kind]
    const pspTx = method === 'cash' ? null : `apmx_${Math.floor(rng() * 1e9).toString(36)}`

    const shared = {
      occurredAt,
      kind,
      method,
      orderId: `ord-${orderNumber}`,
      orderNumber,
      paymentId: `pay-${Math.floor(rng() * 1e6).toString(36)}`,
      pspTransactionId: pspTx,
    }

    // Perna 1 — a conta de resultado/patrimônio da natureza do evento.
    const revenueSide = kind === 'sale'
    out.push({
      ...shared,
      id: `le-${i}-a`,
      account: acct.name,
      accountCode: acct.code,
      debitCents: revenueSide ? 0 : amount,
      creditCents: revenueSide ? amount : 0,
      memo: `${acct.name} · pedido ${orderNumber}`,
    })
    // Perna 2 — a contrapartida no caixa.
    out.push({
      ...shared,
      id: `le-${i}-b`,
      account: CASH_ACCOUNT.name,
      accountCode: CASH_ACCOUNT.code,
      debitCents: revenueSide ? amount : 0,
      creditCents: revenueSide ? 0 : amount,
      memo: `Contrapartida em caixa · pedido ${orderNumber}`,
    })
  }
  return out.sort((a, b) => b.occurredAt.localeCompare(a.occurredAt))
}

export function mockLedgerPage(
  all: LedgerEntry[],
  page: number,
  pageSize: number,
): LedgerPage {
  const start = (page - 1) * pageSize
  return {
    items: all.slice(start, start + pageSize),
    page,
    pageSize,
    total: all.length,
    totalDebitCents: all.reduce((a, e) => a + e.debitCents, 0),
    totalCreditCents: all.reduce((a, e) => a + e.creditCents, 0),
  }
}

export function mockReconciliation(period: AdminPeriod, base = new Date()): ReconciliationReport {
  const items: ReconciliationReport['items'] = [
    {
      id: 'rc-1',
      date: isoDate(daysAgo(2, base)),
      ledgerCents: 4_890_00,
      pspCents: 4_756_23,
      deltaCents: -133_77,
      type: 'fee_mismatch',
      severity: 'warn',
      pspTransactionId: 'apmx_7fk29a',
      orderNumber: '2026-8841',
      note: 'Taxa cobrada acima da tabela contratada (4,39% → 7,13%).',
    },
    {
      id: 'rc-2',
      date: isoDate(daysAgo(4, base)),
      ledgerCents: 0,
      pspCents: 1_120_00,
      deltaCents: 1_120_00,
      type: 'missing_in_ledger',
      severity: 'critical',
      pspTransactionId: 'apmx_3xb81c',
      orderNumber: null,
      note: 'Liquidação no extrato do PSP sem lançamento correspondente. Webhook perdido?',
    },
    {
      id: 'rc-3',
      date: isoDate(daysAgo(6, base)),
      ledgerCents: 12_380_00,
      pspCents: 12_082_37,
      deltaCents: -297_63,
      type: 'amount_mismatch',
      severity: 'critical',
      pspTransactionId: 'apmx_9dd40e',
      orderNumber: '2026-8817',
      note: 'Divergência de valor capturado. Verificar captura parcial.',
    },
    {
      id: 'rc-4',
      date: isoDate(daysAgo(9, base)),
      ledgerCents: 638_90,
      pspCents: 0,
      deltaCents: -638_90,
      type: 'missing_in_psp',
      severity: 'warn',
      pspTransactionId: null,
      orderNumber: '2026-8802',
      note: 'Venda registrada sem contrapartida no extrato — possível pagamento em dinheiro no balcão.',
    },
  ]
  return {
    period,
    lastRunAt: new Date(base.getTime() - 47 * 60_000).toISOString(),
    matchedCount: 1_247,
    divergentCount: items.length,
    totalDeltaCents: items.reduce((a, i) => a + i.deltaCents, 0),
    items,
  }
}

// ---------------------------------------------------------------------------
// 3. Vendedores
// ---------------------------------------------------------------------------

export function mockSellerPerformance(
  period: AdminPeriod,
  base = new Date(),
): SellerPerformanceReport {
  const rng = makeRng(777)
  const sellers: SellerPerformance[] = SELLERS.map((s, i) => {
    const orderCount = 38 + Math.floor(rng() * 190)
    const ticket = Math.round((240 + rng() * 980) * 100)
    const totalCents = orderCount * ticket
    return {
      sellerId: s.id,
      sellerName: s.name,
      storeId: s.storeId,
      storeName: STORES.find((st) => st.id === s.storeId)?.name ?? '—',
      totalCents,
      orderCount,
      avgTicketCents: ticket,
      avgDiscountPct: 0.02 + rng() * 0.16,
      avgMarginPct: 0.08 + rng() * 0.22,
      managerApprovals: Math.floor(rng() * 24),
      series: buildSeries(30, 500 + i, totalCents / 30, base),
    }
  })
  return { period, stores: STORES, sellers }
}

// ---------------------------------------------------------------------------
// 4. Trilha de auditoria
// ---------------------------------------------------------------------------

const AUDIT_TEMPLATES: Array<{
  action: AuditEvent['action']
  entityType: string
  make: (rng: () => number) => { label: string; id: string; from: string | null; to: string | null }
}> = [
  {
    action: 'price_change',
    entityType: 'produto',
    make: (rng) => {
      const from = Math.round((30 + rng() * 700) * 100)
      const to = Math.round(from * (0.82 + rng() * 0.4))
      return {
        label: 'Furadeira de impacto 850W',
        id: `SKU-${4000 + Math.floor(rng() * 900)}`,
        from: `R$ ${(from / 100).toFixed(2).replace('.', ',')}`,
        to: `R$ ${(to / 100).toFixed(2).replace('.', ',')}`,
      }
    },
  },
  {
    action: 'discount_approval',
    entityType: 'pedido',
    make: (rng) => {
      const pct = (8 + rng() * 12).toFixed(1).replace('.', ',')
      return {
        label: `Pedido 2026-${8000 + Math.floor(rng() * 900)}`,
        id: `ord-${8000 + Math.floor(rng() * 900)}`,
        from: '15,0% (teto do cargo)',
        to: `${pct}% aprovado pelo gerente`,
      }
    },
  },
  {
    action: 'order_status_change',
    entityType: 'pedido',
    make: (rng) => ({
      label: `Pedido 2026-${8000 + Math.floor(rng() * 900)}`,
      id: `ord-${8000 + Math.floor(rng() * 900)}`,
      from: 'pago',
      to: 'em separação',
    }),
  },
  {
    action: 'admin_access',
    entityType: 'painel',
    make: () => ({
      label: 'Auditoria contábil',
      id: '/admin/contabil',
      from: null,
      to: null,
    }),
  },
  {
    action: 'refund_issued',
    entityType: 'pagamento',
    make: (rng) => ({
      label: `Estorno R$ ${((50 + rng() * 1800)).toFixed(2).replace('.', ',')}`,
      id: `pay-${Math.floor(rng() * 1e6).toString(36)}`,
      from: 'capturado',
      to: 'estornado',
    }),
  },
  {
    action: 'user_role_change',
    entityType: 'usuário',
    make: () => ({
      label: 'Wanderson Lima',
      id: 'v-05',
      from: 'operador',
      to: 'gerente de loja',
    }),
  },
]

const ACTORS: Array<{ id: string; name: string; role: AuditEvent['actorRole'] }> = [
  { id: 'u-adm-1', name: 'Marlon Gomes', role: 'admin' },
  { id: 'u-adm-2', name: 'Denise Rocha', role: 'admin' },
  { id: 'v-01', name: 'Cleiton Ramos', role: 'operator' },
  { id: 'v-04', name: 'Patrícia Nogueira', role: 'operator' },
  { id: 'sys', name: 'payment-service', role: 'system' },
]

/**
 * Hash falso mas *estável e encadeado*: cada evento deriva do anterior, então
 * a UI consegue demonstrar a cadeia. Não é SHA-256 — é mock, e o cálculo real
 * pertence ao backend (ver `ChainVerification`).
 */
function fakeHash(input: string): string {
  let h1 = 0x811c9dc5
  let h2 = 0x01000193
  for (let i = 0; i < input.length; i++) {
    h1 = Math.imul(h1 ^ input.charCodeAt(i), 0x01000193) >>> 0
    h2 = Math.imul(h2 + input.charCodeAt(i) * (i + 1), 0x85ebca6b) >>> 0
  }
  return (h1.toString(16).padStart(8, '0') + h2.toString(16).padStart(8, '0')).repeat(4).slice(0, 64)
}

export function mockAuditEvents(count = 180, base = new Date()): AuditEvent[] {
  const rng = makeRng(31415)
  const out: AuditEvent[] = []
  let prevHash: string | null = null
  for (let i = 0; i < count; i++) {
    const tpl = AUDIT_TEMPLATES[Math.floor(rng() * AUDIT_TEMPLATES.length)]
    const actor = ACTORS[Math.floor(rng() * ACTORS.length)]
    const detail = tpl.make(rng)
    const sequence = i + 1
    const occurredAt = new Date(base.getTime() - i * (37 * 60_000 + Math.floor(rng() * 900_000)))
      .toISOString()
    const payload = `${sequence}|${occurredAt}|${actor.id}|${tpl.action}|${detail.id}|${prevHash ?? ''}`
    const hash = fakeHash(payload)
    out.push({
      id: `au-${sequence}`,
      sequence,
      occurredAt,
      actorId: actor.id,
      actorName: actor.name,
      actorRole: actor.role,
      action: tpl.action,
      entityType: tpl.entityType,
      entityId: detail.id,
      entityLabel: detail.label,
      fromValue: detail.from,
      toValue: detail.to,
      // Último octeto mascarado — é assim que o backend deve entregar (LGPD).
      ip: `189.${Math.floor(rng() * 255)}.${Math.floor(rng() * 255)}.xxx`,
      hash,
      prevHash,
    })
    prevHash = hash
  }
  // Mais recente primeiro na exibição; a cadeia continua crescente por `sequence`.
  return out.reverse()
}

export function mockAuditPage(all: AuditEvent[], page: number, pageSize: number): AuditEventPage {
  const start = (page - 1) * pageSize
  return { items: all.slice(start, start + pageSize), page, pageSize, total: all.length }
}

export function mockChainVerification(events: AuditEvent[], base = new Date()): ChainVerification {
  const head = events[0]
  return {
    valid: true,
    checkedCount: events.length,
    brokenAtSequence: null,
    verifiedAt: new Date(base.getTime() - 3 * 60_000).toISOString(),
    headHash: head?.hash ?? '',
  }
}

// ---------------------------------------------------------------------------
// 5. Observabilidade
// ---------------------------------------------------------------------------

function latencySeries(seed: number, base: number): number[] {
  const rng = makeRng(seed)
  return Array.from({ length: 30 }, () => Math.round(base * (0.7 + rng() * 0.75)))
}

export function mockObservability(base = new Date()): ObservabilitySnapshot {
  const services: ServiceHealth[] = [
    {
      name: 'auth',
      status: 'ok',
      up: true,
      p50Ms: 18,
      p95Ms: 61,
      p99Ms: 143,
      errorRate: 0.0007,
      rpm: 412,
      uptimePct: 0.9998,
      version: '1.14.2',
      latencySeries: latencySeries(11, 61),
    },
    {
      name: 'catalog',
      status: 'warn',
      up: true,
      p50Ms: 74,
      p95Ms: 412,
      p99Ms: 918,
      errorRate: 0.0042,
      rpm: 1_884,
      uptimePct: 0.9991,
      version: '2.3.0',
      latencySeries: latencySeries(22, 412),
    },
    {
      name: 'order',
      status: 'ok',
      up: true,
      p50Ms: 31,
      p95Ms: 104,
      p99Ms: 268,
      errorRate: 0.0011,
      rpm: 246,
      uptimePct: 0.9997,
      version: '1.9.7',
      latencySeries: latencySeries(33, 104),
    },
    {
      name: 'payment',
      status: 'critical',
      up: true,
      p50Ms: 96,
      p95Ms: 640,
      p99Ms: 2_140,
      errorRate: 0.0218,
      rpm: 158,
      uptimePct: 0.9964,
      version: '3.1.1',
      latencySeries: latencySeries(44, 640),
    },
  ]
  const pending = 184
  const oldestAgeSeconds = 22 * 60
  return {
    collectedAt: base.toISOString(),
    services,
    outbox: {
      pending,
      failed: 7,
      oldestAgeSeconds,
      publishedPerMinute: 63,
      severity: outboxSeverity(pending, oldestAgeSeconds),
    },
    alerts: buildAlerts(base),
  }
}
