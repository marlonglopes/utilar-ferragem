import { expect, type Page } from '@playwright/test'

/**
 * Helpers e seletores compartilhados entre os specs.
 *
 * Rótulos vêm do i18n pt-BR (src/i18n/pt-BR). Preferimos seletores por
 * role/label/texto (resilientes a mudança de markup) e navegação por URL.
 */

export const routes = {
  home: '/',
  category: (slug: string) => `/categoria/${slug}`,
  product: (slug: string) => `/produto/${slug}`,
  search: (q: string) => `/busca?q=${encodeURIComponent(q)}`,
  cart: '/carrinho',
  login: '/entrar',
  register: '/cadastro',
  forgot: '/esqueci-senha',
  account: '/conta',
  checkout: '/checkout',
  ui: '/_dev/ui',
}

// Credenciais: em modo mock qualquer e-mail/senha loga. Estas são as do seed
// real, caso os testes rodem contra a stack live (E2E_BASE_URL).
export const creds = {
  customer: { email: 'test1@utilar.com.br', password: 'utilar123' },
  admin: { email: 'admin@utilar.com.br', password: 'utilar123' },
}

/**
 * Semeia a sessão direto no storage (zustand persist, chave `utilar-auth`),
 * SEM passar pelo formulário. É como o e2e alcança rotas por PAPEL (/admin,
 * /balcao) no modo mock — o papel fica garantido, não dependendo de o mock de
 * login derivar admin/operador do e-mail. addInitScript roda antes dos scripts
 * da página, então o store já nasce logado.
 */
export async function authAs(
  page: Page,
  role: 'admin' | 'store_operator' | 'customer' | 'contador' | 'vendas' | 'almoxarife',
  email?: string
) {
  const user = {
    id: '00000000-0000-4000-8000-000000000099',
    name: `${role} E2E`,
    email: email ?? `${role}@utilar.com.br`,
    role,
    token: 'e2e-mock-token',
  }
  await page.addInitScript((u) => {
    window.localStorage.setItem('utilar-auth', JSON.stringify({ state: { user: u }, version: 0 }))
  }, user)
}

/** Faz login pela página /entrar e espera o redirect pra fora do /entrar. */
export async function login(
  page: Page,
  email = creds.customer.email,
  password = creds.customer.password
) {
  await page.goto(routes.login)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).not.toHaveURL(/\/entrar/, { timeout: 10_000 })
}

/** Link de um card de produto na vitrine (href /produto/:slug). */
export function productLinks(page: Page) {
  return page.locator('a[href^="/produto/"]')
}

/** Abre o primeiro produto listado e retorna a URL do produto. */
export async function openFirstProduct(page: Page): Promise<string> {
  const first = productLinks(page).first()
  await expect(first).toBeVisible()
  await first.click()
  await expect(page).toHaveURL(/\/produto\//)
  return page.url()
}

/** Adiciona o produto atual (página de detalhe) ao carrinho. */
export async function addToCartFromDetail(page: Page) {
  const btn = page.getByRole('button', { name: /Adicionar ao carrinho|Adicionado!/ }).first()
  await expect(btn).toBeVisible()
  await btn.click()
}

/** Botão do carrinho na navbar (abre o drawer). */
export function cartButton(page: Page) {
  return page.getByRole('button', { name: 'Carrinho' }).first()
}

/** Contador do badge do carrinho na navbar (0 se ausente). */
export async function cartCount(page: Page): Promise<number> {
  const txt =
    (await cartButton(page)
      .innerText()
      .catch(() => '')) || ''
  const m = txt.match(/\d+/)
  return m ? Number(m[0]) : 0
}
