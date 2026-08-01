import { describe, it, expect } from 'vitest'
import { fetchStock, fetchMovements, STOCK_REASONS } from '@/lib/adminStockApi'

// VITE_CATALOG_URL vazio em teste → mock determinístico. Cobre o contrato que a
// tela de Estoque usa.
describe('adminStockApi (mock)', () => {
  it('lista com alerta de baixo e sobe os baixos ao topo', async () => {
    const p = await fetchStock({})
    expect(p.data.length).toBeGreaterThan(0)
    // Os de estoque baixo vêm primeiro.
    const firstNonLow = p.data.findIndex((r) => !r.lowStock)
    const lastLow = p.data.map((r) => r.lowStock).lastIndexOf(true)
    if (firstNonLow !== -1 && lastLow !== -1) {
      expect(lastLow).toBeLessThan(firstNonLow)
    }
    // lowStock é derivado de stock <= limite.
    for (const r of p.data) {
      expect(r.lowStock).toBe(r.stock <= r.lowStockThreshold)
    }
  })

  it('filtra só estoque baixo', async () => {
    const p = await fetchStock({ low: true })
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.data.every((r) => r.lowStock)).toBe(true)
  })

  it('busca por nome/sku', async () => {
    const p = await fetchStock({ q: 'fechadura' })
    expect(p.data.every((r) => r.name.toLowerCase().includes('fechadura'))).toBe(true)
  })

  it('NUNCA traz custo (o almoxarife não vê custo)', async () => {
    const p = await fetchStock({})
    for (const r of p.data) {
      expect('cost' in r).toBe(false)
    }
  })

  it('histórico de movimento traz delta + motivo + estoque resultante', async () => {
    const moves = await fetchMovements('p-1')
    expect(moves.length).toBeGreaterThan(0)
    expect(moves[0]).toHaveProperty('delta')
    expect(moves[0]).toHaveProperty('reason')
    expect(moves[0]).toHaveProperty('resultingStock')
  })

  it('oferece motivos padronizados', () => {
    expect(STOCK_REASONS).toContain('Recebimento de fornecedor')
    expect(STOCK_REASONS.length).toBeGreaterThan(2)
  })
})
