import { describe, it, expect } from 'vitest'
import {
  canAccessAdmin,
  isStaffRole,
  landingFor,
  STAFF_ROLES,
  type StaffRole,
} from '@/lib/adminAccess'

/**
 * Matriz de persona do painel. Espelha o 403 do backend (order/catalog/payment/
 * auth). Se divergir do servidor, o servidor é que manda — mas manter o teste
 * aqui é o que impede a divergência passar despercebida no front.
 */
describe('adminAccess — matriz de persona', () => {
  it('reconhece só os papéis de operação como staff', () => {
    expect(STAFF_ROLES).toEqual(['admin', 'contador', 'vendas', 'almoxarife'])
    expect(isStaffRole('admin')).toBe(true)
    expect(isStaffRole('contador')).toBe(true)
    expect(isStaffRole('customer')).toBe(false)
    expect(isStaffRole('seller')).toBe(false)
    expect(isStaffRole('store_operator')).toBe(false) // balcão, não painel
    expect(isStaffRole(undefined)).toBe(false)
  })

  it('admin abre tudo', () => {
    for (const p of [
      '/admin',
      '/admin/contabil',
      '/admin/pedidos',
      '/admin/produtos',
      '/admin/operadores',
      '/admin/trilha',
      '/admin/atividade',
    ]) {
      expect(canAccessAdmin(p, 'admin'), p).toBe(true)
    }
  })

  it('contador: contábil/trilha/observabilidade/pedidos SIM; catálogo e staff NÃO', () => {
    expect(canAccessAdmin('/admin/contabil', 'contador')).toBe(true)
    expect(canAccessAdmin('/admin/trilha', 'contador')).toBe(true)
    expect(canAccessAdmin('/admin/observabilidade', 'contador')).toBe(true)
    expect(canAccessAdmin('/admin/pedidos', 'contador')).toBe(true) // leitura
    // NÃO vê custo (catálogo) nem gere staff, nem a visão geral (margem):
    expect(canAccessAdmin('/admin/produtos', 'contador')).toBe(false)
    expect(canAccessAdmin('/admin/atividade', 'contador')).toBe(false) // trilha do catálogo tem custo
    expect(canAccessAdmin('/admin/operadores', 'contador')).toBe(false)
    expect(canAccessAdmin('/admin', 'contador')).toBe(false)
  })

  it('vendas: catálogo/pedidos/atividade SIM; contábil e staff NÃO', () => {
    expect(canAccessAdmin('/admin/produtos', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/produtos/novo', 'vendas')).toBe(true) // sub-rota
    expect(canAccessAdmin('/admin/produtos/abc-123', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/categorias', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/importar', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/atividade', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/pedidos', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/contabil', 'vendas')).toBe(false)
    expect(canAccessAdmin('/admin/operadores', 'vendas')).toBe(false)
    expect(canAccessAdmin('/admin', 'vendas')).toBe(false)
  })

  it('almoxarife: pedidos + estoque; nada de custo/contábil/staff', () => {
    expect(canAccessAdmin('/admin/pedidos', 'almoxarife')).toBe(true)
    expect(canAccessAdmin('/admin/estoque', 'almoxarife')).toBe(true) // sem custo
    expect(canAccessAdmin('/admin/produtos', 'almoxarife')).toBe(false)
    expect(canAccessAdmin('/admin/contabil', 'almoxarife')).toBe(false)
    expect(canAccessAdmin('/admin/operadores', 'almoxarife')).toBe(false)
    expect(canAccessAdmin('/admin', 'almoxarife')).toBe(false)
  })

  it('estoque: admin/almoxarife/vendas SIM; contador NÃO', () => {
    expect(canAccessAdmin('/admin/estoque', 'admin')).toBe(true)
    expect(canAccessAdmin('/admin/estoque', 'vendas')).toBe(true)
    expect(canAccessAdmin('/admin/estoque', 'contador')).toBe(false)
  })

  it('papel não-staff nunca acessa (fail-closed)', () => {
    for (const role of ['customer', 'seller', 'store_operator', '', undefined]) {
      expect(canAccessAdmin('/admin/pedidos', role as string)).toBe(false)
    }
  })

  it('rota de admin não mapeada → só admin (fail-closed)', () => {
    expect(canAccessAdmin('/admin/rota-nova-qualquer', 'admin')).toBe(true)
    expect(canAccessAdmin('/admin/rota-nova-qualquer', 'vendas')).toBe(false)
  })

  it('landingFor leva cada persona à primeira seção que ela abre', () => {
    const expected: Record<StaffRole, string> = {
      admin: '/admin',
      contador: '/admin/contabil',
      vendas: '/admin/pedidos',
      almoxarife: '/admin/pedidos',
    }
    for (const role of STAFF_ROLES) {
      const home = landingFor(role)
      expect(home, role).toBe(expected[role])
      // e o home é, de fato, acessível pra ela (sem loop de redirect):
      expect(canAccessAdmin(home, role), `${role} acessa o próprio home`).toBe(true)
    }
  })
})
