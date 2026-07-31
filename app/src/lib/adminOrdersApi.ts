// Cliente admin de PEDIDOS — a lista de operação que faltava no painel.
// Consome o order-service (GET /admin/orders + os PATCH de fulfillment que já
// existiam). Mock quando VITE_ORDER_URL está vazio (modo demo do painel).
import { adminGet, adminSend } from '@/lib/adminApi'

const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
export const isAdminOrdersEnabled = ORDER_URL !== ''

export type OrderStatus =
  | 'pending_payment'
  | 'paid'
  | 'picking'
  | 'shipped'
  | 'delivered'
  | 'cancelled'

export type OrderChannel = 'web' | 'balcao'

/** A ação de fulfillment mapeia direto pro PATCH do order-service. */
export type FulfillmentAction = 'picking' | 'shipped' | 'delivered' | 'cancel'

export interface AdminOrder {
  id: string
  status: OrderStatus
  channel: OrderChannel
  subtotal: number
  total: number
  customerName?: string | null
  customerDocument?: string | null
  customerPhone?: string | null
  createdAt: string
  paidAt?: string | null
}

export interface AdminOrdersPage {
  data: AdminOrder[]
  meta: { page: number; per_page: number; total: number; total_pages: number }
}

export interface AdminOrderQuery {
  status?: OrderStatus | 'active' | 'done' | ''
  channel?: OrderChannel | ''
  q?: string
  page?: number
  perPage?: number
}

function query(q: AdminOrderQuery): string {
  const sp = new URLSearchParams()
  if (q.status) sp.set('status', q.status)
  if (q.channel) sp.set('channel', q.channel)
  if (q.q) sp.set('q', q.q)
  sp.set('page', String(q.page ?? 1))
  sp.set('per_page', String(q.perPage ?? 20))
  return `?${sp.toString()}`
}

export async function fetchAdminOrders(q: AdminOrderQuery): Promise<AdminOrdersPage> {
  if (!isAdminOrdersEnabled) return mockOrders(q)
  return adminGet<AdminOrdersPage>(ORDER_URL, `/api/v1/admin/orders${query(q)}`)
}

export async function runFulfillment(id: string, action: FulfillmentAction): Promise<void> {
  if (!isAdminOrdersEnabled) return
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/orders/${id}/${action}`, 'PATCH')
}

// ---------------------------------------------------------------------------
// Mock (modo demo, sem backend). Determinístico.
// ---------------------------------------------------------------------------

const MOCK: AdminOrder[] = [
  { id: 'a1b2c3d4-0001-4000-8000-000000000001', status: 'paid', channel: 'web', subtotal: 189.9, total: 209.8, customerName: 'Ana Silva', customerDocument: '123.456.789-00', createdAt: '2026-07-29T14:02:00Z', paidAt: '2026-07-29T14:05:00Z' },
  { id: 'a1b2c3d4-0002-4000-8000-000000000002', status: 'picking', channel: 'web', subtotal: 72.0, total: 96.9, customerName: 'Bruno Ferreira', createdAt: '2026-07-29T11:20:00Z', paidAt: '2026-07-29T11:22:00Z' },
  { id: 'a1b2c3d4-0003-4000-8000-000000000003', status: 'shipped', channel: 'web', subtotal: 430.0, total: 469.9, customerName: 'Carla Oliveira', createdAt: '2026-07-28T16:40:00Z', paidAt: '2026-07-28T16:41:00Z' },
  { id: 'a1b2c3d4-0004-4000-8000-000000000004', status: 'delivered', channel: 'balcao', subtotal: 34.5, total: 34.5, customerName: 'Balcão — venda avulsa', createdAt: '2026-07-28T09:10:00Z', paidAt: '2026-07-28T09:10:00Z' },
  { id: 'a1b2c3d4-0005-4000-8000-000000000005', status: 'pending_payment', channel: 'web', subtotal: 133.7, total: 158.6, customerName: 'Daniel Santos', createdAt: '2026-07-30T08:15:00Z' },
]

function mockOrders(q: AdminOrderQuery): AdminOrdersPage {
  let rows = MOCK
  if (q.status === 'active') rows = rows.filter((o) => ['pending_payment', 'paid', 'picking', 'shipped'].includes(o.status))
  else if (q.status === 'done') rows = rows.filter((o) => ['delivered', 'cancelled'].includes(o.status))
  else if (q.status) rows = rows.filter((o) => o.status === q.status)
  if (q.channel) rows = rows.filter((o) => o.channel === q.channel)
  if (q.q) {
    const needle = q.q.toLowerCase()
    rows = rows.filter((o) => (o.customerName ?? '').toLowerCase().includes(needle) || o.id.includes(needle))
  }
  const perPage = q.perPage ?? 20
  return {
    data: rows,
    meta: { page: q.page ?? 1, per_page: perPage, total: rows.length, total_pages: Math.max(1, Math.ceil(rows.length / perPage)) },
  }
}
