import { describe, it, expect } from 'vitest'
import { validateCPF } from '@/lib/cpf'

describe('validateCPF', () => {
  it('accepts known valid CPFs', () => {
    expect(validateCPF('529.982.247-25')).toBe(true)
    expect(validateCPF('52998224725')).toBe(true)
    expect(validateCPF('111.444.777-35')).toBe(true)
  })

  it('rejects all-same-digit CPFs', () => {
    for (let d = 0; d <= 9; d++) {
      expect(validateCPF(String(d).repeat(11))).toBe(false)
    }
  })

  it('rejects CPFs with wrong check digits', () => {
    expect(validateCPF('529.982.247-00')).toBe(false)
    expect(validateCPF('111.444.777-00')).toBe(false)
  })

  it('rejects CPFs with wrong length', () => {
    expect(validateCPF('123')).toBe(false)
    expect(validateCPF('')).toBe(false)
    expect(validateCPF('123456789012')).toBe(false)
  })

  it('ignores formatting characters', () => {
    expect(validateCPF('529.982.247-25')).toBe(true)
    expect(validateCPF('529982247-25')).toBe(true)
  })
})
