import { describe, it, expect, vi, afterEach } from 'vitest'
import { isValidCardNumber } from '@/lib/appmaxCard'

/**
 * O seam de tokenização Appmax é DORMENTE (sem VITE_APPMAX_PUBLIC_KEY) e
 * FAIL-CLOSED (com a chave, mas sem o SDK do contrato ainda). Cobrimos os dois
 * estados + a validação Luhn local. O decode/tokenização REAL só existe com o
 * SDK do contrato Appmax — igual ao bloqueio do checkout web.
 */

describe('isValidCardNumber (Luhn)', () => {
  it('aceita números válidos (test cards)', () => {
    expect(isValidCardNumber('4242 4242 4242 4242')).toBe(true) // Stripe test
    expect(isValidCardNumber('4000000000000010')).toBe(true) // Appmax sandbox aprova
  })
  it('recusa número inválido, curto ou longo demais', () => {
    expect(isValidCardNumber('4242 4242 4242 4241')).toBe(false) // dígito trocado
    expect(isValidCardNumber('1234')).toBe(false)
    expect(isValidCardNumber('42424242424242424242')).toBe(false) // 20 dígitos
  })
})

describe('tokenizeCard — seam Appmax', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  const fields = {
    number: '4000000000000010',
    holderName: 'MARIA SOUZA',
    expMonth: '12',
    expYear: '30',
    cvv: '123',
  }

  it('DORMENTE: sem VITE_APPMAX_PUBLIC_KEY, isAppmaxCardEnabled=false e tokenizeCard recusa claramente', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_APPMAX_PUBLIC_KEY', '')
    const mod = await import('@/lib/appmaxCard')
    expect(mod.isAppmaxCardEnabled).toBe(false)
    // instanceof pela classe do MESMO módulo re-importado (resetModules cria outra).
    await expect(mod.tokenizeCard(fields)).rejects.toBeInstanceOf(mod.AppmaxTokenizationError)
    await expect(mod.tokenizeCard(fields)).rejects.toThrow(/não configurada/i)
  })

  it('FAIL-CLOSED: com a chave mas sem o SDK do contrato, recusa (nunca forja token nem manda PAN)', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_APPMAX_PUBLIC_KEY', 'pk_appmax_demo')
    const mod = await import('@/lib/appmaxCard')
    expect(mod.isAppmaxCardEnabled).toBe(true)
    // Número válido → passa da validação Luhn e cai no fail-closed do SDK pendente.
    await expect(mod.tokenizeCard(fields)).rejects.toThrow(/pendente/i)
  })

  it('com a chave, número inválido é barrado antes de qualquer chamada', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_APPMAX_PUBLIC_KEY', 'pk_appmax_demo')
    const mod = await import('@/lib/appmaxCard')
    await expect(mod.tokenizeCard({ ...fields, number: '1234 5678' })).rejects.toThrow(/inválido/i)
  })
})
