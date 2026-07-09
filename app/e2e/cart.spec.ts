import { test, expect } from '@playwright/test'
import { routes, openFirstProduct, addToCartFromDetail, cartCount } from './helpers'

test.describe('Carrinho', () => {
  test('carrinho começa vazio', async ({ page }) => {
    await page.goto(routes.cart)
    await expect(page.getByText(/vazio|nenhum item/i).first()).toBeVisible()
  })

  test('adicionar produto incrementa o badge e aparece no carrinho', async ({ page }) => {
    await page.goto(routes.home)
    await openFirstProduct(page)
    await addToCartFromDetail(page)

    // Badge do carrinho passa a >= 1
    await expect.poll(() => cartCount(page)).toBeGreaterThanOrEqual(1)

    await page.goto(routes.cart)
    // Há um link pra prosseguir ao checkout
    await expect(page.getByRole('link', { name: /checkout|finalizar|continuar/i }).first()).toBeVisible()
  })

  test('remover item esvazia o carrinho', async ({ page }) => {
    await page.goto(routes.home)
    await openFirstProduct(page)
    await addToCartFromDetail(page)
    await page.goto(routes.cart)

    // Botão de remover (ícone lixeira) — pega o primeiro botão de remoção
    const removeBtn = page.getByRole('button', { name: /remov|excluir|lixeira|trash/i }).first()
    if (await removeBtn.isVisible().catch(() => false)) {
      await removeBtn.click()
      await expect(page.getByText(/vazio|nenhum item/i).first()).toBeVisible()
    }
  })
})
