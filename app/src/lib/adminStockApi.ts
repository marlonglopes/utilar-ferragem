// Cliente admin de ESTOQUE — a tela do almoxarife. Ver quantidade + alerta de
// baixo, ajustar com MOTIVO (delta relativo), e o histórico de movimento.
// NUNCA traz custo (o almoxarife não vê custo). Mock quando VITE_CATALOG_URL
// vazio (modo demo). Ver docs/estoque.md.
import { adminGet, adminSend } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
export const isStockAdminEnabled = CATALOG_URL !== ''

export interface StockItem {
  id: string
  sku: string | null
  name: string
  stock: number
  lowStockThreshold: number
  lowStock: boolean
  status: string
}

export interface StockPage {
  data: StockItem[]
  meta: { page: number; per_page: number; total: number; total_pages: number }
}

export interface StockMovement {
  id: string
  delta: number
  reason: string
  resultingStock: number
  actorId: string | null
  actorRole: string | null
  createdAt: string
}

export interface StockQuery {
  q?: string
  low?: boolean
  page?: number
  perPage?: number
}

export interface AdjustInput {
  delta: number
  reason: string
}

function query(q: StockQuery): string {
  const sp = new URLSearchParams()
  if (q.q) sp.set('q', q.q)
  if (q.low) sp.set('low', '1')
  sp.set('page', String(q.page ?? 1))
  sp.set('per_page', String(q.perPage ?? 30))
  return `?${sp.toString()}`
}

export async function fetchStock(q: StockQuery): Promise<StockPage> {
  if (!isStockAdminEnabled) return mockStock(q)
  return adminGet<StockPage>(CATALOG_URL, `/api/v1/admin/stock${query(q)}`)
}

export async function adjustStock(id: string, input: AdjustInput): Promise<void> {
  if (!isStockAdminEnabled) return
  await adminSend<unknown>(CATALOG_URL, `/api/v1/admin/stock/${id}/adjust`, 'POST', input)
}

export async function fetchMovements(id: string): Promise<StockMovement[]> {
  if (!isStockAdminEnabled) return mockMovements(id)
  const res = await adminGet<{ data: StockMovement[] }>(
    CATALOG_URL,
    `/api/v1/admin/stock/${id}/movements`
  )
  return res.data
}

// Motivos sugeridos — padroniza a trilha (a busca por "avaria" só acha se todo
// mundo escreve igual). O operador pode digitar outro.
export const STOCK_REASONS = [
  'Contagem de prateleira',
  'Recebimento de fornecedor',
  'Avaria / perda',
  'Devolução ao fornecedor',
  'Correção de cadastro',
] as const

// ---------------------------------------------------------------------------
// Mock (modo demo).
// ---------------------------------------------------------------------------

const MOCK: StockItem[] = [
  {
    id: 'p-1',
    sku: '1024',
    name: 'Parafuso sextavado 1/4" (cento)',
    stock: 3,
    lowStockThreshold: 5,
    lowStock: true,
    status: 'published',
  },
  {
    id: 'p-2',
    sku: '2048',
    name: 'Fechadura tetra reforçada',
    stock: 42,
    lowStockThreshold: 5,
    lowStock: false,
    status: 'published',
  },
  {
    id: 'p-3',
    sku: '3072',
    name: 'Dobradiça 3" (par)',
    stock: 0,
    lowStockThreshold: 8,
    lowStock: true,
    status: 'published',
  },
  {
    id: 'p-4',
    sku: '4096',
    name: 'Cimento CP-II 50kg',
    stock: 120,
    lowStockThreshold: 20,
    lowStock: false,
    status: 'published',
  },
]

function mockStock(q: StockQuery): StockPage {
  let rows = MOCK
  if (q.low) rows = rows.filter((r) => r.lowStock)
  if (q.q) {
    const n = q.q.toLowerCase()
    rows = rows.filter((r) => r.name.toLowerCase().includes(n) || (r.sku ?? '').includes(n))
  }
  rows = [...rows].sort((a, b) => Number(b.lowStock) - Number(a.lowStock))
  const perPage = q.perPage ?? 30
  return {
    data: rows,
    meta: { page: 1, per_page: perPage, total: rows.length, total_pages: 1 },
  }
}

function mockMovements(id: string): StockMovement[] {
  if (id !== 'p-1') return []
  return [
    {
      id: 'm-1',
      delta: -2,
      reason: 'Avaria / perda',
      resultingStock: 3,
      actorId: 'almoxarife@utilar.com.br',
      actorRole: 'almoxarife',
      createdAt: '2026-07-31T14:00:00Z',
    },
    {
      id: 'm-2',
      delta: 5,
      reason: 'Recebimento de fornecedor',
      resultingStock: 5,
      actorId: 'almoxarife@utilar.com.br',
      actorRole: 'almoxarife',
      createdAt: '2026-07-30T09:00:00Z',
    },
  ]
}
