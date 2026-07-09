import { describe, it, expect, afterEach } from 'vitest'
import { useLocaleStore, SUPPORTED_LOCALES } from '@/store/localeStore'

describe('localeStore', () => {
  afterEach(() => {
    useLocaleStore.getState().setLocale('pt-BR')
  })

  it('suporta pt-BR e en', () => {
    expect(SUPPORTED_LOCALES).toEqual(['pt-BR', 'en'])
  })

  it('setLocale atualiza o estado e o lang do documento', () => {
    useLocaleStore.getState().setLocale('en')
    expect(useLocaleStore.getState().locale).toBe('en')
    expect(document.documentElement.lang).toBe('en')

    useLocaleStore.getState().setLocale('pt-BR')
    expect(useLocaleStore.getState().locale).toBe('pt-BR')
    expect(document.documentElement.lang).toBe('pt-BR')
  })
})
