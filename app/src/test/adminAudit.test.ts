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
})
