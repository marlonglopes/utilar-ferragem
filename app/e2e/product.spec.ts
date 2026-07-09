import { test, expect } from '@playwright/test'
import { routes } from './helpers'

// Slug presente no mockProducts.ts (modo mock).
const SLUG = 'furadeira-bosch-gsb-13-re'

test.describe('Página de produto', () => {
  test('renderiza nome, preço e CTA', async ({ page }) => {
    await page.goto(routes.product(SLUG))
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    // Preço em BRL (R$)
    await expect(page.getByText(/R\$\s?\d/).first()).toBeVisible()
    await expect(page.getByRole('button', { name: /Adicionar ao carrinho/ }).first()).toBeVisible()
  })

  test('tabs de descrição/specs/avaliações existem', async ({ page }) => {
    await page.goto(routes.product(SLUG))
    // Pelo menos uma aba clicável (Descrição / Especificações / Avaliações)
    const tab = page.getByRole('button', { name: /Descri|Especifica|Avalia/i }).first()
    await expect(tab).toBeVisible()
  })

  test('produto inexistente não quebra a página', async ({ page }) => {
    await page.goto(routes.product('slug-que-nao-existe-999'))
    await expect(page.locator('body')).toBeVisible()
  })
})
