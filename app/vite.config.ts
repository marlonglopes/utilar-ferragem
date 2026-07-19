/// <reference types="vitest" />
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// Rotas estáticas indexáveis. Carrinho, checkout, conta, pedido e busca ficam
// de fora de propósito (ver public/robots.txt).
const STATIC_ROUTES = [
  { path: '/', changefreq: 'daily', priority: '1.0' },
  { path: '/categorias', changefreq: 'weekly', priority: '0.8' },
  { path: '/sobre', changefreq: 'monthly', priority: '0.5' },
  { path: '/contato', changefreq: 'monthly', priority: '0.5' },
  { path: '/ajuda', changefreq: 'monthly', priority: '0.6' },
  { path: '/vender', changefreq: 'monthly', priority: '0.7' },
  { path: '/privacidade', changefreq: 'yearly', priority: '0.3' },
  { path: '/termos', changefreq: 'yearly', priority: '0.3' },
]

// Espelha TOP_LEVEL_CATEGORIES de src/lib/taxonomy.ts. Duplicado aqui porque o
// vite.config roda em Node antes do bundle existir. Ao adicionar uma categoria
// nova na taxonomia, acrescente o slug aqui também.
const CATEGORY_SLUGS = [
  'ferramentas',
  'construcao',
  'eletrica',
  'hidraulica',
  'pintura',
  'jardim',
  'seguranca',
  'fixacao',
]

/**
 * Emite sitemap.xml no build com as rotas fixas + as de categoria.
 *
 * As URLs de produto (/produto/:slug) NÃO entram aqui: são milhares e mudam com
 * o estoque, então pertencem a um sitemap gerado no backend a partir do
 * catalog-service e referenciado por um sitemap index. Ver docs/seo-spa.md.
 */
function sitemapPlugin(siteUrl: string): Plugin {
  return {
    name: 'utilar-sitemap',
    apply: 'build',
    generateBundle() {
      const base = siteUrl.replace(/\/$/, '')
      const today = new Date().toISOString().slice(0, 10)

      const urls = [
        ...STATIC_ROUTES,
        ...CATEGORY_SLUGS.map((slug) => ({
          path: `/categoria/${slug}`,
          changefreq: 'daily',
          priority: '0.9',
        })),
      ]

      const body = urls
        .map(
          ({ path: p, changefreq, priority }) =>
            `  <url>\n` +
            `    <loc>${base}${p}</loc>\n` +
            `    <lastmod>${today}</lastmod>\n` +
            `    <changefreq>${changefreq}</changefreq>\n` +
            `    <priority>${priority}</priority>\n` +
            `  </url>`
        )
        .join('\n')

      this.emitFile({
        type: 'asset',
        fileName: 'sitemap.xml',
        source: `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${body}\n</urlset>\n`,
      })
    },
  }
}

export default defineConfig({
  plugins: [
    react(),
    sitemapPlugin(process.env.VITE_SITE_URL || 'https://www.utilarferragem.com.br'),
  ],
  build: {
    rollupOptions: {
      output: {
        // Separa as libs que quase nunca mudam do código do app, para que um
        // deploy de feature não invalide o cache do vendor no navegador.
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          'vendor-query': ['@tanstack/react-query'],
          'vendor-i18n': ['i18next', 'react-i18next'],
        },
      },
    },
  },
  server: {
    port: 5175,
    // Aceita o host do túnel. Sem isto o Vite recusa a requisição vinda do
    // ngrok com "Blocked request. This host is not allowed".
    allowedHosts: true,
    // ── Proxy para os 5 serviços ──────────────────────────────────────────
    //
    // PORQUÊ: o ngrok expõe UMA porta, e o SPA fala com 5 serviços em portas
    // diferentes. Sem proxy, quem abre o túnel de fora recebe o HTML e depois
    // vê o navegador tentar chamar 192.168.0.7:8091 — um IP da rede local, que
    // não existe para ele. A loja carrega vazia.
    //
    // Com o proxy, tudo passa pela mesma origem e um túnel só resolve. Também
    // elimina o CORS: mesma origem, sem preflight.
    //
    // Para usar: deixe as VITE_*_URL VAZIAS no .env.local. Aí o front chama
    // caminho relativo e cai aqui.
    proxy: {
      '/api/v1/products':   { target: 'http://localhost:8091', changeOrigin: true },
      '/api/v1/categories': { target: 'http://localhost:8091', changeOrigin: true },
      '/api/v1/sellers':    { target: 'http://localhost:8091', changeOrigin: true },
      '/api/v1/admin':      { target: 'http://localhost:8091', changeOrigin: true },
      '/api/v1/store':      { target: 'http://localhost:8091', changeOrigin: true },
      '/media':             { target: 'http://localhost:8091', changeOrigin: true },
      '/api/v1/auth':       { target: 'http://localhost:8093', changeOrigin: true },
      '/api/v1/orders':     { target: 'http://localhost:8092', changeOrigin: true },
      '/api/v1/shipping':   { target: 'http://localhost:8092', changeOrigin: true },
      '/api/v1/balcao':     { target: 'http://localhost:8092', changeOrigin: true },
      '/api/v1/payments':   { target: 'http://localhost:8090', changeOrigin: true },
      '/api/v1/ledger':     { target: 'http://localhost:8090', changeOrigin: true },
      '/api/v1/assistant':  { target: 'http://localhost:8094', changeOrigin: true },
    },
  },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  test: {
    environment: 'happy-dom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
    // e2e/ é do Playwright (API própria) — o vitest não deve tentar rodar esses specs.
    exclude: ['**/node_modules/**', '**/dist/**', '**/e2e/**'],
    // Force mock mode em testes — testes legacy (useOrders, OrdersTab, OrderDetailPage,
    // LoginPage, RegisterPage) assumem `is*Enabled === false`. Sem esse override,
    // valores presentes no .env.local vazariam pra dentro do test e quebrariam o
    // branch mock dos hooks.
    env: {
      VITE_API_URL: '',
      VITE_CATALOG_URL: '',
      VITE_ORDER_URL: '',
      VITE_AUTH_URL: '',
      VITE_ASSISTANT_URL: '',
      VITE_STRIPE_PUBLISHABLE_KEY: '',
      VITE_SITE_URL: 'https://www.utilarferragem.com.br',
    },
  },
})
