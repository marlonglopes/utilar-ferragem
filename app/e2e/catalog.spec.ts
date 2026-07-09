import { test, expect } from '@playwright/test'
import { routes, openFirstProduct, productLinks } from './helpers'

test.describe('Catálogo — vitrine e navegação', () => {
  test('home lista produtos com link de detalhe', async ({ page }) => {
    await page.goto(routes.home)
    const count = await productLinks(page).count()
    expect(count).toBeGreaterThan(0)
  })

  test('página de categoria mostra grid de produtos', async ({ page }) => {
    await page.goto(routes.category('ferramentas'))
    await expect(page).toHaveURL(/\/categoria\/ferramentas/)
    await expect(productLinks(page).first()).toBeVisible()
  })

  test('clicar num produto abre a página de detalhe', async ({ page }) => {
    await page.goto(routes.home)
    const url = await openFirstProduct(page)
    expect(url).toMatch(/\/produto\/[a-z0-9-]+/)
    // Detalhe tem título (h1) e CTA de compra
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.getByRole('button', { name: /Adicionar ao carrinho/ }).first()).toBeVisible()
  })
})
