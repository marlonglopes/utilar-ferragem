// Cliente de CASHBACK. Leitura do cliente (/me/cashback) e config do dono
// (/admin/cashback). Mock quando VITE_ORDER_URL vazio.
import { isOrderEnabled, orderGetWithJWT } from '@/lib/api'
import { adminGet, adminSend } from '@/lib/adminApi'

const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''

export interface CashbackEntry {
  kind: 'earn' | 'redeem' | 'reverse' | 'expire'
  amount: number // assinado: earn +, resto −
  orderId?: string
  note?: string
  createdAt: string
}

export interface CashbackInfo {
  active: boolean
  earnRatePct: number
  redeemMaxPct: number
  balance: number
  history: CashbackEntry[]
}

export interface CashbackConfig {
  active: boolean
  earnRatePct: number
  redeemMaxPct: number
  validityDays: number
  minEarnSubtotal: number
  minRedeemSubtotal: number
  campaignRatePct: number
  campaignStartsAt: string | null
  campaignEndsAt: string | null
}

const MOCK: CashbackInfo = {
  active: true,
  earnRatePct: 5,
  redeemMaxPct: 50,
  balance: 37.5,
  history: [
    { kind: 'earn', amount: 12.5, note: 'cashback do pedido', createdAt: '2026-08-01T12:00:00Z' },
    { kind: 'earn', amount: 25, note: 'cashback do pedido', createdAt: '2026-07-20T10:00:00Z' },
    { kind: 'redeem', amount: -10, note: 'resgate no checkout', createdAt: '2026-07-15T09:00:00Z' },
  ],
}

// Saldo + taxa + extrato do cliente logado. Sem backend/sem token → mock (demo).
export async function fetchMyCashback(token: string | null): Promise<CashbackInfo> {
  if (!isOrderEnabled || !token) return MOCK
  return orderGetWithJWT<CashbackInfo>('/api/v1/me/cashback', token)
}

// Admin: config do programa.
export async function fetchCashbackConfig(): Promise<CashbackConfig> {
  if (!isOrderEnabled) {
    return {
      active: true,
      earnRatePct: 5,
      redeemMaxPct: 50,
      validityDays: 90,
      minEarnSubtotal: 0,
      minRedeemSubtotal: 0,
      campaignRatePct: 0,
      campaignStartsAt: null,
      campaignEndsAt: null,
    }
  }
  return adminGet<CashbackConfig>(ORDER_URL, '/api/v1/admin/cashback')
}

export async function updateCashbackConfig(cfg: CashbackConfig): Promise<void> {
  if (!isOrderEnabled) return
  await adminSend<unknown>(ORDER_URL, '/api/v1/admin/cashback', 'PUT', cfg)
}

// Overrides de taxa por categoria: mapa categoryId → %. Categoria ausente usa a
// taxa base.
export async function fetchCategoryRates(): Promise<Record<string, number>> {
  if (!isOrderEnabled) return {}
  const res = await adminGet<{ rates: Record<string, number> }>(
    ORDER_URL,
    '/api/v1/admin/cashback/categories'
  )
  return res.rates ?? {}
}

export async function setCategoryRate(categoryId: string, ratePct: number): Promise<void> {
  if (!isOrderEnabled) return
  await adminSend<unknown>(
    ORDER_URL,
    `/api/v1/admin/cashback/categories/${encodeURIComponent(categoryId)}`,
    'PUT',
    { ratePct }
  )
}

export async function deleteCategoryRate(categoryId: string): Promise<void> {
  if (!isOrderEnabled) return
  await adminSend<unknown>(
    ORDER_URL,
    `/api/v1/admin/cashback/categories/${encodeURIComponent(categoryId)}`,
    'DELETE'
  )
}

export const isCashbackAdminEnabled = ORDER_URL !== ''
