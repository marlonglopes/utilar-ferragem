import { describe, it, expect } from 'vitest'
import { validateCPF } from '@/lib/cpf'

// Regressão do boleto (2026-08-28): o checkout validava só "11 dígitos", então
// "12345678901" (11 dígitos, dígito verificador errado) passava no cliente e só
// a Stripe recusava com tax_id_invalid — o comprador via "payment gateway error"
// genérico. validateCPF checa o dígito verificador; o checkout agora depende dele.
describe('validateCPF', () => {
  it('rejeita o CPF sequencial falso 12345678901 (o que dava o bug)', () => {
    expect(validateCPF('12345678901')).toBe(false)
  })

  it('aceita CPFs válidos (com e sem máscara)', () => {
    expect(validateCPF('11144477735')).toBe(true)
    expect(validateCPF('111.444.777-35')).toBe(true)
  })

  it('rejeita dígitos repetidos, tamanho errado e vazio', () => {
    expect(validateCPF('11111111111')).toBe(false)
    expect(validateCPF('123')).toBe(false)
    expect(validateCPF('')).toBe(false)
  })
})
