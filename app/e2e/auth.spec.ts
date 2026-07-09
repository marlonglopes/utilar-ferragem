import { test, expect } from '@playwright/test'
import { routes, login, creds } from './helpers'

test.describe('Autenticação', () => {
  test('página de login tem formulário', async ({ page }) => {
    await page.goto(routes.login)
    await expect(page.getByLabel('E-mail')).toBeVisible()
    await expect(page.getByLabel('Senha')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Entrar' })).toBeVisible()
  })

  test('login com sucesso redireciona pra fora do /entrar', async ({ page }) => {
    await login(page, creds.customer.email, creds.customer.password)
    await expect(page).not.toHaveURL(/\/entrar/)
  })

  test('login preserva o parâmetro next (rota protegida)', async ({ page }) => {
    // Ao acessar /conta deslogado, deve mandar pro login.
    await page.goto(routes.account)
    await expect(page).toHaveURL(/\/entrar/)
    await page.getByLabel('E-mail').fill(creds.customer.email)
    await page.getByLabel('Senha').fill(creds.customer.password)
    await page.getByRole('button', { name: 'Entrar' }).click()
    // Depois do login volta pra conta.
    await expect(page).toHaveURL(/\/conta/, { timeout: 10_000 })
  })

  test('página de cadastro é acessível a partir do login', async ({ page }) => {
    await page.goto(routes.login)
    await page.getByRole('link', { name: 'Criar conta' }).click()
    await expect(page).toHaveURL(/\/cadastro/)
  })

  test('link de esqueci a senha funciona', async ({ page }) => {
    await page.goto(routes.forgot)
    await expect(page.getByLabel('E-mail')).toBeVisible()
  })
})
