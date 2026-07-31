import { describe, it, expect } from 'vitest'
import { fetchOperators, fetchStores, fetchUsers } from '@/lib/adminStaffApi'

// Em teste, VITE_AUTH_URL vazio → mocks determinísticos. Cobre o contrato que a
// tela de Staff usa (operadores, lojas, busca de usuário).
describe('adminStaffApi (modo mock)', () => {
  it('lista operadores', async () => {
    const ops = await fetchOperators()
    expect(ops.length).toBeGreaterThan(0)
    expect(ops[0]).toHaveProperty('userId')
    expect(ops[0]).toHaveProperty('discountCeilingPct')
  })

  it('lista lojas', async () => {
    const stores = await fetchStores()
    expect(stores.length).toBeGreaterThan(0)
    expect(stores[0]).toHaveProperty('code')
  })

  it('busca usuário por nome', async () => {
    const users = await fetchUsers('ana')
    expect(users.some((u) => u.name.toLowerCase().includes('ana'))).toBe(true)
  })

  it('busca vazia devolve todos (mock)', async () => {
    const users = await fetchUsers('')
    expect(users.length).toBeGreaterThan(0)
  })
})
