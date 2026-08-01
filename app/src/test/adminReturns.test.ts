import { describe, it, expect } from 'vitest'
import { fetchReturns, RETURN_STATUS_LABEL } from '@/lib/adminReturnsApi'

// VITE_ORDER_URL vazio → mock. Cobre o contrato da fila de devoluções.
describe('adminReturnsApi (mock)', () => {
  it('lista a fila aberta e normaliza para camelCase', async () => {
    const rows = await fetchReturns('')
    expect(rows.length).toBeGreaterThan(0)
    // Normalizado (o backend serializa PascalCase; o front não deve ver isso).
    expect(rows[0]).toHaveProperty('orderId')
    expect(rows[0]).toHaveProperty('refundTotal')
    expect(rows[0]).not.toHaveProperty('OrderID')
    // A fila aberta esconde encerradas.
    expect(rows.every((r) => !['refunded', 'rejected'].includes(r.status))).toBe(true)
  })

  it('filtra por situação', async () => {
    const received = await fetchReturns('received')
    expect(received.every((r) => r.status === 'received')).toBe(true)
  })

  it('tem rótulo pt-BR para cada situação', () => {
    expect(RETURN_STATUS_LABEL.requested).toBe('Solicitada')
    expect(RETURN_STATUS_LABEL.refunded).toBe('Estornada')
  })
})
