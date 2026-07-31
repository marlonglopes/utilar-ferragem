// Cliente admin de STAFF — operadores de loja (o backend já existia; faltava a
// tela). Junta 3 leituras do auth-service (usuários, lojas, operadores) + criar
// e editar operador. É a base das personas: achar a pessoa e dar o papel.
import { adminGet, adminSend } from '@/lib/adminApi'

const AUTH_URL = import.meta.env.VITE_AUTH_URL ?? ''
export const isStaffAdminEnabled = AUTH_URL !== ''

export type StoreLevel = 'operator' | 'supervisor' | 'manager'

export interface AdminUser {
  id: string
  name: string
  email: string
  role: string
  cpf: string | null
  phone: string | null
  emailVerified: boolean
  createdAt: string
}

export interface Store {
  id: string
  code: string
  name: string
  active: boolean
}

export interface Operator {
  userId: string
  name: string
  email: string
  storeId: string
  storeCode: string
  storeName: string
  level: StoreLevel
  discountCeilingPct: number
  canApproveDiscount: boolean
  active: boolean
  createdAt: string
}

export interface CreateOperatorInput {
  userId: string
  storeId: string
  level: StoreLevel
  discountCeilingPct?: number
}

export interface UpdateOperatorInput {
  storeId?: string
  level?: StoreLevel
  discountCeilingPct?: number
  active?: boolean
}

interface Page<T> {
  data: T[]
  meta?: { total: number }
}

export async function fetchOperators(): Promise<Operator[]> {
  if (!isStaffAdminEnabled) return MOCK_OPERATORS
  const p = await adminGet<Page<Operator>>(AUTH_URL, '/api/v1/admin/operators')
  return p.data
}

export async function fetchStores(): Promise<Store[]> {
  if (!isStaffAdminEnabled) return MOCK_STORES
  const p = await adminGet<Page<Store>>(AUTH_URL, '/api/v1/admin/stores')
  return p.data
}

export async function fetchUsers(q: string): Promise<AdminUser[]> {
  if (!isStaffAdminEnabled) return MOCK_USERS.filter((u) => matchUser(u, q))
  const sp = new URLSearchParams()
  if (q) sp.set('q', q)
  sp.set('per_page', '20')
  const p = await adminGet<Page<AdminUser>>(AUTH_URL, `/api/v1/admin/users?${sp.toString()}`)
  return p.data
}

export async function createOperator(input: CreateOperatorInput): Promise<void> {
  if (!isStaffAdminEnabled) return
  await adminSend<unknown>(AUTH_URL, '/api/v1/admin/operators', 'POST', input)
}

export async function updateOperator(userId: string, input: UpdateOperatorInput): Promise<void> {
  if (!isStaffAdminEnabled) return
  await adminSend<unknown>(AUTH_URL, `/api/v1/admin/operators/${userId}`, 'PATCH', input)
}

export const LEVEL_LABEL: Record<StoreLevel, string> = {
  operator: 'Operador',
  supervisor: 'Supervisor',
  manager: 'Gerente',
}

function matchUser(u: AdminUser, q: string): boolean {
  if (!q) return true
  const n = q.toLowerCase()
  return u.name.toLowerCase().includes(n) || u.email.toLowerCase().includes(n)
}

// ---------------------------------------------------------------------------
// Mock (modo demo)
// ---------------------------------------------------------------------------

const MOCK_STORES: Store[] = [
  { id: 's-1', code: 'MATRIZ', name: 'Utilar Ferragem - Matriz', active: true },
  { id: 's-2', code: 'FIL-002', name: 'Utilar Ferragem - Santo André', active: true },
]

const MOCK_OPERATORS: Operator[] = [
  { userId: 'u-14', name: 'Nicolas Dias', email: 'test14@utilar.com.br', storeId: 's-1', storeCode: 'MATRIZ', storeName: 'Matriz', level: 'operator', discountCeilingPct: 5, canApproveDiscount: false, active: true, createdAt: '2026-07-20T10:00:00Z' },
  { userId: 'u-15', name: 'Olívia Fernandes', email: 'test15@utilar.com.br', storeId: 's-1', storeCode: 'MATRIZ', storeName: 'Matriz', level: 'manager', discountCeilingPct: 20, canApproveDiscount: true, active: true, createdAt: '2026-07-20T10:00:00Z' },
]

const MOCK_USERS: AdminUser[] = [
  { id: 'u-1', name: 'Ana Silva', email: 'test1@utilar.com.br', role: 'customer', cpf: null, phone: null, emailVerified: true, createdAt: '2026-07-01T10:00:00Z' },
  { id: 'u-2', name: 'Bruno Ferreira', email: 'test2@utilar.com.br', role: 'customer', cpf: null, phone: null, emailVerified: true, createdAt: '2026-07-02T10:00:00Z' },
  { id: 'u-c', name: 'Contador Utilar', email: 'contador@utilar.com.br', role: 'customer', cpf: null, phone: null, emailVerified: true, createdAt: '2026-07-03T10:00:00Z' },
]
