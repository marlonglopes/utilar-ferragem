import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import i18n from '@/i18n'

export type Locale = 'pt-BR' | 'en'

export const SUPPORTED_LOCALES: Locale[] = ['pt-BR', 'en']

interface LocaleState {
  locale: Locale
  setLocale: (locale: Locale) => void
}

function detectInitialLocale(): Locale {
  if (typeof window === 'undefined') return 'pt-BR'
  try {
    const raw = localStorage.getItem('utilar-locale-v2')
    if (!raw) return 'pt-BR'
    const stored = (JSON.parse(raw) as { state?: { locale?: string } }).state?.locale
    return stored === 'en' ? 'en' : 'pt-BR'
  } catch { return 'pt-BR' }
}

export const useLocaleStore = create<LocaleState>()(
  persist(
    (set) => ({
      locale: detectInitialLocale(),
      setLocale: (locale) => {
        set({ locale })
        i18n.changeLanguage(locale)
        if (typeof document !== 'undefined') {
          document.documentElement.lang = locale
        }
      },
    }),
    {
      name: 'utilar-locale-v2',
      onRehydrateStorage: () => (state) => {
        if (state?.locale) {
          i18n.changeLanguage(state.locale)
          if (typeof document !== 'undefined') {
            document.documentElement.lang = state.locale
          }
        }
      },
    }
  )
)

export function getCurrentLocale(): Locale {
  return useLocaleStore.getState().locale
}
