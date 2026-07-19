import { describe, it, expect } from 'vitest'
import {
  computeBalcaoPricing,
  maxDiscountPctBeforeCost,
  discountCeilingFor,
  isCNPJ,
  round2,
  HEALTHY_MARGIN_PCT,
  type PricedLine,
} from '@/store/balcaoStore'

/** Linha com margem bruta de 50% (custo = metade do preço). */
function line(overrides: Partial<PricedLine> = {}): PricedLine {
  return { unitPrice: 100, unitCost: 50, quantity: 1, ...overrides }
}

describe('computeBalcaoPricing — totais', () => {
  it('soma subtotal, custo e contagem de itens', () => {
    const p = computeBalcaoPricing({
      items: [line({ quantity: 2 }), line({ unitPrice: 50, unitCost: 30, quantity: 3 })],
      discountPct: 0,
    })
    expect(p.subtotal).toBe(350) // 100*2 + 50*3
    expect(p.cost).toBe(190) // 50*2 + 30*3
    expect(p.itemCount).toBe(5)
    expect(p.lineCount).toBe(2)
    expect(p.total).toBe(350)
  })

  it('aplica desconto percentual sobre o subtotal', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 10 })
    expect(p.discountAmount).toBe(10)
    expect(p.total).toBe(90)
    expect(p.grossProfit).toBe(40)
  })

  it('trata pedido vazio sem dividir por zero', () => {
    const p = computeBalcaoPricing({ items: [], discountPct: 20 })
    expect(p.subtotal).toBe(0)
    expect(p.total).toBe(0)
    expect(p.marginPct).toBe(0)
    expect(p.status).toBe('healthy')
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
  })

  it('limita desconto ao intervalo 0–100', () => {
    expect(computeBalcaoPricing({ items: [line()], discountPct: -5 }).discountPct).toBe(0)
    expect(computeBalcaoPricing({ items: [line()], discountPct: 250 }).discountPct).toBe(100)
  })

  it('ignora quantidade negativa', () => {
    const p = computeBalcaoPricing({ items: [line({ quantity: -3 })], discountPct: 0 })
    expect(p.subtotal).toBe(0)
    expect(p.itemCount).toBe(0)
  })

  it('arredonda para centavos sem ruído de ponto flutuante', () => {
    const p = computeBalcaoPricing({
      items: [{ unitPrice: 10.1, unitCost: 5.05, quantity: 3 }],
      discountPct: 7,
    })
    expect(p.subtotal).toBe(30.3)
    expect(p.discountAmount).toBe(2.12)
    expect(p.total).toBe(28.18)
  })
})

describe('computeBalcaoPricing — margem', () => {
  it('margem saudável sem desconto', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 0 })
    expect(p.marginPct).toBe(50)
    expect(p.baseMarginPct).toBe(50)
    expect(p.status).toBe('healthy')
  })

  it('vira âmbar quando a margem cai abaixo do saudável mas segue positiva', () => {
    // custo 90 de 100: margem base 10% — já abaixo do limite saudável.
    const p = computeBalcaoPricing({
      items: [line({ unitCost: 90 })],
      discountPct: 0,
    })
    expect(p.marginPct).toBeLessThan(HEALTHY_MARGIN_PCT)
    expect(p.marginPct).toBeGreaterThan(0)
    expect(p.status).toBe('warning')
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
  })

  it('MARGEM NEGATIVA: desconto abaixo do custo bloqueia a venda', () => {
    // 60% de desconto sobre item com custo 50% → total 40 < custo 50.
    const p = computeBalcaoPricing({ items: [line()], discountPct: 60, role: 'manager' })
    expect(p.total).toBe(40)
    expect(p.grossProfit).toBe(-10)
    expect(p.marginPct).toBeLessThan(0)
    expect(p.status).toBe('negative')
    expect(p.belowCost).toBe(true)
    expect(p.blocked).toBe(true)
  })

  it('exatamente no custo não é prejuízo, mas é âmbar', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 50, role: 'manager' })
    expect(p.total).toBe(50)
    expect(p.grossProfit).toBe(0)
    expect(p.marginPct).toBe(0)
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
    expect(p.status).toBe('warning')
  })

  it('margem é calculada sobre a venda, não sobre o custo', () => {
    // total 80, custo 50 → lucro 30 → 30/80 = 37,5%
    const p = computeBalcaoPricing({ items: [line()], discountPct: 20, role: 'manager' })
    expect(p.total).toBe(80)
    expect(p.marginPct).toBe(37.5)
    // margem antes do desconto continua a de referência
    expect(p.baseMarginPct).toBe(50)
  })

  it('custo zero rende 100% de margem', () => {
    const p = computeBalcaoPricing({ items: [line({ unitCost: 0 })], discountPct: 0 })
    expect(p.marginPct).toBe(100)
    expect(p.status).toBe('healthy')
  })
})

describe('computeBalcaoPricing — teto por cargo', () => {
  it('operador tem teto de 12% e não estoura dentro dele', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 12, role: 'operator' })
    expect(p.ceilingPct).toBe(12)
    expect(p.overCeiling).toBe(false)
    expect(p.requiresApproval).toBe(false)
  })

  it('acima do teto marca aprovação do gerente SEM bloquear', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 18, role: 'operator' })
    expect(p.overCeiling).toBe(true)
    expect(p.requiresApproval).toBe(true)
    // margem segue positiva → não bloqueia
    expect(p.blocked).toBe(false)
    expect(p.status).toBe('healthy')
  })

  it('supervisor tem teto maior que operador', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 18, role: 'supervisor' })
    expect(p.ceilingPct).toBe(20)
    expect(p.requiresApproval).toBe(false)
  })

  it('gerente não é barrado por teto, mas ainda é barrado por prejuízo', () => {
    const ok = computeBalcaoPricing({ items: [line()], discountPct: 45, role: 'manager' })
    expect(ok.requiresApproval).toBe(false)
    expect(ok.blocked).toBe(false)

    const loss = computeBalcaoPricing({ items: [line()], discountPct: 70, role: 'manager' })
    expect(loss.requiresApproval).toBe(false)
    expect(loss.blocked).toBe(true)
  })

  it('papel padrão é operador', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 15 })
    expect(p.ceilingPct).toBe(12)
    expect(p.requiresApproval).toBe(true)
  })

  it('pedido vazio nunca exige aprovação', () => {
    const p = computeBalcaoPricing({ items: [], discountPct: 90 })
    expect(p.overCeiling).toBe(false)
    expect(p.requiresApproval).toBe(false)
  })
})

describe('maxDiscountPctBeforeCost', () => {
  it('devolve a margem base como desconto máximo', () => {
    expect(maxDiscountPctBeforeCost([line()])).toBe(50)
  })

  it('é 0 quando o custo já é igual ou maior que o preço', () => {
    expect(maxDiscountPctBeforeCost([line({ unitCost: 100 })])).toBe(0)
    expect(maxDiscountPctBeforeCost([line({ unitCost: 120 })])).toBe(0)
  })

  it('é 0 para pedido vazio', () => {
    expect(maxDiscountPctBeforeCost([])).toBe(0)
  })

  it('casa com o ponto exato em que a venda passa a ser prejuízo', () => {
    const items = [line({ unitCost: 70, quantity: 2 })]
    const max = maxDiscountPctBeforeCost(items)
    expect(computeBalcaoPricing({ items, discountPct: max, role: 'manager' }).blocked).toBe(false)
    expect(
      computeBalcaoPricing({ items, discountPct: max + 1, role: 'manager' }).blocked
    ).toBe(true)
  })
})

describe('helpers', () => {
  it('discountCeilingFor devolve o teto de cada cargo', () => {
    expect(discountCeilingFor('operator')).toBe(12)
    expect(discountCeilingFor('supervisor')).toBe(20)
    expect(discountCeilingFor('manager')).toBe(100)
  })

  it('isCNPJ distingue CPF de CNPJ', () => {
    expect(isCNPJ('12345678901')).toBe(false)
    expect(isCNPJ('12.345.678/0001-90')).toBe(true)
  })

  it('round2 arredonda para duas casas', () => {
    expect(round2(1.005)).toBe(1.01)
    expect(round2(0.1 + 0.2)).toBe(0.3)
  })
})
