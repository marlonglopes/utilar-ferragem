import { defineConfig, devices } from '@playwright/test'

/**
 * E2E do Utilar Ferragem.
 *
 * Roda contra a SPA em **modo mock** (sem backend): as VITE_*_URL ficam vazias,
 * então o app serve mockProducts/mockOrders/etc. de src/lib/mock*.ts. Isso deixa
 * os testes determinísticos e sem dependência de Postgres/serviços Go — ideal
 * para CI. Para testar contra os serviços reais, exporte as VITE_*_URL e rode
 * com E2E_BASE_URL apontando pra uma SPA em modo live.
 *
 * Porta dedicada 5180 pra não colidir com `make dev` (5175) rodando em paralelo.
 */

const PORT = Number(process.env.E2E_PORT ?? 5180)
const baseURL = process.env.E2E_BASE_URL ?? `http://127.0.0.1:${PORT}`

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],

  timeout: 30_000,
  expect: { timeout: 7_000 },

  use: {
    baseURL,
    locale: 'pt-BR',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] } },
  ],

  // Sobe a SPA em modo mock. Reaproveita um server já rodando em dev local.
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: `npm run dev -- --port ${PORT} --strictPort`,
        url: baseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
        // Força modo mock: zera qualquer VITE_*_URL herdada do shell.
        env: {
          VITE_API_URL: '',
          VITE_CATALOG_URL: '',
          VITE_ORDER_URL: '',
          VITE_AUTH_URL: '',
          VITE_STRIPE_PUBLISHABLE_KEY: '',
        },
      },
})
