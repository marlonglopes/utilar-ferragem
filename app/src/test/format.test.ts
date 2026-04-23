import { describe, it, expect } from 'vitest'
import { formatCPF, formatCNPJ, formatCEP, formatPhone, formatCurrency } from '@/lib/format'

describe('formatCPF', () => {
  it('formats partial input progressively', () => {
    expect(formatCPF('123')).toBe('123')
    expect(formatCPF('123456')).toBe('123.456')
    expect(formatCPF('123456789')).toBe('123.456.789')
    expect(formatCPF('12345678901')).toBe('123.456.789-01')
  })

  it('strips non-digits', () => {
    expect(formatCPF('abc123def456ghi789jk01')).toBe('123.456.789-01')
  })

  it('truncates at 11 digits', () => {
    expect(formatCPF('123456789012345')).toBe('123.456.789-01')
  })
})

describe('formatCNPJ', () => {
  it('formats a full CNPJ', () => {
    expect(formatCNPJ('11222333000181')).toBe('11.222.333/0001-81')
  })

  it('formats partial input', () => {
    expect(formatCNPJ('11')).toBe('11')
    expect(formatCNPJ('11222')).toBe('11.222')
  })
})

describe('formatCEP', () => {
  it('formats an 8-digit CEP', () => {
    expect(formatCEP('01310100')).toBe('01310-100')
  })

  it('handles partial input', () => {
    expect(formatCEP('01310')).toBe('01310')
    expect(formatCEP('013')).toBe('013')
  })
})

describe('formatPhone', () => {
  it('formats a mobile number (11 digits)', () => {
    expect(formatPhone('11999998888')).toBe('(11) 99999-8888')
  })

  it('formats a landline (10 digits)', () => {
    expect(formatPhone('1133334444')).toBe('(11) 3333-4444')
  })

  it('handles partial input', () => {
    expect(formatPhone('11')).toBe('(11')
    expect(formatPhone('1')).toBe('(1')
  })
})

describe('formatCurrency', () => {
  it('formats BRL correctly', () => {
    const result = formatCurrency(1234.56, 'BRL', 'pt-BR')
    expect(result).toContain('1.234,56')
    expect(result).toContain('R$')
  })

  it('formats zero', () => {
    const result = formatCurrency(0, 'BRL', 'pt-BR')
    expect(result).toContain('0,00')
  })
})
