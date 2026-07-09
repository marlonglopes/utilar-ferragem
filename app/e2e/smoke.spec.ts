import { test, expect } from '@playwright/test'
import { routes } from './helpers'

test.describe('Smoke — app carrega e navega', () => {
  test('home carrega com navbar e produtos', async ({ page }) => {
    await page.goto(routes.home)
    // Navbar: botão do carrinho (abre o drawer)
    await expect(page.getByRole('button', { name: 'Carrinho' })).toBeVisible()
    // Vitrine tem pelo menos um produto
    await expect(page.locator('a[href^="/produto/"]').first()).toBeVisible()
  })

  test('rotas públicas principais respondem', async ({ page }) => {
    for (const path of [routes.cart, routes.login, routes.register, routes.forgot]) {
      const res = await page.goto(path)
      expect(res?.status(), `GET ${path}`).toBeLessThan(400)
      await expect(page.locator('body')).toBeVisible()
    }
  })

  test('rota inexistente mostra 404', async ({ page }) => {
    await page.goto('/rota-que-nao-existe-123')
    await expect(page.getByText(/404|não encontrad|not found/i).first()).toBeVisible()
  })

  test('design system /_dev/ui renderiza', async ({ page }) => {
    await page.goto(routes.ui)
    await expect(page.getByRole('button').first()).toBeVisible()
  })
})
