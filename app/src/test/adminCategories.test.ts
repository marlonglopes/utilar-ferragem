import { describe, it, expect } from 'vitest'
import { fetchCategories, isValidCategoryId } from '@/lib/adminCategoriesApi'

describe('adminCategoriesApi', () => {
  it('lista categorias (mock)', async () => {
    const cats = await fetchCategories()
    expect(cats.length).toBeGreaterThan(0)
    expect(cats[0]).toHaveProperty('id')
    expect(cats[0]).toHaveProperty('name')
  })

  it('valida o slug do id', () => {
    expect(isValidCategoryId('fechaduras')).toBe(true)
    expect(isValidCategoryId('material-eletrico')).toBe(true)
    expect(isValidCategoryId('Fechaduras')).toBe(false) // maiúscula
    expect(isValidCategoryId('com espaço')).toBe(false)
    expect(isValidCategoryId('acentuação')).toBe(false)
    expect(isValidCategoryId('')).toBe(false)
  })
})
