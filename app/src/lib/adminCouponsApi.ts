// Cliente admin de CUPONS — CRUD sobre o order-service (admin-only). Mock quando
// VITE_ORDER_URL vazio. Espelha adminShippingApi.
import { adminGet, adminSend } from '@/lib/adminApi'

const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
export const isCouponAdminEnabled = ORDER_URL !== ''

export type CouponType = 'percent' | 'fixed'

export interface Coupon {
  id: string
  code: string
  type: CouponType
  value: number
  minSubtotal: number
  maxUses: number | null
  uses: number
  active: boolean
  expiresAt: string | null
}

export interface CouponInput {
  code: string
  type: CouponType
  value: number
  minSubtotal?: number
  maxUses?: number | null
  active?: boolean
  expiresAt?: string | null
}

export async function fetchCoupons(): Promise<Coupon[]> {
  if (!isCouponAdminEnabled) return [...MOCK]
  const res = await adminGet<{ coupons: Coupon[] }>(ORDER_URL, '/api/v1/admin/coupons')
  return res.coupons ?? []
}

export async function createCoupon(input: CouponInput): Promise<void> {
  if (!isCouponAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, '/api/v1/admin/coupons', 'POST', input)
}

export async function updateCoupon(id: string, input: Partial<CouponInput>): Promise<void> {
  if (!isCouponAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/coupons/${id}`, 'PATCH', input)
}

export async function deleteCoupon(id: string): Promise<void> {
  if (!isCouponAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/coupons/${id}`, 'DELETE')
}

// Mock (modo demo) — mostra a tela funcionando sem backend.
const MOCK: Coupon[] = [
  {
    id: 'c-1',
    code: 'OBRA10',
    type: 'percent',
    value: 10,
    minSubtotal: 100,
    maxUses: null,
    uses: 3,
    active: true,
    expiresAt: null,
  },
]
