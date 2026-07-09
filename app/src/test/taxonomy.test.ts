import { describe, it, expect } from 'vitest'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'

describe('taxonomy', () => {
  it('tem as 8 categorias de topo', () => {
    expect(TOP_LEVEL_CATEGORIES).toHaveLength(8)
  })

  it('todo nó tem slug, labelKey e icon', () => {
    for (const node of TOP_LEVEL_CATEGORIES) {
      expect(node.slug).toBeTruthy()
      expect(node.labelKey).toMatch(/^taxonomy\./)
      expect(node.icon).toBeTruthy()
    }
  })

  it('slugs são únicos e URL-safe', () => {
    const slugs = TOP_LEVEL_CATEGORIES.map((n) => n.slug)
    expect(new Set(slugs).size).toBe(slugs.length)
    for (const s of slugs) {
      expect(s).toMatch(/^[a-z0-9-]+$/)
    }
  })

  it('inclui as categorias-chave da ferragem', () => {
    const slugs = TOP_LEVEL_CATEGORIES.map((n) => n.slug)
    for (const key of ['ferramentas', 'construcao', 'eletrica', 'fixacao']) {
      expect(slugs).toContain(key)
    }
  })
})
