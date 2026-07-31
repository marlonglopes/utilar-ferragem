import { test, expect } from '@playwright/test'
import { authAs } from './helpers'

/**
 * Cobertura de navegador do BALCÃO (PDV).
 *
 * O PDV é feito para o tablet do vendedor e nunca teve teste de navegador. Um
 * erro de bundle/hook/markup derrubaria a venda de balcão sem a suíte perceber.
 *
 * Escopo DELIBERADO: renderização, não autorização. Em modo mock/dev o
 * `isDevBypass()` do BalcaoRoute libera o PDV de propósito — o guard de papel
 * (store_operator/admin sim; customer/seller não) já é coberto por
 * `src/test/balcaoRoute.test.tsx` com `isAuthEnabled` real.
 */

test.describe('Balcão (PDV)', () => {
  test('o operador abre o PDV', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (e) => errors.push(String(e)))

    await authAs(page, 'store_operator')
    await page.goto('/balcao')

    // "Nova comanda" é o controle sempre presente do PDV (ComandaTabs) — se ele
    // aparece, a tela principal montou.
    await expect(page.getByRole('button', { name: 'Nova comanda' })).toBeVisible()
    expect(errors, 'erros de runtime no /balcao').toEqual([])
  })

  test('a fila de aprovações renderiza', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (e) => errors.push(String(e)))

    await authAs(page, 'store_operator')
    await page.goto('/balcao/aprovacoes')

    // Duas telas legítimas: "Aprovações pendentes" (quem homologa desconto) e
    // "Fila de aprovação restrita" (operador comum, sem esse poder). Em mock o
    // operador semeado não homologa, então cai na segunda — as duas provam que
    // a página montou. O guard de poder já é testado no unit; aqui é smoke.
    await expect(
      page.getByRole('heading', { name: /Aprovações pendentes|Fila de aprovação restrita/ }),
    ).toBeVisible()
    expect(errors, 'erros de runtime em /balcao/aprovacoes').toEqual([])
  })

  test('o admin também alcança o balcão', async ({ page }) => {
    await authAs(page, 'admin')
    await page.goto('/balcao')
    await expect(page.getByRole('button', { name: 'Nova comanda' })).toBeVisible()
  })
})
