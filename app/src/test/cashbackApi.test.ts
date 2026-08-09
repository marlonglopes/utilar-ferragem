import { describe, it, expect } from 'vitest'
import { fetchMyCashback, fetchCashbackConfig } from '@/lib/cashbackApi'

// Sem backend (VITE_ORDER_URL vazio nos testes), a lib cai no mock — o que
// mantém a tela de cashback demonstrável offline.
describe('cashbackApi (mock)', () => {
  it('fetchMyCashback devolve saldo e extrato do mock', async () => {
    const info = await fetchMyCashback(null)
    expect(info.active).toBe(true)
    expect(info.balance).toBeGreaterThan(0)
    expect(info.earnRatePct).toBe(5)
    expect(info.history.length).toBeGreaterThan(0)
    // O extrato tem entradas assinadas (ganho +, resgate −).
    expect(info.history.some((e) => e.kind === 'earn' && e.amount > 0)).toBe(true)
    expect(info.history.some((e) => e.kind === 'redeem' && e.amount < 0)).toBe(true)
  })

  it('fetchCashbackConfig devolve a política padrão no mock', async () => {
    const cfg = await fetchCashbackConfig()
    expect(cfg.earnRatePct).toBe(5)
    expect(cfg.redeemMaxPct).toBe(50)
    expect(cfg.validityDays).toBe(90)
  })
})
