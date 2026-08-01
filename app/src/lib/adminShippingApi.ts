// Cliente admin de FRETE — CRUD da tabela de faixas por CEP. É o que o cliente
// PAGA, então só admin. Consome o order-service. Mock quando VITE_ORDER_URL
// vazio. Ver docs/shipping-api.md.
//
// ⚠️ O seed veio com faixas de CEP de SÃO PAULO; a loja é no RIO GRANDE DO SUL.
// Esta tela é o que permite corrigir.
import { adminGet, adminSend } from '@/lib/adminApi'

const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
export const isShippingAdminEnabled = ORDER_URL !== ''

export interface ShippingRate {
  id: string
  zoneName: string
  cepStart: number
  cepEnd: number
  serviceCode: string
  serviceName: string
  baseCost: number
  costPerItem: number
  deliveryDays: number
  freeAbove: number
  active: boolean
}

export type ShippingRateInput = Omit<ShippingRate, 'id'>

export async function fetchRates(): Promise<ShippingRate[]> {
  if (!isShippingAdminEnabled) return [...MOCK]
  const res = await adminGet<{ data: ShippingRate[] }>(ORDER_URL, '/api/v1/admin/shipping-rates')
  return res.data ?? []
}

export async function createRate(input: ShippingRateInput): Promise<void> {
  if (!isShippingAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, '/api/v1/admin/shipping-rates', 'POST', input)
}

export async function updateRate(id: string, input: ShippingRateInput): Promise<void> {
  if (!isShippingAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/shipping-rates/${id}`, 'PATCH', input)
}

export async function deleteRate(id: string): Promise<void> {
  if (!isShippingAdminEnabled) return
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/shipping-rates/${id}`, 'DELETE')
}

/** 8 dígitos (97000000) → CEP formatado (97000-000). */
export function formatCep(n: number): string {
  const s = String(n).padStart(8, '0')
  return `${s.slice(0, 5)}-${s.slice(5)}`
}

/** "97000-000" ou "97000000" → 97000000 (ou null se inválido). */
export function parseCep(s: string): number | null {
  const digits = s.replace(/\D/g, '')
  if (digits.length !== 8) return null
  return Number(digits)
}

// Uma faixa detectada como sendo de SP (o problema do seed): 01000000–05999999.
export function looksLikeSaoPaulo(r: ShippingRate): boolean {
  return r.cepStart >= 1000000 && r.cepEnd <= 5999999
}

// ---------------------------------------------------------------------------
// Mock (modo demo) — reproduz o seed de SP de propósito, para a tela mostrar o
// aviso e o dono ver como corrigir.
// ---------------------------------------------------------------------------

const MOCK: ShippingRate[] = [
  {
    id: 'sr-1',
    zoneName: 'Capital SP',
    cepStart: 1000000,
    cepEnd: 5999999,
    serviceCode: 'standard',
    serviceName: 'Entrega padrão',
    baseCost: 19.9,
    costPerItem: 1.5,
    deliveryDays: 3,
    freeAbove: 299,
    active: true,
  },
  {
    id: 'sr-2',
    zoneName: 'Capital SP',
    cepStart: 1000000,
    cepEnd: 5999999,
    serviceCode: 'express',
    serviceName: 'Entrega expressa',
    baseCost: 34.9,
    costPerItem: 2,
    deliveryDays: 1,
    freeAbove: 0,
    active: true,
  },
]
