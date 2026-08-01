import { describe, it, expect } from 'vitest'
import { fetchPaymentConfig, PROVIDER_LABEL, METHOD_LABEL } from '@/lib/adminPaymentApi'

// VITE_API_URL vazio → mock. A config é read-only e nunca traz segredo.
describe('adminPaymentApi (mock)', () => {
  it('traz provider, métodos e saúde — sem segredo', async () => {
    const cfg = await fetchPaymentConfig()
    expect(cfg.provider).toBeTruthy()
    expect(cfg.methods.length).toBeGreaterThan(0)
    expect(typeof cfg.healthy).toBe('boolean')
    // Estruturalmente não há campo de segredo no contrato.
    expect(JSON.stringify(cfg).toLowerCase()).not.toContain('secret')
    expect(JSON.stringify(cfg).toLowerCase()).not.toContain('key')
  })

  it('rótulos amigáveis de provider e método', () => {
    expect(PROVIDER_LABEL['appmax-v1']).toBe('Appmax (AppStore v1)')
    expect(METHOD_LABEL.pix).toBe('Pix')
    expect(METHOD_LABEL.credit_card).toBe('Cartão de crédito')
  })
})
