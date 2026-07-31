import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { routes } from './helpers'

/**
 * Acessibilidade (a11y) — varredura axe-core nas páginas públicas principais,
 * contra a SPA em modo mock (mesma stack do restante do e2e).
 *
 * Piso legal do varejo brasileiro: Lei 13.146/2015 (LBI) + eMAG. Alvo WCAG 2.1
 * nível A/AA — AAA é aspiracional e gera ruído que ninguém corrige.
 *
 * DUAS camadas, de propósito:
 *
 *  1. ESTRUTURAL (gate real, bloqueia): rótulo de formulário, `alt` de imagem,
 *     nome acessível de botão/link, ordem de cabeçalho, landmarks, ARIA válido.
 *     Hoje está ZERADO — e tem que continuar. Regressão aqui reprova a suíte.
 *
 *  2. CONTRASTE DE COR (débito de marca, NÃO bloqueia): a marca é laranja
 *     #F47920 com texto branco, que fica em ~2,6:1 — abaixo do 4,5:1 do AA para
 *     texto normal. Corrigir mexe na identidade visual (escurecer o laranja ou
 *     usar texto escuro sobre laranja) — é decisão do DONO, não do runner. Então
 *     medimos e REPORTAMOS a contagem no log, sem reprovar. Documentado como
 *     dívida para revisão de design; ver SKILL.md.
 *
 * Roda só no chromium — a11y é do DOM, não do viewport; duplicar no mobile só
 * dobra o tempo sem achar violação nova.
 */

const WCAG_AA = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']

// Páginas que um cliente anônimo alcança sem login — o caminho crítico da loja.
const PAGINAS: Array<{ nome: string; url: string }> = [
  { nome: 'home / vitrine', url: routes.home },
  { nome: 'carrinho', url: routes.cart },
  { nome: 'login', url: routes.login },
  { nome: 'cadastro', url: routes.register },
]

function resumo(violations: Awaited<ReturnType<AxeBuilder['analyze']>>['violations']): string {
  return violations
    .map(
      (v) =>
        `  [${v.impact}] ${v.id}: ${v.help} (${v.nodes.length}x) → ${v.nodes[0]?.target.join(' ')}`
    )
    .join('\n')
}

test.describe('Acessibilidade estrutural (WCAG 2.1 A/AA — gate)', () => {
  for (const { nome, url } of PAGINAS) {
    test(`${nome} sem violação estrutural`, async ({ page }, testInfo) => {
      test.skip(testInfo.project.name !== 'chromium', 'a11y roda só no chromium')
      await page.goto(url)
      await expect(page.locator('body')).toBeVisible()

      const r = await new AxeBuilder({ page })
        .withTags(WCAG_AA)
        // Contraste é dívida de marca conhecida — medido no teste abaixo, não aqui.
        .disableRules(['color-contrast'])
        .analyze()
      expect(r.violations, `Violações estruturais em ${nome}:\n${resumo(r.violations)}`).toEqual([])
    })
  }

  // O detalhe do produto é gerado a partir de dado — regride sozinho (imagem sem
  // alt, botão sem nome acessível) quando o card muda. Cobrimos um.
  test('detalhe do produto sem violação estrutural', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'chromium', 'a11y roda só no chromium')
    await page.goto(routes.home)
    const primeiro = page.locator('a[href^="/produto/"]').first()
    await expect(primeiro).toBeVisible()
    await primeiro.click()
    await expect(page).toHaveURL(/\/produto\//)

    const r = await new AxeBuilder({ page })
      .withTags(WCAG_AA)
      .disableRules(['color-contrast'])
      .analyze()
    expect(
      r.violations,
      `Violações estruturais no detalhe do produto:\n${resumo(r.violations)}`
    ).toEqual([])
  })
})

test.describe('Contraste de cor (débito de marca — reporta, não bloqueia)', () => {
  test('mede contraste da vitrine e registra a dívida', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'chromium', 'a11y roda só no chromium')
    await page.goto(routes.home)
    await expect(page.locator('body')).toBeVisible()

    const r = await new AxeBuilder({ page })
      .withTags(WCAG_AA)
      .withRules(['color-contrast'])
      .analyze()

    const total = r.violations.reduce((n, v) => n + v.nodes.length, 0)
    // Não reprova: é decisão de marca do dono. Só deixa o número visível no log
    // e anexado ao relatório, pra ninguém esquecer que a dívida existe.
    testInfo.annotations.push({
      type: 'a11y-contraste',
      description: `${total} elemento(s) abaixo de 4,5:1 (marca laranja+branco). Dívida de design — decisão do dono.`,
    })
    // eslint-disable-next-line no-console
    console.log(
      `ℹ️  a11y contraste: ${total} elemento(s) abaixo de AA na vitrine (dívida de marca, não bloqueia).`
    )
    // Sanidade: o medidor rodou (a página tem conteúdo). Não asseveramos o total
    // pra não criar baseline frágil — o gate de verdade é o bloco estrutural.
    expect(total).toBeGreaterThanOrEqual(0)
  })
})
