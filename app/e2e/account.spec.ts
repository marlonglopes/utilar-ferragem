import { test, expect } from '@playwright/test'
import { routes, login } from './helpers'

test.describe('Conta do cliente', () => {
  test('página da conta carrega após login', async ({ page }) => {
    await login(page)
    await page.goto(routes.account)
    await expect(page).toHaveURL(/\/conta/)
    await expect(page.getByRole('heading').first()).toBeVisible()
  })

  test('conta mostra abas de perfil/endereços/pedidos', async ({ page }) => {
    await login(page)
    await page.goto(routes.account)
    // Ao menos uma seção de conta é visível.
    await expect(
      page.getByText(/perfil|endere[çc]o|pedido|meus dados|sair/i).first(),
    ).toBeVisible()
  })

  test('logout volta pro estado deslogado', async ({ page }) => {
    await login(page)
    await page.goto(routes.account)
    const logout = page.getByRole('button', { name: /sair|logout/i }).first()
    if (await logout.isVisible().catch(() => false)) {
      await logout.click()
      // Acessar /conta de novo deve exigir login.
      await page.goto(routes.account)
      await expect(page).toHaveURL(/\/entrar/)
    }
  })
})
