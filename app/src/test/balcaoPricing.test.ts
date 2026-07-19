import { describe, it, expect } from 'vitest'
import {
  computeBalcaoPricing,
  maxDiscountPctBeforeCost,
  isCNPJ,
  round2,
  HEALTHY_MARGIN_PCT,
  type PricedLine,
} from '@/store/balcaoStore'

/**
 * Linha com margem bruta de 50% (custo = metade do preço).
 *
 * `ceilingPct` é sempre explícito nestes testes porque o teto deixou de ser
 * derivado do cargo no front — ele vem de `GET /api/v1/store/me`.
 */
function line(overrides: Partial<PricedLine> = {}): PricedLine {
  return { unitPrice: 100, unitCost: 50, quantity: 1, ...overrides }
}

describe('computeBalcaoPricing — totais', () => {
  it('soma subtotal, custo e contagem de itens', () => {
    const p = computeBalcaoPricing({
      items: [line({ quantity: 2 }), line({ unitPrice: 50, unitCost: 30, quantity: 3 })],
      discountPct: 0, ceilingPct: 12 })
    expect(p.subtotal).toBe(350) // 100*2 + 50*3
    expect(p.cost).toBe(190) // 50*2 + 30*3
    expect(p.itemCount).toBe(5)
    expect(p.lineCount).toBe(2)
    expect(p.total).toBe(350)
  })

  it('aplica desconto percentual sobre o subtotal', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 10, ceilingPct: 12 })
    expect(p.discountAmount).toBe(10)
    expect(p.total).toBe(90)
    expect(p.grossProfit).toBe(40)
  })

  it('trata pedido vazio sem dividir por zero', () => {
    const p = computeBalcaoPricing({ items: [], discountPct: 20, ceilingPct: 12 })
    expect(p.subtotal).toBe(0)
    expect(p.total).toBe(0)
    expect(p.marginPct).toBe(0)
    expect(p.status).toBe('healthy')
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
  })

  it('limita desconto ao intervalo 0–100', () => {
    expect(computeBalcaoPricing({ items: [line()], discountPct: -5, ceilingPct: 12 }).discountPct).toBe(0)
    expect(computeBalcaoPricing({ items: [line()], discountPct: 250, ceilingPct: 12 }).discountPct).toBe(100)
  })

  it('ignora quantidade negativa', () => {
    const p = computeBalcaoPricing({ items: [line({ quantity: -3 })], discountPct: 0, ceilingPct: 12 })
    expect(p.subtotal).toBe(0)
    expect(p.itemCount).toBe(0)
  })

  it('arredonda para centavos sem ruído de ponto flutuante', () => {
    const p = computeBalcaoPricing({
      items: [{ unitPrice: 10.1, unitCost: 5.05, quantity: 3 }],
      discountPct: 7, ceilingPct: 12 })
    expect(p.subtotal).toBe(30.3)
    expect(p.discountAmount).toBe(2.12)
    expect(p.total).toBe(28.18)
  })
})

describe('computeBalcaoPricing — margem', () => {
  it('margem saudável sem desconto', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 0, ceilingPct: 12 })
    expect(p.marginPct).toBe(50)
    expect(p.baseMarginPct).toBe(50)
    expect(p.status).toBe('healthy')
  })

  it('vira âmbar quando a margem cai abaixo do saudável mas segue positiva', () => {
    // custo 90 de 100: margem base 10% — já abaixo do limite saudável.
    const p = computeBalcaoPricing({
      items: [line({ unitCost: 90 })],
      discountPct: 0, ceilingPct: 12 })
    expect(p.marginPct).toBeLessThan(HEALTHY_MARGIN_PCT)
    expect(p.marginPct).toBeGreaterThan(0)
    expect(p.status).toBe('warning')
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
  })

  it('MARGEM NEGATIVA: desconto abaixo do custo bloqueia a venda', () => {
    // 60% de desconto sobre item com custo 50% → total 40 < custo 50.
    const p = computeBalcaoPricing({ items: [line()], discountPct: 60, ceilingPct: 100 })
    expect(p.total).toBe(40)
    expect(p.grossProfit).toBe(-10)
    expect(p.marginPct).toBeLessThan(0)
    expect(p.status).toBe('negative')
    expect(p.belowCost).toBe(true)
    expect(p.blocked).toBe(true)
  })

  it('exatamente no custo não é prejuízo, mas é âmbar', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 50, ceilingPct: 100 })
    expect(p.total).toBe(50)
    expect(p.grossProfit).toBe(0)
    expect(p.marginPct).toBe(0)
    expect(p.belowCost).toBe(false)
    expect(p.blocked).toBe(false)
    expect(p.status).toBe('warning')
  })

  it('margem é calculada sobre a venda, não sobre o custo', () => {
    // total 80, custo 50 → lucro 30 → 30/80 = 37,5%
    const p = computeBalcaoPricing({ items: [line()], discountPct: 20, ceilingPct: 100 })
    expect(p.total).toBe(80)
    expect(p.marginPct).toBe(37.5)
    // margem antes do desconto continua a de referência
    expect(p.baseMarginPct).toBe(50)
  })

  it('custo zero rende 100% de margem', () => {
    const p = computeBalcaoPricing({ items: [line({ unitCost: 0 })], discountPct: 0, ceilingPct: 12 })
    expect(p.marginPct).toBe(100)
    expect(p.status).toBe('healthy')
  })
})

describe('computeBalcaoPricing — teto vindo do backend', () => {
  it('operador tem teto de 12% e não estoura dentro dele', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 12, ceilingPct: 12 })
    expect(p.ceilingPct).toBe(12)
    expect(p.overCeiling).toBe(false)
    expect(p.requiresApproval).toBe(false)
  })

  it('acima do teto marca aprovação do gerente SEM bloquear', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 18, ceilingPct: 12 })
    expect(p.overCeiling).toBe(true)
    expect(p.requiresApproval).toBe(true)
    // margem segue positiva → não bloqueia
    expect(p.blocked).toBe(false)
    expect(p.status).toBe('healthy')
  })

  it('supervisor tem teto maior que operador', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 18, ceilingPct: 20 })
    expect(p.ceilingPct).toBe(20)
    expect(p.requiresApproval).toBe(false)
  })

  it('gerente não é barrado por teto, mas ainda é barrado por prejuízo', () => {
    const ok = computeBalcaoPricing({ items: [line()], discountPct: 45, ceilingPct: 100 })
    expect(ok.requiresApproval).toBe(false)
    expect(ok.blocked).toBe(false)

    const loss = computeBalcaoPricing({ items: [line()], discountPct: 70, ceilingPct: 100 })
    expect(loss.requiresApproval).toBe(false)
    expect(loss.blocked).toBe(true)
  })

  it('teto 0 (fail-closed) manda todo desconto para aprovação', () => {
    // É o estado enquanto GET /api/v1/store/me não respondeu, ou quando falhou.
    // O erro seguro é pedir aprovação a mais, nunca conceder desconto a mais.
    const p = computeBalcaoPricing({ items: [line()], discountPct: 1, ceilingPct: 0 })
    expect(p.ceilingPct).toBe(0)
    expect(p.overCeiling).toBe(true)
    expect(p.requiresApproval).toBe(true)
    // ...mas não bloqueia: a venda continua possível, só nasce pendente.
    expect(p.blocked).toBe(false)
  })

  it('teto negativo é tratado como 0 e não vira desconto livre', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 5, ceilingPct: -10 })
    expect(p.ceilingPct).toBe(0)
    expect(p.requiresApproval).toBe(true)
  })

  it('um teto individual fora da tabela de cargos é respeitado', () => {
    // O backend resolve COALESCE(override do indivíduo, teto do cargo) e manda
    // o número pronto. O front não pode "corrigir" para o valor do cargo.
    const p = computeBalcaoPricing({ items: [line()], discountPct: 15, ceilingPct: 17.5 })
    expect(p.ceilingPct).toBe(17.5)
    expect(p.requiresApproval).toBe(false)
  })

  it('pedido vazio nunca exige aprovação', () => {
    const p = computeBalcaoPricing({ items: [], discountPct: 90, ceilingPct: 12 })
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
    expect(computeBalcaoPricing({ items, discountPct: max, ceilingPct: 100 }).blocked).toBe(false)
    expect(
      computeBalcaoPricing({ items, discountPct: max + 1, ceilingPct: 100 }).blocked
    ).toBe(true)
  })
})

describe('computeBalcaoPricing — custo real vs. estimado', () => {
  it('não marca estimativa quando todas as linhas têm custo real', () => {
    const p = computeBalcaoPricing({
      items: [line({ costIsEstimated: false }), line({ costIsEstimated: false })],
      discountPct: 0,
      ceilingPct: 12,
    })
    expect(p.costEstimated).toBe(false)
    expect(p.marginPct).toBe(50)
  })

  it('UMA linha estimada contamina a margem do pedido inteiro', () => {
    // O total é uma soma: se um dos custos é chute, a margem resultante é
    // aproximada — não dá para rotular "quase certa".
    const p = computeBalcaoPricing({
      items: [line({ costIsEstimated: false }), line({ costIsEstimated: true })],
      discountPct: 0,
      ceilingPct: 12,
    })
    expect(p.costEstimated).toBe(true)
  })

  it('custo real e custo estimado produzem margens diferentes — daí o rótulo', () => {
    // Preço 100. Custo real 40 → margem 60%. Estimado (72% do preço) → 28%.
    const real = computeBalcaoPricing({
      items: [line({ unitCost: 40, costIsEstimated: false })],
      discountPct: 0,
      ceilingPct: 12,
    })
    const estimated = computeBalcaoPricing({
      items: [line({ unitCost: 72, costIsEstimated: true })],
      discountPct: 0,
      ceilingPct: 12,
    })

    expect(real.marginPct).toBe(60)
    expect(real.costEstimated).toBe(false)
    expect(estimated.marginPct).toBe(28)
    expect(estimated.costEstimated).toBe(true)

    // 32 pontos de margem de diferença: é exatamente por isso que a UI precisa
    // dizer qual dos dois números o vendedor está olhando.
    expect(real.marginPct - estimated.marginPct).toBe(32)
  })

  it('linha sem a flag é tratada como custo real (não inventa alarme)', () => {
    const p = computeBalcaoPricing({ items: [line()], discountPct: 0, ceilingPct: 12 })
    expect(p.costEstimated).toBe(false)
  })
})

describe('helpers', () => {
  it('isCNPJ distingue CPF de CNPJ', () => {
    expect(isCNPJ('12345678901')).toBe(false)
    expect(isCNPJ('12.345.678/0001-90')).toBe(true)
  })

  it('round2 arredonda para duas casas', () => {
    expect(round2(1.005)).toBe(1.01)
    expect(round2(0.1 + 0.2)).toBe(0.3)
  })
})
