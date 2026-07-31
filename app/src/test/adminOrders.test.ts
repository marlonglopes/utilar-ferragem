import { describe, it, expect } from 'vitest'
import { fetchAdminOrders } from '@/lib/adminOrdersApi'

// Em teste, VITE_ORDER_URL é vazio → o cliente cai no mock determinístico.
// Cobre o contrato de filtro que a tela de Pedidos usa (status, canal, busca).
describe('adminOrdersApi (modo mock)', () => {
  it('lista todos por padrão e meta.total bate', async () => {
    const p = await fetchAdminOrders({})
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.meta.total).toBe(p.data.length)
  })

  it('filtra por status específico', async () => {
    const p = await fetchAdminOrders({ status: 'paid' })
    expect(p.data.length).toBeGreaterThan(0)
    expect(p.data.every((o) => o.status === 'paid')).toBe(true)
  })

  it('filtra pelo grupo "em aberto" (active)', async () => {
    const p = await fetchAdminOrders({ status: 'active' })
    expect(
      p.data.every((o) => ['pending_payment', 'paid', 'picking', 'shipped'].includes(o.status))
    ).toBe(true)
  })

  it('filtra por canal balcão', async () => {
    const p = await fetchAdminOrders({ channel: 'balcao' })
    expect(p.data.every((o) => o.channel === 'balcao')).toBe(true)
  })

  it('busca pelo nome do cliente', async () => {
    const p = await fetchAdminOrders({ q: 'ana' })
    expect(p.data.some((o) => (o.customerName ?? '').toLowerCase().includes('ana'))).toBe(true)
  })
})
