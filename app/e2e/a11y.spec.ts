import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { routes } from './helpers'

/**
 * Acessibilidade (a11y) — varredura axe-core nas páginas públicas principais.
 *
 * Roda contra a SPA em modo mock (mesma stack do restante do e2e). Falha se
 * houver violação de nível A/AA de WCAG 2.1 — que é o piso legal no varejo
 * brasileiro (Lei 13.146/2015, LBI). Contraste, rótulo de formulário, ordem de
 * cabeçalho, `alt` de imagem, landmarks: tudo que impede leitor de tela ou
 * navegação só-teclado de funcionar.
 *
 * Escopo A/AA de propósito: AAA é aspiracional e gera ruído que ninguém corrige.
 * Rodamos só no projeto chromium (o motor de a11y é o mesmo no mobile; duplicar
 * só dobra o tempo sem achar violação nova).
 */

// Páginas que um cliente anônimo alcança sem login — o caminho crítico da loja.
const PAGINAS: Array<{ nome: string; url: string }> = [
  { nome: 'home / vitrine', url: routes.home },
  { nome: 'carrinho', url: routes.cart },
  { nome: 'login', url: routes.login },
  { nome: 'cadastro', url: routes.register },
]

test.describe('Acessibilidade (WCAG 2.1 A/AA)', () => {
  for (const { nome, url } of PAGINAS) {
    test(`${nome} sem violações A/AA`, async ({ page }, testInfo) => {
      // a11y é comportamento do DOM, não do viewport — roda uma vez (chromium).
      test.skip(testInfo.project.name !== 'chromium', 'a11y roda só no chromium')
      await page.goto(url)
      await expect(page.locator('body')).toBeVisible()

      const resultado = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .analyze()

      // Mensagem útil no fail: qual regra, qual seletor. Log inteiro do axe é
      // ilegível; extraímos só o essencial pra quem for corrigir.
      const resumo = resultado.violations.map(
        (v) => `  [${v.impact}] ${v.id}: ${v.help} (${v.nodes.length}x) → ${v.nodes[0]?.target.join(' ')}`,
      )
      expect(resultado.violations, `Violações a11y em ${nome}:\n${resumo.join('\n')}`).toEqual([])
    })
  }

  // A página de detalhe do produto é gerada a partir de dado — regride sozinha
  // (imagem sem alt, botão sem nome acessível) quando o card muda. Cobrimos uma.
  test('detalhe do produto sem violações A/AA', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'chromium', 'a11y roda só no chromium')
    await page.goto(routes.home)
    const primeiro = page.locator('a[href^="/produto/"]').first()
    await expect(primeiro).toBeVisible()
    await primeiro.click()
    await expect(page).toHaveURL(/\/produto\//)

    const resultado = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .analyze()
    const resumo = resultado.violations.map(
      (v) => `  [${v.impact}] ${v.id}: ${v.help} (${v.nodes.length}x) → ${v.nodes[0]?.target.join(' ')}`,
    )
    expect(resultado.violations, `Violações a11y no detalhe do produto:\n${resumo.join('\n')}`).toEqual([])
  })
})
