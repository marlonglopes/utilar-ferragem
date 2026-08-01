import { describe, it, expect } from 'vitest'
import { fetchReviews, REVIEW_STATUS_LABEL } from '@/lib/adminReviewsApi'

// VITE_CATALOG_URL vazio → mock. Contrato da fila de moderação.
describe('adminReviewsApi (mock)', () => {
  it('lista pendentes por padrão', async () => {
    const rows = await fetchReviews('pending')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.every((r) => r.status === 'pending')).toBe(true)
    expect(rows[0]).toHaveProperty('rating')
    expect(rows[0]).toHaveProperty('authorName')
  })

  it('separa por estado', async () => {
    const pub = await fetchReviews('published')
    expect(pub.every((r) => r.status === 'published')).toBe(true)
    const rej = await fetchReviews('rejected')
    expect(rej.every((r) => r.status === 'rejected')).toBe(true)
  })

  it('rótulos pt-BR', () => {
    expect(REVIEW_STATUS_LABEL.pending).toBe('Pendentes')
    expect(REVIEW_STATUS_LABEL.published).toBe('Publicadas')
  })
})
