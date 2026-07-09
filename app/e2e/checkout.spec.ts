import { test, expect } from '@playwright/test'
import { routes, login, openFirstProduct, addToCartFromDetail } from './helpers'

test.describe('Checkout', () => {
  test('checkout é rota protegida — desloga redireciona pro login', async ({ page }) => {
    await page.goto(routes.checkout)
    await expect(page).toHaveURL(/\/entrar/)
  })

  test('fluxo: login + produto no carrinho → checkout carrega', async ({ page }) => {
    await login(page)

    // Adiciona um produto ao carrinho
    await page.goto(routes.home)
    await openFirstProduct(page)
    await addToCartFromDetail(page)

    // Vai pro checkout
    await page.goto(routes.checkout)
    await expect(page).toHaveURL(/\/checkout/)
    await expect(page.locator('body')).toBeVisible()
    // Deve mostrar algum passo do checkout (endereço/entrega/pagamento)
    await expect(
      page.getByText(/endereço|entrega|pagamento|frete/i).first(),
    ).toBeVisible()
  })

  test('métodos de pagamento Pix/boleto/cartão aparecem no checkout', async ({ page }) => {
    await login(page)
    await page.goto(routes.home)
    await openFirstProduct(page)
    await addToCartFromDetail(page)
    await page.goto(routes.checkout)

    // Pelo menos uma menção aos métodos de pagamento BR.
    await expect(page.getByText(/pix|boleto|cart[ãa]o/i).first()).toBeVisible()
  })
})
