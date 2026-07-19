/**
 * Contrato das APIs do dashboard administrativo.
 *
 * IMPORTANTE — este arquivo é a fonte da verdade do contrato enquanto o backend
 * não existe. Quando `docs/admin-dashboard-api.md` for implementado pelos
 * serviços, estes tipos devem casar campo a campo. Divergência aqui = bug lá.
 *
 * CONVENÇÕES (valem para todo o contrato):
 *
 * - **Dinheiro é inteiro em centavos** (`*Cents`). Nunca float — soma de
 *   `number` em reais acumula erro e auditoria contábil não tolera centavo
 *   perdido. `1234` = R$ 12,34.
 * - **Percentual é fração** no intervalo 0..1 (`0.0342` = 3,42%), não 0..100.
 * - **Datas são ISO-8601 UTC** com timezone explícito (`2026-07-18T13:05:00Z`).
 *   O front converte para America/Sao_Paulo na exibição.
 * - **Período** sempre `from`/`to` inclusivos, formato `YYYY-MM-DD`.
 * - Toda rota abaixo exige `Authorization: Bearer <jwt>` **com papel `admin`
 *   verificado no servidor**. O guard do front é conveniência de navegação, não
 *   fronteira de segurança.
 */

// ---------------------------------------------------------------------------
// Comuns
// ---------------------------------------------------------------------------

/** Janela de consulta. Inclusiva nos dois extremos. */
export interface AdminPeriod {
  from: string // YYYY-MM-DD
  to: string // YYYY-MM-DD
}

/** Presets do seletor de período — resolvidos no front para `AdminPeriod`. */
export type AdminPeriodPreset = 'today' | 'week' | 'month' | 'quarter' | 'year'

/** Severidade semântica. Dirige cor E forma (pill/faixa) na UI. */
export type Severity = 'ok' | 'warn' | 'critical'

export interface Paginated<T> {
  items: T[]
  page: number // 1-based
  pageSize: number
  total: number // total de itens, não de páginas
}

/**
 * Métodos como o payment-service os grava (`ledger_entries.payment_method`).
 * `card` é o valor real do backend — não desdobra em crédito/débito no livro,
 * e o front NÃO inventa a distinção. `credit_card`/`debit_card`/`cash` existem
 * para o funil do order-service e para o balcão.
 */
export type PaymentMethod =
  | 'pix'
  | 'boleto'
  | 'card'
  | 'credit_card'
  | 'debit_card'
  | 'cash'

export type OrderStatus =
  | 'pending'
  | 'paid'
  | 'picking'
  | 'shipped'
  | 'delivered'
  | 'canceled'
  | 'refunded'

// ---------------------------------------------------------------------------
// 1. Visão geral — GET {ORDER_URL}/api/v1/admin/overview?from&to
// ---------------------------------------------------------------------------

/** Ponto de uma série temporal diária (sparkline / barras). */
export interface TimePoint {
  date: string // YYYY-MM-DD
  valueCents: number
  orders: number
}

export interface OverviewKpis {
  /** Vendas líquidas do dia corrente (fuso da loja), já descontados estornos. */
  todayCents: number
  weekCents: number
  monthCents: number
  /** Mesmo recorte do período anterior equivalente — habilita o delta "vs.". */
  todayPrevCents: number
  weekPrevCents: number
  monthPrevCents: number
  /** Ticket médio do período do filtro. */
  avgTicketCents: number
  avgTicketPrevCents: number
  orderCount: number
  orderCountPrev: number
}

export interface StatusBucket {
  status: OrderStatus
  count: number
  valueCents: number
}

/**
 * Funil de pagamento. `created` = intenções criadas; `confirmed` = capturadas.
 * A taxa de conversão é derivada no front (`confirmed / created`) — o backend
 * NÃO deve mandar a razão pré-calculada, para que o front possa recalcular ao
 * cruzar filtros sem uma segunda chamada.
 */
export interface PaymentFunnel {
  created: number
  confirmed: number
  failed: number
  expired: number
  /** Quebra por método — mesma semântica dos campos acima. */
  byMethod: Array<{
    method: PaymentMethod
    created: number
    confirmed: number
  }>
}

/**
 * Pedido travado: pagamento confirmado mas o pedido não avançou.
 * É o sintoma nº 1 de outbox parado — ver `OutboxStats`.
 */
export interface StuckOrder {
  orderId: string
  orderNumber: string
  status: OrderStatus
  paidAt: string // ISO
  /** Horas desde `paidAt`. Calculado no backend para não depender do relógio do cliente. */
  stuckForHours: number
  totalCents: number
  customerName: string
}

export interface AdminOverview {
  period: AdminPeriod
  kpis: OverviewKpis
  /** Série diária do período — alimenta o gráfico de barras e as sparklines. */
  series: TimePoint[]
  byStatus: StatusBucket[]
  funnel: PaymentFunnel
  stuckOrders: StuckOrder[]
  alerts: Alert[]
}

// ---------------------------------------------------------------------------
// 2. Auditoria contábil — payment-service
// ---------------------------------------------------------------------------

/**
 * GET {API_URL}/api/v1/admin/accounting/summary?from&to
 *
 * Identidade que o backend DEVE garantir (o front assere e destaca se quebrar):
 *   netCents = grossCents - pspFeeCents - refundCents - chargebackCents
 */
export interface AccountingSummary {
  period: AdminPeriod
  grossCents: number
  pspFeeCents: number
  refundCents: number
  chargebackCents: number
  /** Taxa de antecipação de recebíveis (conta 4.1.2). */
  anticipationFeeCents: number
  /** Repasses a vendedores já reconhecidos (conta 4.2.1). */
  sellerSplitCents: number
  netCents: number
  transactionCount: number
  /** Mesma estrutura recortada por método de pagamento. */
  byMethod: AccountingByMethod[]
  /** Série diária de receita líquida — sparkline do topo. */
  series: TimePoint[]
}

export interface AccountingByMethod {
  method: PaymentMethod
  grossCents: number
  pspFeeCents: number
  refundCents: number
  /**
   * `null` quando a fonte não recorta chargeback por método — é o caso do
   * `/api/v1/ledger/by-method`. Exibir 0 ali seria afirmar "não houve
   * chargeback neste método", que é diferente de "não sabemos".
   */
  chargebackCents: number | null
  netCents: number
  transactions: number
  /** Taxa efetiva do PSP no método (fração 0..1). Derivável, mas vem pronta
   *  porque o PSP cobra fixo + percentual e a razão simples mentiria. */
  effectiveFeeRate: number
}

/** Um lançamento do livro-razão. Partida dobrada: todo evento gera 2+ linhas. */
export interface LedgerEntry {
  id: string
  /** ISO. Data de competência contábil, não de criação da linha. */
  occurredAt: string
  /** Conta contábil, ex. `1.1.01 Caixa`, `3.1.01 Receita de vendas`. */
  account: string
  accountCode: string
  /** Exatamente um dos dois é > 0; o outro é 0. */
  debitCents: number
  creditCents: number
  /** Natureza do lançamento — dirige o chip na tabela. */
  kind: 'sale' | 'psp_fee' | 'refund' | 'chargeback' | 'adjustment' | 'payout'
  method: PaymentMethod | null
  /** Rastro até a origem — a UI linka para o pedido. */
  orderId: string | null
  orderNumber: string | null
  paymentId: string | null
  /** ID da transação no PSP, para bater com o extrato do Appmax. */
  pspTransactionId: string | null
  memo: string
}

/**
 * GET {API_URL}/api/v1/admin/accounting/entries?from&to&kind&method&page&pageSize
 * Ordenação padrão: `occurredAt` desc. Resposta: `Paginated<LedgerEntry>` +
 * os totais do FILTRO INTEIRO (não da página) para o rodapé da tabela.
 */
export interface LedgerPage extends Paginated<LedgerEntry> {
  totalDebitCents: number
  totalCreditCents: number
}

/**
 * GET {API_URL}/api/v1/admin/accounting/reconciliation?from&to
 * Divergência entre o que o payment-service registrou e o extrato do PSP.
 */
export interface ReconciliationItem {
  id: string
  date: string // YYYY-MM-DD
  /** O que nosso livro diz. */
  ledgerCents: number
  /** O que o extrato do PSP diz. */
  pspCents: number
  /** `pspCents - ledgerCents`. Positivo = PSP tem a mais. */
  deltaCents: number
  type: 'missing_in_ledger' | 'missing_in_psp' | 'amount_mismatch' | 'fee_mismatch'
  severity: Severity
  pspTransactionId: string | null
  orderNumber: string | null
  note: string
}

export interface ReconciliationReport {
  period: AdminPeriod
  /** Quando o job de reconciliação rodou pela última vez (ISO). */
  lastRunAt: string
  matchedCount: number
  divergentCount: number
  totalDeltaCents: number
  items: ReconciliationItem[]
}

/**
 * GET {API_URL}/api/v1/admin/accounting/export?from&to&format=csv|ofx
 *
 * Responde `200` com `Content-Type: text/csv` (ou `application/x-ofx`) e
 * `Content-Disposition: attachment`. **Não** responde JSON com base64 — o
 * arquivo pode ter centenas de milhares de linhas e não deve passar pela
 * memória do browser duas vezes.
 *
 * Enquanto o backend não existe, o front gera o CSV localmente a partir do
 * livro carregado (ver `adminExport.ts`). O cabeçalho do CSV local é
 * exatamente o que o backend deve emitir, para o contador não ver diferença.
 */
export type ExportFormat = 'csv' | 'ofx'

// ---------------------------------------------------------------------------
// 3. Desempenho dos vendedores — order-service
// ---------------------------------------------------------------------------

/**
 * GET {ORDER_URL}/api/v1/admin/sellers/performance?from&to&storeId
 *
 * DEPENDÊNCIA: exige o papel de operador e a atribuição de pedido que outro
 * agente está criando no backend. Sem `orders.operator_id` populado, esta rota
 * não tem como agrupar.
 *
 * SEGURANÇA: `avgMarginPct` é derivado do custo do produto. O backend deve
 * devolver **a margem já agregada**, nunca o custo unitário por item — custo de
 * aquisição não deve trafegar para o navegador.
 */
export interface SellerPerformance {
  sellerId: string
  sellerName: string
  storeId: string
  storeName: string
  totalCents: number
  orderCount: number
  avgTicketCents: number
  /** Desconto médio concedido, fração 0..1 do valor bruto. */
  avgDiscountPct: number
  /** Margem média realizada, fração 0..1. Já agregada no backend. */
  avgMarginPct: number
  /** Pedidos que estouraram o teto de desconto do cargo e foram ao gerente. */
  managerApprovals: number
  /** Série diária de vendas do vendedor — sparkline da linha da tabela. */
  series: TimePoint[]
}

export interface SellerPerformanceReport {
  period: AdminPeriod
  stores: Array<{ id: string; name: string }>
  sellers: SellerPerformance[]
}

// ---------------------------------------------------------------------------
// 4. Trilha de auditoria — auth-service
// ---------------------------------------------------------------------------

export type AuditAction =
  | 'price_change'
  | 'discount_approval'
  | 'order_status_change'
  | 'admin_access'
  | 'refund_issued'
  | 'user_role_change'

/**
 * Um evento da trilha. **Imutável e encadeado por hash**: cada registro carrega
 * o hash do anterior, de modo que alterar uma linha antiga invalida toda a
 * cadeia dali em diante. O backend está implementando assim; a UI exibe o
 * estado da verificação (ver `ChainVerification`).
 */
export interface AuditEvent {
  id: string
  /** Posição na cadeia, 1-based e contígua. Buraco = adulteração. */
  sequence: number
  occurredAt: string // ISO
  actorId: string
  actorName: string
  actorRole: 'admin' | 'seller' | 'operator' | 'system'
  action: AuditAction
  /** Tipo + id da coisa alterada, ex. `product` / `SKU-4471`. */
  entityType: string
  entityId: string
  entityLabel: string
  /** Valor anterior e novo, já serializados como texto de exibição.
   *  `null` em ações que não são mudança de valor (ex. `admin_access`). */
  fromValue: string | null
  toValue: string | null
  /** IP de origem — mascarado no último octeto pelo backend (LGPD). */
  ip: string
  /** SHA-256 hex deste registro e do anterior. `prevHash` do primeiro é null. */
  hash: string
  prevHash: string | null
}

/**
 * GET {AUTH_URL}/api/v1/admin/audit/verify?from&to
 * Recalcula a cadeia no servidor. É a única fonte de verdade da integridade —
 * o front NÃO tenta verificar hash no cliente (não teria como confiar no
 * resultado, e o cálculo em JS sobre milhares de eventos travaria a aba).
 */
export interface ChainVerification {
  valid: boolean
  checkedCount: number
  /** Sequence do primeiro registro que não bate. `null` se a cadeia está íntegra. */
  brokenAtSequence: number | null
  verifiedAt: string // ISO
  /** Hash do último registro — o "âncora" que pode ser publicado externamente. */
  headHash: string
}

/** GET {AUTH_URL}/api/v1/admin/audit/events?from&to&actorId&entityType&action&page&pageSize */
export type AuditEventPage = Paginated<AuditEvent>

// ---------------------------------------------------------------------------
// 5. Observabilidade
// ---------------------------------------------------------------------------

export interface ServiceHealth {
  name: 'auth' | 'catalog' | 'order' | 'payment'
  status: Severity
  up: boolean
  /** Latência em ms. p50/p95/p99 da janela de 5 min. */
  p50Ms: number
  p95Ms: number
  p99Ms: number
  /** Fração 0..1 de respostas 5xx na janela. */
  errorRate: number
  /** Requisições por minuto. */
  rpm: number
  uptimePct: number
  version: string
  /** Série curta de p95 (últimos 30 pontos) — sparkline. */
  latencySeries: number[]
}

/**
 * Fila do outbox do payment-service. Se `pending` cresce ou `oldestAgeSeconds`
 * dispara, pagamento confirmado não está virando pedido pago — é o mesmo
 * incidente que aparece em `AdminOverview.stuckOrders`.
 */
export interface OutboxStats {
  pending: number
  failed: number
  /** Idade em segundos do evento pendente mais antigo. 0 se a fila está vazia. */
  oldestAgeSeconds: number
  /** Eventos publicados por minuto (vazão do relay). */
  publishedPerMinute: number
  severity: Severity
}

export interface Alert {
  id: string
  severity: Severity
  title: string
  detail: string
  /** Origem: qual serviço/subsistema disparou. */
  source: string
  firedAt: string // ISO
  /** Rota do dashboard que investiga este alerta, ex. `/admin/observabilidade`. */
  href?: string
}

export interface ObservabilitySnapshot {
  /** ISO — quando o snapshot foi coletado. */
  collectedAt: string
  services: ServiceHealth[]
  outbox: OutboxStats
  alerts: Alert[]
}
