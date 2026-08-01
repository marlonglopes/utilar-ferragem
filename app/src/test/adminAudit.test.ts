import { describe, it, expect } from 'vitest'
import { fetchAuditActivity } from '@/lib/adminAuditApi'

// Em teste, VITE_CATALOG_URL é vazio → mock determinístico. Cobre o contrato de
// filtro que a tela de Atividade usa (ação, ator).
describe('adminAuditApi (modo mock)', () => {
  it('lista e meta.total bate', async () => {
    const p = await fetchAuditActivity({})
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.meta.total).toBe(p.data.length)
  })

  it('filtra por ação', async () => {
    const p = await fetchAuditActivity({ action: 'product.update' })
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.data.every((e) => e.action === 'product.update')).toBe(true)
  })

  it('filtra por ator', async () => {
    const p = await fetchAuditActivity({ actor: 'admin' })
    expect(p.data.every((e) => (e.actorId ?? '').toLowerCase().includes('admin'))).toBe(true)
  })

  it('traz o de→para nas mudanças', async () => {
    const p = await fetchAuditActivity({ action: 'product.update' })
    const withPrice = p.data.find((e) => 'price' in e.changes)
    expect(withPrice?.changes.price).toHaveProperty('old')
    expect(withPrice?.changes.price).toHaveProperty('new')
  })

  it('unifica as três fontes: toda linha tem source, e há catalog/staff/operacao', async () => {
    const p = await fetchAuditActivity({})
    expect(p.data.every((e) => !!e.source)).toBe(true)
    const sources = new Set(p.data.map((e) => e.source))
    expect(sources.has('catalog')).toBe(true)
    expect(sources.has('staff')).toBe(true)
    expect(sources.has('operacao')).toBe(true)
  })

  it('ordena por data decrescente (mais recente primeiro) na trilha unificada', async () => {
    const p = await fetchAuditActivity({})
    for (let i = 1; i < p.data.length; i++) {
      expect(p.data[i - 1].createdAt >= p.data[i].createdAt).toBe(true)
    }
  })

  it('filtra por fonte (source=staff traz só staff)', async () => {
    const p = await fetchAuditActivity({ source: 'staff' })
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.data.every((e) => e.source === 'staff')).toBe(true)
    // e é a mudança de papel, com o de→para do papel
    const roleChange = p.data.find((e) => e.action === 'user.role.update')
    expect(roleChange?.changes.role).toMatchObject({ old: 'customer', new: 'vendas' })
  })

  it('eventos de operação carregam o valor (amount) do desconto/estorno', async () => {
    const p = await fetchAuditActivity({ source: 'operacao' })
    expect(p.data.some((e) => typeof e.amount === 'number' && e.amount > 0)).toBe(true)
  })
})
