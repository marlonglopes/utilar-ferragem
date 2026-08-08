// Validação de cupom no CHECKOUT (preview) — separado do CRUD admin.
//
// O valor devolvido aqui é só para MOSTRAR o desconto ao cliente. O valor
// cobrado de verdade é recalculado no servidor (order-service), sobre o subtotal
// autoritativo, no POST /orders — o corpo do cliente nunca é fonte de verdade.
import { isOrderEnabled, orderPostWithJWT } from '@/lib/api'

export interface CouponPreview {
  code: string
  type: 'percent' | 'fixed'
  discount: number
}

function round2(v: number): number {
  return Math.round(v * 100) / 100
}

// Modo demo (VITE_ORDER_URL vazio): valida contra a lista de mock, para a tela
// funcionar sem backend. NÃO é a regra de verdade — só o suficiente pra demo.
async function validateMock(code: string, subtotal: number): Promise<CouponPreview> {
  const { fetchCoupons } = await import('@/lib/adminCouponsApi')
  const list = await fetchCoupons()
  const c = list.find((x) => x.code === code)
  if (!c || !c.active) throw new Error('cupom inválido')
  if (subtotal < c.minSubtotal) {
    throw new Error(`este cupom vale a partir de R$ ${c.minSubtotal.toFixed(2)}`)
  }
  const raw = c.type === 'percent' ? (subtotal * c.value) / 100 : c.value
  return { code: c.code, type: c.type, discount: round2(Math.min(raw, subtotal)) }
}

/**
 * Confere o cupom e devolve o desconto (sem gastar uso). Lança Error com
 * mensagem acionável se o cupom não valer para este carrinho.
 */
export async function validateCoupon(
  rawCode: string,
  subtotal: number,
  token: string | null
): Promise<CouponPreview> {
  const code = rawCode.trim().toUpperCase()
  if (!code) throw new Error('digite um código')
  if (!isOrderEnabled || !token) return validateMock(code, subtotal)
  return orderPostWithJWT<CouponPreview>('/api/v1/coupons/validate', token, {
    code,
    subtotal,
  })
}
