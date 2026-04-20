import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

import ptBRCommon from './pt-BR/common.json'
import enCommon from './en/common.json'

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'pt-BR',
    defaultNS: 'common',
    resources: {
      'pt-BR': { common: ptBRCommon },
      en: { common: enCommon },
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupLocalStorage: 'utilar-locale',
    },
    interpolation: { escapeValue: false },
  })

export default i18n
