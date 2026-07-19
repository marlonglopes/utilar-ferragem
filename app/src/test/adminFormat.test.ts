import { describe, it, expect } from 'vitest'
import {
  accountingImbalanceCents,
  avgTicketCents,
  conversionRate,
  discountSeverity,
  effectiveFeeRate,
  formatCents,
  formatCentsCompact,
  formatCount,
  formatDelta,
  formatDuration,
  formatLatency,
  formatPercent,
  ledgerBalanceCents,
  marginSeverity,
  outboxSeverity,
  pctChange,
  sellerTotals,
  shortHash,
  sortAlerts,
  sortSellers,
  sumSeries,
} from '@/lib/adminFormat'
import { ledgerToCsv, LEDGER_CSV_HEADER } from '@/lib/adminExport'
import type { Alert, LedgerEntry, SellerPerformance } from '@/lib/adminTypes'

/** Espaço fino não-quebrável que o Intl pt-BR usa entre "R$" e o número. */
function normalize(s: string): string {
  // \u00A0 (NBSP) e \u202F (espaco fino nao-quebravel) vao escapados, nao
  // literais: caractere invisivel dentro de regex e armadilha de revisao.
  return s.replace(/[\u00A0\u202F]/g, ' ')
}

describe('formatCents', () => {
  it('formata centavos em reais', () => {
    expect(normalize(formatCents(1234))).toBe('R$ 12,34')
    expect(normalize(formatCents(0))).toBe('R$ 0,00')
    expect(normalize(formatCents(100_000_00))).toBe('R$ 100.000,00')
  })

  it('mantém o sinal em valores negativos (estorno, divergência)', () => {
    expect(normalize(formatCents(-133_77))).toBe('-R$ 133,77')
  })

  it('não perde o centavo — 1 centavo não vira zero', () => {
    expect(normalize(formatCents(1))).toBe('R$ 0,01')
  })

  it('trata valor não finito como zero em vez de exibir NaN', () => {
    expect(normalize(formatCents(Number.NaN))).toBe('R$ 0,00')
  })
})

describe('formatCentsCompact', () => {
  it('mostra o valor cheio abaixo de mil reais', () => {
    expect(normalize(formatCentsCompact(940_00))).toBe('R$ 940,00')
  })

  it('compacta milhares e milhões', () => {
    expect(normalize(formatCentsCompact(12_345_00))).toBe('R$ 12,3 mil')
    expect(normalize(formatCentsCompact(1_240_000_00))).toBe('R$ 1,24 mi')
  })

  it('preserva o sinal ao compactar', () => {
    expect(normalize(formatCentsCompact(-12_345_00))).toBe('-R$ 12,3 mil')
  })
})

describe('formatPercent / formatDelta', () => {
  it('trata a entrada como FRAÇÃO 0..1, não 0..100', () => {
    expect(formatPercent(0.0342)).toBe('3,4%')
    expect(formatPercent(0.0342, 2)).toBe('3,42%')
    expect(formatPercent(1)).toBe('100,0%')
  })

  it('assina o delta com sinal de menos tipográfico', () => {
    expect(formatDelta(0.124)).toBe('+12,4%')
    expect(formatDelta(-0.03)).toBe('−3,0%')
    expect(formatDelta(0)).toBe('0,0%')
  })

  it('devolve travessão para valor não finito', () => {
    expect(formatPercent(Number.NaN)).toBe('—')
    expect(formatDelta(Number.POSITIVE_INFINITY)).toBe('—')
  })
})

describe('formatCount / formatDuration / formatLatency / shortHash', () => {
  it('agrupa milhar no padrão pt-BR', () => {
    expect(formatCount(1482)).toBe('1.482')
  })

  it('escala a duração pela grandeza', () => {
    expect(formatDuration(45)).toBe('45s')
    expect(formatDuration(720)).toBe('12min')
    expect(formatDuration(3600 * 3 + 60 * 20)).toBe('3h 20min')
    expect(formatDuration(3600 * 52)).toBe('2d 4h')
    expect(formatDuration(-1)).toBe('—')
  })

  it('troca ms por s acima de um segundo', () => {
    expect(formatLatency(412)).toBe('412ms')
    expect(formatLatency(2140)).toBe('2,14s')
  })

  it('encurta o hash preservando as duas pontas', () => {
    const h = 'a'.repeat(8) + 'b'.repeat(48) + 'c'.repeat(8)
    expect(shortHash(h)).toBe('aaaaaaaa…cccccccc')
    expect(shortHash(null)).toBe('—')
    expect(shortHash('curto')).toBe('curto')
  })
})

describe('pctChange', () => {
  it('calcula a variação relativa', () => {
    expect(pctChange(110, 100)).toBeCloseTo(0.1, 10)
    expect(pctChange(90, 100)).toBeCloseTo(-0.1, 10)
  })

  it('devolve null quando a base é zero — não inventa "+100%"', () => {
    expect(pctChange(100, 0)).toBeNull()
    expect(pctChange(0, 0)).toBeNull()
  })

  it('usa o módulo da base para não inverter o sinal em base negativa', () => {
    expect(pctChange(-50, -100)).toBeCloseTo(0.5, 10)
  })
})

describe('conversionRate', () => {
  it('divide confirmados por criados', () => {
    expect(conversionRate({ created: 1482, confirmed: 1291 })).toBeCloseTo(0.87112, 4)
  })

  it('devolve null sem nenhuma tentativa de pagamento', () => {
    expect(conversionRate({ created: 0, confirmed: 0 })).toBeNull()
  })
})

describe('avgTicketCents', () => {
  it('arredonda para centavo inteiro', () => {
    expect(avgTicketCents(1000, 3)).toBe(333)
  })

  it('devolve zero sem pedidos em vez de dividir por zero', () => {
    expect(avgTicketCents(1000, 0)).toBe(0)
  })
})

describe('sumSeries', () => {
  it('soma a série diária', () => {
    expect(
      sumSeries([
        { date: '2026-07-01', valueCents: 100, orders: 1 },
        { date: '2026-07-02', valueCents: 250, orders: 2 },
      ]),
    ).toBe(350)
  })
})

describe('accountingImbalanceCents', () => {
  const base = {
    grossCents: 100_000,
    pspFeeCents: 3_000,
    refundCents: 2_000,
    chargebackCents: 500,
  }

  it('devolve zero quando a identidade contábil fecha', () => {
    expect(accountingImbalanceCents({ ...base, netCents: 94_500 })).toBe(0)
  })

  it('devolve a diferença exata quando não fecha', () => {
    expect(accountingImbalanceCents({ ...base, netCents: 94_600 })).toBe(100)
    expect(accountingImbalanceCents({ ...base, netCents: 94_400 })).toBe(-100)
  })
})

describe('effectiveFeeRate', () => {
  it('divide a taxa pelo bruto', () => {
    expect(effectiveFeeRate(100_000, 4_390)).toBeCloseTo(0.0439, 6)
  })

  it('devolve null com bruto zero', () => {
    expect(effectiveFeeRate(0, 100)).toBeNull()
  })
})

function entry(debit: number, credit: number): LedgerEntry {
  return {
    id: `x-${debit}-${credit}`,
    occurredAt: '2026-07-01T10:00:00Z',
    account: 'Caixa',
    accountCode: '1.1.01',
    debitCents: debit,
    creditCents: credit,
    kind: 'sale',
    method: 'pix',
    orderId: 'ord-1',
    orderNumber: '2026-0001',
    paymentId: 'pay-1',
    pspTransactionId: 'apmx_1',
    memo: 'teste',
  }
}

describe('ledgerBalanceCents', () => {
  it('fecha em zero com partida dobrada correta', () => {
    expect(ledgerBalanceCents([entry(1000, 0), entry(0, 1000)])).toBe(0)
  })

  it('expõe o lançamento órfão', () => {
    expect(ledgerBalanceCents([entry(1000, 0), entry(0, 700)])).toBe(300)
  })
})

describe('severidades', () => {
  it('classifica margem pelos cortes de ferragem', () => {
    expect(marginSeverity(0.05)).toBe('critical')
    expect(marginSeverity(0.15)).toBe('warn')
    expect(marginSeverity(0.3)).toBe('ok')
  })

  it('classifica desconto com o sinal invertido da margem', () => {
    expect(discountSeverity(0.02)).toBe('ok')
    expect(discountSeverity(0.1)).toBe('warn')
    expect(discountSeverity(0.2)).toBe('critical')
  })

  it('faz a idade da fila do outbox pesar mais que o tamanho', () => {
    // Fila grande escoando rápido: saudável.
    expect(outboxSeverity(80, 5)).toBe('ok')
    // Fila minúscula parada há muito tempo: incidente.
    expect(outboxSeverity(3, 1200)).toBe('critical')
    expect(outboxSeverity(150, 30)).toBe('warn')
  })
})

function seller(over: Partial<SellerPerformance>): SellerPerformance {
  return {
    sellerId: 's',
    sellerName: 'Fulano',
    storeId: 'l',
    storeName: 'Loja',
    totalCents: 0,
    orderCount: 0,
    avgTicketCents: 0,
    avgDiscountPct: 0,
    avgMarginPct: 0,
    managerApprovals: 0,
    series: [],
    ...over,
  }
}

describe('sortSellers', () => {
  const list = [
    seller({ sellerId: 'a', sellerName: 'Ana', totalCents: 300, avgMarginPct: 0.05 }),
    seller({ sellerId: 'b', sellerName: 'Bruno', totalCents: 100, avgMarginPct: 0.3 }),
    seller({ sellerId: 'c', sellerName: 'Célia', totalCents: 200, avgMarginPct: 0.2 }),
  ]

  it('ordena por volume desc por padrão', () => {
    expect(sortSellers(list, 'totalCents').map((s) => s.sellerId)).toEqual(['a', 'c', 'b'])
  })

  it('separa "vende muito" de "vende bem" ao ordenar por margem', () => {
    expect(sortSellers(list, 'avgMarginPct').map((s) => s.sellerId)).toEqual(['b', 'c', 'a'])
  })

  it('respeita a direção ascendente', () => {
    expect(sortSellers(list, 'totalCents', 'asc').map((s) => s.sellerId)).toEqual(['b', 'c', 'a'])
  })

  it('não muta o array de entrada', () => {
    const copy = [...list]
    sortSellers(list, 'avgMarginPct')
    expect(list).toEqual(copy)
  })
})

describe('sellerTotals', () => {
  it('soma volumes e pondera médias por valor vendido', () => {
    const t = sellerTotals([
      seller({ totalCents: 900_00, orderCount: 90, avgMarginPct: 0.1, avgDiscountPct: 0.2, managerApprovals: 3 }),
      seller({ totalCents: 100_00, orderCount: 10, avgMarginPct: 0.5, avgDiscountPct: 0.0, managerApprovals: 1 }),
    ])
    expect(t.totalCents).toBe(1_000_00)
    expect(t.orderCount).toBe(100)
    expect(t.managerApprovals).toBe(4)
    expect(t.avgTicketCents).toBe(1_000)
    // Ponderada, não simples: média simples daria 0,30 e 0,10.
    expect(t.avgMarginPct).toBeCloseTo(0.14, 10)
    expect(t.avgDiscountPct).toBeCloseTo(0.18, 10)
  })

  it('não divide por zero com lista vazia', () => {
    const t = sellerTotals([])
    expect(t.totalCents).toBe(0)
    expect(t.avgMarginPct).toBe(0)
    expect(t.avgTicketCents).toBe(0)
  })
})

describe('sortAlerts', () => {
  const mk = (id: string, severity: Alert['severity'], firedAt: string): Alert => ({
    id,
    severity,
    title: id,
    detail: '',
    source: 's',
    firedAt,
  })

  it('coloca crítico antes de atenção, mesmo sendo mais antigo', () => {
    const sorted = sortAlerts([
      mk('novo-aviso', 'warn', '2026-07-18T12:00:00Z'),
      mk('velho-critico', 'critical', '2026-07-17T08:00:00Z'),
    ])
    expect(sorted.map((a) => a.id)).toEqual(['velho-critico', 'novo-aviso'])
  })

  it('desempata pela recência dentro da mesma severidade', () => {
    const sorted = sortAlerts([
      mk('antigo', 'warn', '2026-07-17T08:00:00Z'),
      mk('recente', 'warn', '2026-07-18T08:00:00Z'),
    ])
    expect(sorted.map((a) => a.id)).toEqual(['recente', 'antigo'])
  })
})

describe('ledgerToCsv', () => {
  it('emite o cabeçalho do contrato com separador ponto-e-vírgula', () => {
    const csv = ledgerToCsv([])
    expect(csv.startsWith('﻿')).toBe(true)
    expect(csv).toContain(LEDGER_CSV_HEADER.join(';'))
  })

  it('formata dinheiro com vírgula decimal para o Excel pt-BR', () => {
    const csv = ledgerToCsv([entry(123456, 0)])
    expect(csv).toContain(';1234,56;0,00;')
  })

  it('escapa campo que contém o separador', () => {
    const e = entry(100, 0)
    e.memo = 'Venda; com ponto-e-vírgula'
    expect(ledgerToCsv([e])).toContain('"Venda; com ponto-e-vírgula"')
  })

  it('duplica aspas internas conforme a RFC 4180', () => {
    const e = entry(100, 0)
    e.memo = 'Pedido "urgente"'
    // Aspas internas duplicadas E o campo inteiro envelopado.
    expect(ledgerToCsv([e])).toContain('"Pedido ""urgente"""')
  })

  it('usa CRLF entre as linhas', () => {
    expect(ledgerToCsv([entry(100, 0)])).toContain('\r\n')
  })
})
