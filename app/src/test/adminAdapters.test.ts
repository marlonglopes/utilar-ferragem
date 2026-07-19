import { describe, it, expect } from 'vitest'
import {
  adaptAuditAction,
  adaptAuditRecord,
  adaptChainVerification,
  adaptDailyPoint,
  adaptDiscrepancy,
  adaptDiscrepancySeverity,
  adaptEntry,
  adaptMethodBreakdown,
  adaptReconciliation,
  adaptSummary,
  auditValueToText,
  maskIp,
  toLedgerKind,
  toPaymentMethod,
  type ApiAuditRecord,
  type ApiDiscrepancy,
  type ApiEntryRow,
  type ApiLedgerSummary,
} from '@/lib/adminAdapters'
import { accountingImbalanceCents, ledgerBalanceCents } from '@/lib/adminFormat'

/**
 * Estes testes protegem a fronteira entre o dialeto contábil do
 * payment-service (`/api/v1/ledger/*`) e o modelo de view do painel. É onde um
 * rename de campo no backend passaria despercebido e viraria "R$ 0,00" na tela
 * — o modo de falha mais perigoso de um painel financeiro, porque parece dado.
 */

const period = { from: '2026-07-01', to: '2026-07-31' }

const apiSummary: ApiLedgerSummary = {
  from: '2026-07-01T00:00:00Z',
  to: '2026-07-31T23:59:59Z',
  currency: 'BRL',
  grossCents: 1_000_000,
  pspFeesCents: 43_900,
  anticipationFeesCents: 12_000,
  refundsCents: 21_000,
  chargebacksCents: 6_800,
  sellerSplitCents: 150_000,
  netCents: 778_300, // gross - fees - refunds - chargebacks - split
  transactionCount: 412,
}

describe('adaptSummary', () => {
  it('traduz os nomes plurais do backend para o modelo de view', () => {
    const s = adaptSummary(period, apiSummary, [], [])
    expect(s.grossCents).toBe(1_000_000)
    expect(s.pspFeeCents).toBe(43_900) // pspFeesCents → pspFeeCents
    expect(s.refundCents).toBe(21_000) // refundsCents → refundCents
    expect(s.chargebackCents).toBe(6_800) // chargebacksCents → chargebackCents
    expect(s.anticipationFeeCents).toBe(12_000)
    expect(s.sellerSplitCents).toBe(150_000)
    expect(s.transactionCount).toBe(412)
  })

  it('produz um resumo que fecha na identidade contábil do backend', () => {
    const s = adaptSummary(period, apiSummary, [], [])
    expect(accountingImbalanceCents(s)).toBe(0)
  })

  it('também aceita a leitura em que a antecipação entra no líquido', () => {
    // A fórmula documentada não lista antecipação; a checagem tolera as duas
    // para não disparar um alarme falso. Ver `accountingImbalanceCents`.
    const s = adaptSummary(period, { ...apiSummary, netCents: 778_300 - 12_000 }, [], [])
    expect(accountingImbalanceCents(s)).toBe(0)
  })

  it('acusa desequilíbrio real, que nenhuma das duas leituras explica', () => {
    const s = adaptSummary(period, { ...apiSummary, netCents: 900_000 }, [], [])
    expect(accountingImbalanceCents(s)).not.toBe(0)
  })
})

describe('adaptMethodBreakdown', () => {
  const m = {
    method: 'card',
    grossCents: 500_000,
    pspFeesCents: 21_950,
    refundsCents: 10_000,
    netCents: 468_050,
    saleCount: 180,
  }

  it('devolve chargeback null — a rota não recorta por método', () => {
    // Crucial: 0 afirmaria "não houve chargeback neste método".
    expect(adaptMethodBreakdown(m).chargebackCents).toBeNull()
  })

  it('deriva a taxa efetiva sobre o bruto', () => {
    expect(adaptMethodBreakdown(m).effectiveFeeRate).toBeCloseTo(0.0439, 6)
  })

  it('não divide por zero quando o método não teve movimento', () => {
    expect(adaptMethodBreakdown({ ...m, grossCents: 0 }).effectiveFeeRate).toBe(0)
  })
})

describe('toPaymentMethod', () => {
  it('preserva `card` em vez de inventar crédito', () => {
    // O livro não desdobra cartão; afirmar "crédito" seria inventar dado.
    expect(toPaymentMethod('card')).toBe('card')
  })

  it('mapeia os métodos conhecidos', () => {
    expect(toPaymentMethod('pix')).toBe('pix')
    expect(toPaymentMethod('boleto')).toBe('boleto')
  })

  it('trata método ausente ou desconhecido como null', () => {
    expect(toPaymentMethod('')).toBeNull()
    expect(toPaymentMethod(undefined)).toBeNull()
    expect(toPaymentMethod('bitcoin')).toBeNull()
  })
})

describe('toLedgerKind', () => {
  it('normaliza `fee` para `psp_fee`', () => {
    expect(toLedgerKind('fee')).toBe('psp_fee')
  })

  it('preserva as naturezas conhecidas', () => {
    expect(toLedgerKind('sale')).toBe('sale')
    expect(toLedgerKind('chargeback')).toBe('chargeback')
  })

  it('cai em `adjustment` para natureza desconhecida, em vez de quebrar', () => {
    expect(toLedgerKind('coisa_nova')).toBe('adjustment')
  })
})

describe('adaptEntry', () => {
  const row: ApiEntryRow = {
    transactionId: 'tx-1',
    occurredAt: '2026-07-10T14:00:00Z',
    kind: 'sale',
    sourceType: 'order',
    sourceId: '2026-8841',
    description: 'Venda balcão',
    account: '3.1.1',
    accountName: 'Receita bruta',
    side: 'credit',
    amountCents: 48_900,
    paymentMethod: 'pix',
  }

  it('desdobra side + amount nas colunas de débito e crédito', () => {
    const credit = adaptEntry(row, 0)
    expect(credit.creditCents).toBe(48_900)
    expect(credit.debitCents).toBe(0)

    const debit = adaptEntry({ ...row, side: 'debit' }, 1)
    expect(debit.debitCents).toBe(48_900)
    expect(debit.creditCents).toBe(0)
  })

  it('mantém a partida dobrada ao converter as duas pernas', () => {
    const pair = [
      adaptEntry(row, 0),
      adaptEntry({ ...row, side: 'debit', account: '1.1.1', accountName: 'Caixa' }, 1),
    ]
    expect(ledgerBalanceCents(pair)).toBe(0)
  })

  it('liga a linha ao pedido quando a origem é um pedido', () => {
    const e = adaptEntry(row, 0)
    expect(e.orderId).toBe('2026-8841')
    expect(e.paymentId).toBeNull()
  })

  it('liga ao pagamento quando a origem é um pagamento', () => {
    const e = adaptEntry({ ...row, sourceType: 'payment', sourceId: 'pay-9' }, 0)
    expect(e.paymentId).toBe('pay-9')
    expect(e.orderId).toBeNull()
  })

  it('gera chave estável a partir da transação e do índice', () => {
    expect(adaptEntry(row, 3).id).toBe('tx-1:3')
  })

  it('usa o memo e cai na descrição quando não há memo', () => {
    expect(adaptEntry({ ...row, memo: 'obs' }, 0).memo).toBe('obs')
    expect(adaptEntry(row, 0).memo).toBe('Venda balcão')
  })
})

describe('adaptDailyPoint', () => {
  it('usa o líquido como valor da série e zera pedidos', () => {
    const p = adaptDailyPoint({
      day: '2026-07-10',
      grossCents: 100,
      pspFeesCents: 5,
      refundsCents: 0,
      netCents: 95,
    })
    // O livro conta lançamentos, não pedidos — zero aqui é honesto.
    expect(p).toEqual({ date: '2026-07-10', valueCents: 95, orders: 0 })
  })
})

describe('reconciliação', () => {
  const d: ApiDiscrepancy = {
    id: 'd-1',
    runId: 'r-1',
    paymentId: 'pay-1',
    pspPaymentId: 'apmx_1',
    kind: 'amount_mismatch',
    severity: 'medium',
    localValue: '100,00',
    pspValue: '90,00',
    amountDeltaCents: -1_000,
    detectedAt: '2026-07-12T10:00:00Z',
  }

  it('força ledger_missing a crítico, seja qual for a severidade do backend', () => {
    // Dinheiro confirmado sem lançamento nunca é "atenção".
    expect(adaptDiscrepancySeverity('ledger_missing', 'low')).toBe('critical')
    expect(adaptDiscrepancySeverity('ledger_missing', 'info')).toBe('critical')
  })

  it('traduz a escala textual do backend', () => {
    expect(adaptDiscrepancySeverity('amount_mismatch', 'high')).toBe('critical')
    expect(adaptDiscrepancySeverity('amount_mismatch', 'medium')).toBe('warn')
    expect(adaptDiscrepancySeverity('amount_mismatch', 'low')).toBe('ok')
  })

  it('trata severidade desconhecida como atenção, nunca como normal', () => {
    expect(adaptDiscrepancySeverity('amount_mismatch', 'sei-la')).toBe('warn')
  })

  it('mapeia os tipos de divergência', () => {
    expect(adaptDiscrepancy(d).type).toBe('amount_mismatch')
    expect(adaptDiscrepancy({ ...d, kind: 'ledger_missing' }).type).toBe('missing_in_ledger')
    expect(adaptDiscrepancy({ ...d, kind: 'missing_at_psp' }).type).toBe('missing_in_psp')
  })

  it('compõe a nota a partir dos dois lados quando não há detalhe', () => {
    expect(adaptDiscrepancy(d).note).toContain('100,00')
    expect(adaptDiscrepancy(d).note).toContain('90,00')
  })

  it('soma o delta total do relatório', () => {
    const r = adaptReconciliation(period, [d, { ...d, id: 'd-2', amountDeltaCents: -500 }])
    expect(r.divergentCount).toBe(2)
    expect(r.totalDeltaCents).toBe(-1_500)
  })
})

describe('trilha de auditoria', () => {
  const rec: ApiAuditRecord = {
    Seq: 42,
    OccurredAt: '2026-07-15T09:00:00Z',
    Service: 'payment-service',
    ActorID: 'u-1',
    ActorRole: 'admin',
    ActorIP: '189.12.34.56',
    ActorUserAgent: 'Mozilla',
    EntityType: 'ledger_export',
    EntityID: 'csv',
    Action: 'export',
    OldValue: null,
    NewValue: { format: 'csv' },
    RequestID: 'req-1',
    PrevHash: 'aaa',
    Hash: 'bbb',
  }

  it('lê o PascalCase que o pkg/audit realmente serializa', () => {
    const e = adaptAuditRecord(rec)
    expect(e.sequence).toBe(42)
    expect(e.actorId).toBe('u-1')
    expect(e.hash).toBe('bbb')
    expect(e.prevHash).toBe('aaa')
  })

  it('trata o primeiro elo da cadeia (prevHash vazio) como gênese', () => {
    expect(adaptAuditRecord({ ...rec, PrevHash: '' }).prevHash).toBeNull()
  })

  it('mascara o último octeto do IP que o backend envia inteiro', () => {
    expect(maskIp('189.12.34.56')).toBe('189.12.34.xxx')
    expect(adaptAuditRecord(rec).ip).toBe('189.12.34.xxx')
  })

  it('não vaza IPv6 inteiro', () => {
    expect(maskIp('2001:db8:85a3:0:0:8a2e:370:7334')).toBe('2001:db8:…')
  })

  it('mapeia a ação genérica para o verbo de negócio da tela', () => {
    expect(adaptAuditAction('ledger_export', 'export')).toBe('admin_access')
    expect(adaptAuditAction('order', 'access')).toBe('admin_access')
    expect(adaptAuditAction('order', 'approve')).toBe('discount_approval')
    expect(adaptAuditAction('user', 'update')).toBe('user_role_change')
    expect(adaptAuditAction('product', 'update')).toBe('price_change')
  })

  it('serializa o valor de/para em texto legível', () => {
    expect(auditValueToText(null)).toBeNull()
    expect(auditValueToText({})).toBeNull()
    expect(auditValueToText('pago')).toBe('pago')
    expect(auditValueToText({ status: 'pago' })).toBe('status: pago')
    expect(auditValueToText({ a: 1, b: 2 })).toBe('a: 1 · b: 2')
  })

  it('papel desconhecido vira system em vez de quebrar a renderização', () => {
    expect(adaptAuditRecord({ ...rec, ActorRole: 'robo' }).actorRole).toBe('system')
  })
})

describe('adaptChainVerification', () => {
  it('mapeia a cadeia íntegra', () => {
    const v = adaptChainVerification({ valid: true, headSeq: 900, headHash: 'ff' })
    expect(v.valid).toBe(true)
    expect(v.checkedCount).toBe(900)
    expect(v.brokenAtSequence).toBeNull()
  })

  it('preserva a posição do rompimento', () => {
    const v = adaptChainVerification({
      valid: false,
      headSeq: 900,
      headHash: 'ff',
      brokenAtSeq: 317,
    })
    expect(v.valid).toBe(false)
    expect(v.brokenAtSequence).toBe(317)
  })
})
