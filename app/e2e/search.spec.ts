import { test, expect } from '@playwright/test'
import { routes, productLinks } from './helpers'

test.describe('Busca e filtros', () => {
  test('busca via URL retorna resultados', async ({ page }) => {
    await page.goto(routes.search('furadeira'))
    await expect(page).toHaveURL(/\/busca\?q=furadeira/)
    await expect(productLinks(page).first()).toBeVisible()
  })

  test('busca pela navbar navega para /busca', async ({ page }, testInfo) => {
    // A barra de busca por digitação vive no header desktop; no mobile a busca
    // é acessada por outro fluxo (a busca por URL acima cobre ambos).
    test.skip(testInfo.project.name === 'mobile', 'busca por digitação é desktop-only')
    await page.goto(routes.home)
    const box = page.getByRole('searchbox').or(page.getByPlaceholder(/buscar/i)).first()
    await box.click()
    await box.fill('cimento')
    await box.press('Enter')
    await expect(page).toHaveURL(/\/busca/)
  })

  test('busca sem resultado mostra estado vazio', async ({ page }) => {
    await page.goto(routes.search('zzxxqq-produto-inexistente-999'))
    // Não deve haver cards de produto
    await expect(productLinks(page)).toHaveCount(0)
  })
})
