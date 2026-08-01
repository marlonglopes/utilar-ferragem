import { test, expect } from '@playwright/test'
import { authAs } from './helpers'

/**
 * Cobertura de navegador do PAINEL ADMIN.
 *
 * O e2e existente cobria só a vitrine (cliente). O admin — onde o dono mexe em
 * pedido, custo, categoria e operador — não tinha nenhum teste de navegador,
 * então uma tela podia quebrar no bundle (lazy import, hook, markup) sem a
 * suíte perceber. Estes testes sobem cada tela de operação como o dono sobe.
 *
 * Escopo DELIBERADO: renderização, não autorização. Em modo mock/dev o
 * `isDevBypass()` do AdminRoute libera o painel de propósito (para ser
 * demonstrável sem backend), então o guard de papel NÃO é exercitável aqui — e
 * já é coberto por `src/test/adminRoute.test.tsx` com `isAuthEnabled` real.
 *
 * `authAs` semeia o papel admin no storage para o chrome refletir um dono
 * logado (e-mail no cabeçalho, ações de operação).
 */

// Todas as telas do AdminShell. O badge "Utilar · Admin" é o landmark estável
// (aparece em todas), e cada tela tem um <h1> com o próprio título.
const ADMIN_PAGES = [
  '/admin',
  '/admin/pedidos',
  '/admin/estoque',
  '/admin/devolucoes',
  '/admin/atividade',
  '/admin/operadores',
  '/admin/categorias',
  '/admin/produtos',
  '/admin/contabil',
  '/admin/importar',
  '/admin/avaliacoes',
  '/admin/frete',
]

test.describe('Admin — painel de operação', () => {
  test('o dono acessa o painel', async ({ page }) => {
    await authAs(page, 'admin')
    await page.goto('/admin')
    // Badge fixo do chrome do admin — prova que o AdminShell montou.
    await expect(page.getByText('Utilar · Admin')).toBeVisible()
  })

  // Menu filtrado por PERSONA — a metade visível do requisito. Cada papel vê só
  // as suas seções (a fronteira real é o 403 do backend; isto é o conforto).
  // Assertivas por href (robustas a viewport: mobile mostra rótulo curto).
  // Locator CSS (não getByRole): no mobile a barra fica display:none atrás do
  // hambúrguer e sai da árvore de acessibilidade — getByRole não a acharia. O
  // que queremos medir é quais links são RENDERIZADOS (filtrados por papel),
  // presentes no DOM mesmo colapsados.
  const nav = (page: import('@playwright/test').Page) =>
    page.locator('nav[aria-label="Seções do painel"]')

  test('contador vê contábil/pedidos, NÃO vê produtos/operadores', async ({ page }) => {
    await authAs(page, 'contador')
    await page.goto('/admin/contabil')
    const n = nav(page)
    await expect(n.locator('a[href^="/admin/contabil"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/pedidos"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/produtos"]')).toHaveCount(0)
    await expect(n.locator('a[href^="/admin/operadores"]')).toHaveCount(0)
  })

  test('vendas vê produtos/categorias/pedidos, NÃO vê contábil/operadores', async ({ page }) => {
    await authAs(page, 'vendas')
    await page.goto('/admin/produtos')
    const n = nav(page)
    await expect(n.locator('a[href^="/admin/produtos"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/categorias"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/pedidos"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/contabil"]')).toHaveCount(0)
    await expect(n.locator('a[href^="/admin/operadores"]')).toHaveCount(0)
  })

  test('almoxarife vê pedidos + estoque, NÃO vê produtos/contábil', async ({ page }) => {
    await authAs(page, 'almoxarife')
    await page.goto('/admin/estoque')
    const n = nav(page)
    await expect(n.locator('a[href^="/admin/pedidos"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/estoque"]')).toHaveCount(1)
    await expect(n.locator('a[href^="/admin/produtos"]')).toHaveCount(0)
    await expect(n.locator('a[href^="/admin/contabil"]')).toHaveCount(0)
  })

  for (const path of ADMIN_PAGES) {
    test(`a tela ${path} renderiza sem quebrar`, async ({ page }) => {
      const errors: string[] = []
      page.on('pageerror', (e) => errors.push(String(e)))

      const res = await page.goto(path)
      expect(res?.status(), `HTTP de ${path}`).toBeLessThan(400)
      // O chrome montou e a página tem um cabeçalho — não é uma tela em branco
      // nem a fronteira de erro do lazy chunk.
      await expect(page.getByText('Utilar · Admin')).toBeVisible()
      await expect(page.locator('h1').first()).toBeVisible()

      expect(errors, `erros de runtime em ${path}`).toEqual([])
    })
  }
})
