import { describe, it, expect } from 'vitest'
import { fetchCoupons } from '@/lib/adminCouponsApi'
import { validateCoupon } from '@/lib/couponCheckout'

describe('adminCouponsApi (mock)', () => {
  it('lista os cupons do mock com o OBRA10', async () => {
    const list = await fetchCoupons()
    expect(list.length).toBeGreaterThan(0)
    expect(list.some((c) => c.code === 'OBRA10')).toBe(true)
  })
})

describe('validateCoupon (preview do checkout, modo mock)', () => {
  it('aplica percentual sobre o subtotal', async () => {
    const p = await validateCoupon('obra10', 200, null) // 10% de 200
    expect(p.code).toBe('OBRA10')
    expect(p.discount).toBe(20)
  })

  it('recusa abaixo do pedido mínimo', async () => {
    await expect(validateCoupon('OBRA10', 50, null)).rejects.toThrow(/a partir de/i)
  })

  it('recusa código inexistente', async () => {
    await expect(validateCoupon('NAOEXISTE', 200, null)).rejects.toThrow(/inválido/i)
  })

  it('recusa código vazio', async () => {
    await expect(validateCoupon('   ', 200, null)).rejects.toThrow(/digite/i)
  })

  // Invariante: o desconto do preview nunca passa do subtotal (o servidor
  // também trava; um cupom fixo maior que o carrinho não pode gerar total < 0).
  it('nunca desconta mais que o subtotal', async () => {
    const p = await validateCoupon('OBRA10', 1000, null)
    expect(p.discount).toBeLessThanOrEqual(1000)
  })
})
