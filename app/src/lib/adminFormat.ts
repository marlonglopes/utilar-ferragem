import type {
  AccountingSummary,
  Alert,
  LedgerEntry,
  PaymentFunnel,
  PaymentMethod,
  SellerPerformance,
  Severity,
  TimePoint,
} from '@/lib/adminTypes'

/**
 * Formatação e agregados do dashboard admin.
 *
 * Tudo aqui é função pura sobre centavos inteiros. Dinheiro nunca vira float
 * antes da última divisão por 100 na formatação — é o único ponto onde a
 * imprecisão de ponto flutuante não consegue mais se acumular.
 */

const BRL = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

/** R$ 12,34 a partir de 1234. Negativos vêm com o sinal (−R$ 12,34). */
export function formatCents(cents: number): string {
  return BRL.format((Number.isFinite(cents) ? cents : 0) / 100)
}

/**
 * Forma compacta para KPI grande: R$ 12,3 mil / R$ 1,24 mi.
 * Abaixo de mil mostra o valor cheio — "R$ 0,9 mil" seria pior que "R$ 940,00".
 */
export function formatCentsCompact(cents: number): string {
  const abs = Math.abs(cents)
  if (abs < 100_000) return formatCents(cents)
  const sign = cents < 0 ? '-' : ''
  const reais = abs / 100
  if (reais < 1_000_000) {
    return `${sign}R$ ${(reais / 1000).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} mil`
  }
  return `${sign}R$ ${(reais / 1_000_000).toLocaleString('pt-BR', { maximumFractionDigits: 2 })} mi`
}

/**
 * Percentual a partir de uma FRAÇÃO (0.0342 → "3,4%").
 * O contrato inteiro usa fração 0..1; receber 0..100 aqui é bug de contrato.
 */
export function formatPercent(fraction: number, digits = 1): string {
  if (!Number.isFinite(fraction)) return '—'
  return `${(fraction * 100).toLocaleString('pt-BR', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })}%`
}

/** Delta assinado para comparação com o período anterior: "+12,4%" / "−3,0%". */
export function formatDelta(fraction: number, digits = 1): string {
  if (!Number.isFinite(fraction)) return '—'
  const sign = fraction > 0 ? '+' : fraction < 0 ? '−' : ''
  return `${sign}${Math.abs(fraction * 100).toLocaleString('pt-BR', {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })}%`
}

/** Contagem simples com separador de milhar. */
export function formatCount(n: number): string {
  return (Number.isFinite(n) ? n : 0).toLocaleString('pt-BR')
}

/** Duração legível a partir de segundos: "45s", "12min", "3h 20min", "2d 4h". */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const s = Math.floor(seconds)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}min`
  const h = Math.floor(m / 60)
  if (h < 24) {
    const rm = m % 60
    return rm === 0 ? `${h}h` : `${h}h ${rm}min`
  }
  const d = Math.floor(h / 24)
  const rh = h % 24
  return rh === 0 ? `${d}d` : `${d}d ${rh}h`
}

/** Latência: sub-ms não existe na prática, então inteiro em ms até 1s, depois s. */
export function formatLatency(ms: number): string {
  if (!Number.isFinite(ms)) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toLocaleString('pt-BR', { maximumFractionDigits: 2 })}s`
}

/** Hash longo vira `a1b2c3d4…9f8e7d6c` — o suficiente para conferir a olho. */
export function shortHash(hash: string | null): string {
  if (!hash) return '—'
  if (hash.length <= 20) return hash
  return `${hash.slice(0, 8)}…${hash.slice(-8)}`
}

const METHOD_LABELS: Record<PaymentMethod, string> = {
  pix: 'Pix',
  boleto: 'Boleto',
  // O livro-razão grava só `card`; crédito/débito vêm do funil e do balcão.
  card: 'Cartão',
  credit_card: 'Crédito',
  debit_card: 'Débito',
  cash: 'Dinheiro',
}

export function methodLabel(method: PaymentMethod | null): string {
  return method ? METHOD_LABELS[method] : '—'
}

const KIND_LABELS: Record<LedgerEntry['kind'], string> = {
  sale: 'Venda',
  psp_fee: 'Taxa PSP',
  refund: 'Estorno',
  chargeback: 'Chargeback',
  adjustment: 'Ajuste',
  payout: 'Repasse',
}

export function ledgerKindLabel(kind: LedgerEntry['kind']): string {
  return KIND_LABELS[kind]
}

// ---------------------------------------------------------------------------
// Agregados
// ---------------------------------------------------------------------------

/**
 * Variação relativa entre dois períodos.
 *
 * Base zero não tem variação definida: 0 → 100 não é "+∞%" nem "+100%", é
 * "não comparável". Devolve `null` para a UI mostrar "—" em vez de um número
 * inventado que o dono usaria para tomar decisão.
 */
export function pctChange(current: number, previous: number): number | null {
  if (!Number.isFinite(current) || !Number.isFinite(previous)) return null
  if (previous === 0) return null
  return (current - previous) / Math.abs(previous)
}

/** Taxa de conversão do funil de pagamento: confirmados / criados. */
export function conversionRate(funnel: Pick<PaymentFunnel, 'created' | 'confirmed'>): number | null {
  if (funnel.created <= 0) return null
  return funnel.confirmed / funnel.created
}

/** Ticket médio em centavos, arredondado. Sem pedidos, devolve 0. */
export function avgTicketCents(totalCents: number, orderCount: number): number {
  if (orderCount <= 0) return 0
  return Math.round(totalCents / orderCount)
}

/** Soma de uma série diária. */
export function sumSeries(series: TimePoint[]): number {
  return series.reduce((acc, p) => acc + p.valueCents, 0)
}

/**
 * Confere a identidade contábil do resumo.
 *
 * A forma canônica, segundo o payment-service, é:
 *
 *   net = bruto − taxas PSP − estornos − chargebacks − repasses
 *
 * **Ambiguidade conhecida:** a taxa de antecipação (conta 4.1.2) é uma despesa
 * financeira, mas a documentação do `Summary` não a lista na fórmula do
 * líquido. Como um banner vermelho falso ("o resumo não fecha") é pior que
 * nenhuma checagem — destruiria a confiança na única tela que existe para
 * gerar confiança — a verificação aceita as DUAS leituras e só acusa
 * desequilíbrio quando nenhuma delas fecha. Pendência registrada em
 * `docs/admin-dashboard-api.md`.
 *
 * Devolve a diferença em centavos (0 = fecha), escolhendo a menor divergência
 * entre as duas interpretações.
 */
export function accountingImbalanceCents(
  s: Pick<
    AccountingSummary,
    'grossCents' | 'pspFeeCents' | 'refundCents' | 'chargebackCents' | 'netCents'
  > &
    Partial<Pick<AccountingSummary, 'anticipationFeeCents' | 'sellerSplitCents'>>,
): number {
  const anticipation = s.anticipationFeeCents ?? 0
  const split = s.sellerSplitCents ?? 0
  const base = s.grossCents - s.pspFeeCents - s.refundCents - s.chargebackCents - split
  const withoutAnticipation = s.netCents - base
  const withAnticipation = s.netCents - (base - anticipation)
  return Math.abs(withAnticipation) < Math.abs(withoutAnticipation)
    ? withAnticipation
    : withoutAnticipation
}

/** Taxa efetiva do PSP sobre o bruto (fração). Bruto zero → null. */
export function effectiveFeeRate(grossCents: number, pspFeeCents: number): number | null {
  if (grossCents <= 0) return null
  return pspFeeCents / grossCents
}

/**
 * Partida dobrada: no livro inteiro, soma de débitos = soma de créditos.
 * Diferente de zero significa lançamento órfão — destacado na UI.
 */
export function ledgerBalanceCents(entries: LedgerEntry[]): number {
  return entries.reduce((acc, e) => acc + e.debitCents - e.creditCents, 0)
}

/**
 * Ordena vendedores. O padrão do dono é volume, mas o que ele *pediu* foi
 * separar "vende muito" de "vende bem" — por isso margem e desconto são
 * critérios de primeira classe, não colunas decorativas.
 */
export type SellerSortKey =
  | 'totalCents'
  | 'orderCount'
  | 'avgTicketCents'
  | 'avgDiscountPct'
  | 'avgMarginPct'
  | 'managerApprovals'

export function sortSellers(
  sellers: SellerPerformance[],
  key: SellerSortKey,
  dir: 'asc' | 'desc' = 'desc',
): SellerPerformance[] {
  const mult = dir === 'desc' ? -1 : 1
  return [...sellers].sort((a, b) => {
    const d = a[key] - b[key]
    if (d !== 0) return d * mult
    return a.sellerName.localeCompare(b.sellerName, 'pt-BR')
  })
}

/** Totais da tabela de vendedores — o rodapé agrega o filtro inteiro. */
export function sellerTotals(sellers: SellerPerformance[]): {
  totalCents: number
  orderCount: number
  avgTicketCents: number
  avgDiscountPct: number
  avgMarginPct: number
  managerApprovals: number
} {
  const totalCents = sellers.reduce((a, s) => a + s.totalCents, 0)
  const orderCount = sellers.reduce((a, s) => a + s.orderCount, 0)
  const managerApprovals = sellers.reduce((a, s) => a + s.managerApprovals, 0)
  // Médias ponderadas por VALOR, não simples: um vendedor com 2 pedidos não
  // pode puxar a margem da loja tanto quanto um com 200.
  const weight = totalCents
  const avgDiscountPct =
    weight > 0 ? sellers.reduce((a, s) => a + s.avgDiscountPct * s.totalCents, 0) / weight : 0
  const avgMarginPct =
    weight > 0 ? sellers.reduce((a, s) => a + s.avgMarginPct * s.totalCents, 0) / weight : 0
  return {
    totalCents,
    orderCount,
    avgTicketCents: avgTicketCents(totalCents, orderCount),
    avgDiscountPct,
    avgMarginPct,
    managerApprovals,
  }
}

/**
 * Severidade da margem. Os cortes são de ferragem: abaixo de 12% a venda não
 * paga a operação; acima de 22% está saudável.
 */
export function marginSeverity(fraction: number): Severity {
  if (fraction < 0.12) return 'critical'
  if (fraction < 0.22) return 'warn'
  return 'ok'
}

/** Severidade do desconto concedido — espelho da margem, sinal invertido. */
export function discountSeverity(fraction: number): Severity {
  if (fraction > 0.15) return 'critical'
  if (fraction > 0.08) return 'warn'
  return 'ok'
}

/**
 * Severidade da fila do outbox. Idade importa mais que tamanho: 500 eventos
 * escoando em 10s é saudável; 3 eventos parados há uma hora é incidente.
 */
export function outboxSeverity(pending: number, oldestAgeSeconds: number): Severity {
  if (oldestAgeSeconds >= 900 || pending >= 500) return 'critical'
  if (oldestAgeSeconds >= 120 || pending >= 100) return 'warn'
  return 'ok'
}

/** Ordena alertas por severidade (crítico primeiro) e depois por recência. */
export function sortAlerts(alerts: Alert[]): Alert[] {
  const rank: Record<Severity, number> = { critical: 0, warn: 1, ok: 2 }
  return [...alerts].sort((a, b) => {
    const d = rank[a.severity] - rank[b.severity]
    if (d !== 0) return d
    return b.firedAt.localeCompare(a.firedAt)
  })
}
