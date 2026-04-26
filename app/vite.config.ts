/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  server: { port: 5175 },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  test: {
    environment: 'happy-dom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
    // Force mock mode em testes — testes legacy (useOrders, OrdersTab, OrderDetailPage,
    // LoginPage, RegisterPage) assumem `is*Enabled === false`. Sem esse override,
    // valores presentes no .env.local vazariam pra dentro do test e quebrariam o
    // branch mock dos hooks.
    env: {
      VITE_API_URL: '',
      VITE_CATALOG_URL: '',
      VITE_ORDER_URL: '',
      VITE_AUTH_URL: '',
      VITE_STRIPE_PUBLISHABLE_KEY: '',
    },
  },
})
