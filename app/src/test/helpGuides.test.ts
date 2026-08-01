import { describe, it, expect } from 'vitest'
import { GUIDES, GUIDE_CATEGORIES, findGuide, guidesByCategory } from '@/lib/helpGuides'

describe('helpGuides — guias de operação da equipe', () => {
  it('todo guia tem slug, título, categoria válida, resumo e blocos', () => {
    for (const g of GUIDES) {
      expect(g.slug, g.slug).toMatch(/^[a-z0-9-]+$/)
      expect(g.title.length).toBeGreaterThan(3)
      expect(g.summary.length).toBeGreaterThan(5)
      expect(g.blocks.length).toBeGreaterThan(0)
      expect(GUIDE_CATEGORIES).toContain(g.category)
    }
  })

  it('slugs são únicos (deep-link não colide)', () => {
    const slugs = GUIDES.map((g) => g.slug)
    expect(new Set(slugs).size).toBe(slugs.length)
  })

  it('cobre as features-chave da sessão', () => {
    for (const slug of [
      'balcao-fazer-venda',
      'balcao-desconto-aprovacao',
      'imagens-em-lote',
      'estoque-ajuste',
      'devolucoes',
      'frete',
      'personas-acesso',
      'auditoria-atividade',
    ]) {
      expect(findGuide(slug), slug).toBeDefined()
    }
  })

  it('findGuide devolve undefined para slug inexistente', () => {
    expect(findGuide('nao-existe')).toBeUndefined()
  })

  it('guidesByCategory agrupa todos os guias', () => {
    const grouped = guidesByCategory()
    const total = Object.values(grouped).reduce((n, arr) => n + arr.length, 0)
    expect(total).toBe(GUIDES.length)
  })
})
