// Cliente admin da TRILHA de atividade unificada (CloudTrail): "quem fez o quê,
// quando" — juntando as três fontes de auditoria, cada uma no seu serviço/banco:
//   - catalog  (catalog_audit_log):   produto, preço, estoque, import, review
//   - staff    (auth store_audit):    operador, teto de desconto, MUDANÇA DE PAPEL
//   - operação (order balcao+return): PDV, desconto, aprovação, devolução, ESTORNO
//
// Não há join entre bancos (são separados por serviço): buscamos em paralelo e
// mesclamos no cliente. Uma fonte que dá 403 (persona sem acesso àquele serviço)
// ou cai é PULADA — o admin vê tudo; o vendas vê só o catálogo; ninguém trava.
import { adminGet } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
const AUTH_URL = import.meta.env.VITE_AUTH_URL ?? ''
const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''

export type AuditSource = 'catalog' | 'staff' | 'operacao'

// Cada serviço tem um PATH distinto de propósito: no modo túnel (single-origin)
// todas as VITE_*_URL são a mesma origem e o proxy do vite roteia por PATH — se
// os três usassem /admin/audit, o proxy não saberia para qual serviço mandar.
// Por isso order = /admin/order-audit e auth = /admin/staff-audit. Ver vite.config.ts.
const SOURCES: ReadonlyArray<{ key: AuditSource; label: string; base: string; path: string }> = [
  { key: 'catalog', label: 'Catálogo', base: CATALOG_URL, path: '/api/v1/admin/audit' },
  { key: 'staff', label: 'Staff', base: AUTH_URL, path: '/api/v1/admin/staff-audit' },
  { key: 'operacao', label: 'Operação', base: ORDER_URL, path: '/api/v1/admin/order-audit' },
]

export const SOURCE_LABEL: Record<AuditSource, string> = {
  catalog: 'Catálogo',
  staff: 'Staff',
  operacao: 'Operação',
}

// Habilitado se QUALQUER fonte tem URL. Sem nenhuma → modo demo (mock).
export const isAuditActivityEnabled = SOURCES.some((s) => s.base !== '')

export interface FieldChange {
  old?: unknown
  new?: unknown
}

export interface AuditEntry {
  id: string
  source: AuditSource
  actorId: string | null
  actorRole: string | null
  action: string
  entity: string
  entityId: string | null
  changes: Record<string, FieldChange>
  amount?: number | null
  requestId: string | null
  createdAt: string
}

export interface AuditPage {
  data: AuditEntry[]
  meta: { page: number; per_page: number; total: number; total_pages: number }
}

export interface AuditQuery {
  action?: string
  actor?: string
  entity?: string
  source?: AuditSource | ''
  page?: number
  perPage?: number
}

// Quanto puxar de CADA fonte antes de mesclar. A mesclagem+ordenação é no
// cliente (bancos separados não dão join), então paginamos sobre a janela. Um
// teto generoso cobre a leitura de atividade recente; ir além é raro e fica
// para um filtro por período/ator.
const SOURCE_CAP = 100

function serverQuery(q: AuditQuery): string {
  const sp = new URLSearchParams()
  if (q.action) sp.set('action', q.action)
  if (q.actor) sp.set('actor', q.actor)
  if (q.entity) sp.set('entity', q.entity)
  sp.set('page', '1')
  sp.set('per_page', String(SOURCE_CAP))
  return `?${sp.toString()}`
}

type RawPage = { data: Omit<AuditEntry, 'source'>[] }

export async function fetchAuditActivity(q: AuditQuery): Promise<AuditPage> {
  if (!isAuditActivityEnabled) return mockAudit(q)

  const wanted = SOURCES.filter((s) => s.base !== '' && (!q.source || q.source === s.key))
  const perSource = await Promise.all(
    wanted.map(async (s) => {
      try {
        const pg = await adminGet<RawPage>(s.base, `${s.path}${serverQuery(q)}`)
        return pg.data.map((r) => ({ ...r, source: s.key }))
      } catch {
        // 403 (persona sem acesso a esta fonte) ou rede: pula a fonte, não trava.
        return [] as AuditEntry[]
      }
    })
  )

  const merged = perSource.flat().sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  const perPage = q.perPage ?? 30
  const page = Math.max(1, q.page ?? 1)
  const total = merged.length
  const start = (page - 1) * perPage
  return {
    data: merged.slice(start, start + perPage),
    meta: {
      page,
      per_page: perPage,
      total,
      total_pages: Math.max(1, Math.ceil(total / perPage)),
    },
  }
}

// Rótulos amigáveis das ações reais que as três trilhas gravam.
export const AUDIT_ACTION_LABEL: Record<string, string> = {
  // catálogo
  'product.create': 'Produto criado',
  'product.update': 'Produto editado',
  'product.archive': 'Produto arquivado',
  'product.images.upload': 'Imagem enviada',
  'product.images.cover': 'Capa definida',
  'product.images.reorder': 'Imagens reordenadas',
  'product.import': 'Produto importado',
  'product.price_tiers': 'Preço por quantidade',
  'stock.adjust': 'Estoque ajustado',
  'import.batch.commit': 'Lote importado',
  'import.batch.dryrun': 'Lote (simulação)',
  'import.profile.create': 'Perfil de importação',
  'review.published': 'Avaliação publicada',
  // staff (auth)
  'user.role.update': 'Papel alterado',
  'operator.created': 'Operador criado',
  'operator.level_changed': 'Cargo do operador alterado',
  'operator.updated': 'Operador editado',
  // operação (order)
  'order.created': 'Pedido de balcão criado',
  'discount.applied': 'Desconto aplicado',
  'discount.approved': 'Desconto aprovado',
  'discount.rejected': 'Desconto recusado',
  'discount.capped': 'Desconto no teto do cargo',
  'order.cancelled': 'Pedido cancelado',
  'return.requested': 'Devolução solicitada',
  'return.approved': 'Devolução aprovada',
  'return.rejected': 'Devolução recusada',
  'return.received': 'Devolução recebida',
  'return.refunded': 'Estorno feito',
  'return.stock_restored': 'Estoque devolvido',
}

// ---------------------------------------------------------------------------
// Mock (modo demo). Determinístico, com as TRÊS fontes representadas.
// ---------------------------------------------------------------------------

const MOCK: AuditEntry[] = [
  {
    id: '1',
    source: 'catalog',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.update',
    entity: 'product',
    entityId: 'p-100',
    changes: { price: { old: 299.9, new: 249.9 }, stock: { old: 15, new: 8 } },
    requestId: 'req-1',
    createdAt: '2026-07-31T10:12:00Z',
  },
  {
    id: 's1',
    source: 'staff',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'user.role.update',
    entity: 'user',
    entityId: 'u-200',
    changes: { role: { old: 'customer', new: 'vendas' } },
    requestId: 'req-s1',
    createdAt: '2026-07-31T09:55:00Z',
  },
  {
    id: 'o1',
    source: 'operacao',
    actorId: 'op-centro@utilar.com.br',
    actorRole: 'store_operator',
    action: 'discount.applied',
    entity: 'discount',
    entityId: 'ord-300',
    changes: { discountPct: { old: 0, new: 12 } },
    amount: 34.5,
    requestId: 'req-o1',
    createdAt: '2026-07-31T09:40:00Z',
  },
  {
    id: 'o2',
    source: 'operacao',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'return.refunded',
    entity: 'return',
    entityId: 'ord-280',
    changes: { status: { old: 'received', new: 'refunded' } },
    amount: 129.9,
    requestId: 'req-o2',
    createdAt: '2026-07-30T17:20:00Z',
  },
  {
    id: '2',
    source: 'catalog',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.update',
    entity: 'product',
    entityId: 'p-101',
    changes: { status: { old: 'draft', new: 'published' } },
    requestId: 'req-2',
    createdAt: '2026-07-30T09:40:00Z',
  },
  {
    id: '4',
    source: 'catalog',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.create',
    entity: 'product',
    entityId: 'p-103',
    changes: { status: { old: null, new: 'draft' } },
    requestId: 'req-4',
    createdAt: '2026-07-29T14:20:00Z',
  },
]

function mockAudit(q: AuditQuery): AuditPage {
  let rows = MOCK
  if (q.source) rows = rows.filter((r) => r.source === q.source)
  if (q.action) rows = rows.filter((r) => r.action === q.action)
  if (q.entity) rows = rows.filter((r) => r.entity === q.entity)
  if (q.actor) {
    const needle = q.actor.toLowerCase()
    rows = rows.filter((r) => (r.actorId ?? '').toLowerCase().includes(needle))
  }
  rows = [...rows].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  const perPage = q.perPage ?? 30
  const page = Math.max(1, q.page ?? 1)
  const start = (page - 1) * perPage
  return {
    data: rows.slice(start, start + perPage),
    meta: {
      page,
      per_page: perPage,
      total: rows.length,
      total_pages: Math.max(1, Math.ceil(rows.length / perPage)),
    },
  }
}
