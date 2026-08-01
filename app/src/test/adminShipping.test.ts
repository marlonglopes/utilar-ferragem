import { describe, it, expect } from 'vitest'
import {
  fetchRates,
  formatCep,
  looksLikeSaoPaulo,
  parseCep,
  type ShippingRate,
} from '@/lib/adminShippingApi'

describe('adminShippingApi (mock)', () => {
  it('formata e parseia CEP (8 dígitos)', () => {
    expect(formatCep(97000000)).toBe('97000-000')
    expect(parseCep('97000-000')).toBe(97000000)
    expect(parseCep('97000000')).toBe(97000000)
    expect(parseCep('123')).toBeNull() // menos de 8 dígitos
  })

  it('detecta faixa de São Paulo (o bug do seed)', () => {
    const sp: ShippingRate = {
      id: 'x',
      zoneName: 'Capital SP',
      cepStart: 1000000,
      cepEnd: 5999999,
      serviceCode: 'standard',
      serviceName: 'Padrão',
      baseCost: 10,
      costPerItem: 0,
      deliveryDays: 3,
      freeAbove: 0,
      active: true,
    }
    const rs: ShippingRate = { ...sp, zoneName: 'RS', cepStart: 97000000, cepEnd: 97999999 }
    expect(looksLikeSaoPaulo(sp)).toBe(true)
    expect(looksLikeSaoPaulo(rs)).toBe(false)
  })

  it('lista as faixas do mock (que reproduz o seed de SP)', async () => {
    const rates = await fetchRates()
    expect(rates.length).toBeGreaterThan(0)
    // O mock reproduz o problema para a tela mostrar o aviso.
    expect(rates.some(looksLikeSaoPaulo)).toBe(true)
  })
})
