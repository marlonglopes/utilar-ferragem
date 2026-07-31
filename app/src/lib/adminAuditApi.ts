// Cliente admin da TRILHA de atividade (CloudTrail): "quem fez o quê, quando".
// Consome o catalog-service (GET /admin/audit sobre catalog_audit_log). Mock
// quando VITE_CATALOG_URL vazio (modo demo).
import { adminGet } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
export const isAuditActivityEnabled = CATALOG_URL !== ''

export interface FieldChange {
  old?: unknown
  new?: unknown
}

export interface AuditEntry {
  id: string
  actorId: string | null
  actorRole: string | null
  action: string
  entity: string
  entityId: string | null
  changes: Record<string, FieldChange>
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
  page?: number
  perPage?: number
}

function query(q: AuditQuery): string {
  const sp = new URLSearchParams()
  if (q.action) sp.set('action', q.action)
  if (q.actor) sp.set('actor', q.actor)
  if (q.entity) sp.set('entity', q.entity)
  sp.set('page', String(q.page ?? 1))
  sp.set('per_page', String(q.perPage ?? 30))
  return `?${sp.toString()}`
}

export async function fetchAuditActivity(q: AuditQuery): Promise<AuditPage> {
  if (!isAuditActivityEnabled) return mockAudit(q)
  return adminGet<AuditPage>(CATALOG_URL, `/api/v1/admin/audit${query(q)}`)
}

// Rótulos amigáveis das ações reais que a trilha grava.
export const AUDIT_ACTION_LABEL: Record<string, string> = {
  'product.create': 'Produto criado',
  'product.update': 'Produto editado',
  'product.archive': 'Produto arquivado',
  'product.images.upload': 'Imagem enviada',
  'product.images.cover': 'Capa definida',
  'product.images.reorder': 'Imagens reordenadas',
  'product.import': 'Produto importado',
  'product.price_tiers': 'Preço por quantidade',
  'import.batch.commit': 'Lote importado',
  'import.batch.dryrun': 'Lote (simulação)',
  'import.profile.create': 'Perfil de importação',
  'review.published': 'Avaliação publicada',
}

// ---------------------------------------------------------------------------
// Mock (modo demo). Determinístico.
// ---------------------------------------------------------------------------

const MOCK: AuditEntry[] = [
  {
    id: '1',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.update',
    entity: 'product',
    entityId: 'p-100',
    changes: { price: { old: 299.9, new: 249.9 }, stock: { old: 15, new: 8 } },
    requestId: 'req-1',
    createdAt: '2026-07-30T10:12:00Z',
  },
  {
    id: '2',
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
    id: '3',
    actorId: 'ingestor',
    actorRole: 'admin',
    action: 'product.images.upload',
    entity: 'product',
    entityId: 'p-102',
    changes: {},
    requestId: 'req-3',
    createdAt: '2026-07-29T18:02:00Z',
  },
  {
    id: '4',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.create',
    entity: 'product',
    entityId: 'p-103',
    changes: { status: { old: null, new: 'draft' } },
    requestId: 'req-4',
    createdAt: '2026-07-29T14:20:00Z',
  },
  {
    id: '5',
    actorId: 'admin@utilar.com.br',
    actorRole: 'admin',
    action: 'product.archive',
    entity: 'product',
    entityId: 'p-104',
    changes: { status: { old: 'published', new: 'archived' } },
    requestId: 'req-5',
    createdAt: '2026-07-28T16:00:00Z',
  },
]

function mockAudit(q: AuditQuery): AuditPage {
  let rows = MOCK
  if (q.action) rows = rows.filter((r) => r.action === q.action)
  if (q.actor) {
    const needle = q.actor.toLowerCase()
    rows = rows.filter((r) => (r.actorId ?? '').toLowerCase().includes(needle))
  }
  if (q.entity) rows = rows.filter((r) => r.entity === q.entity)
  const perPage = q.perPage ?? 30
  return {
    data: rows,
    meta: {
      page: q.page ?? 1,
      per_page: perPage,
      total: rows.length,
      total_pages: Math.max(1, Math.ceil(rows.length / perPage)),
    },
  }
}
