import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'

import ptBRCommon from './pt-BR/common.json'
import ptBRCatalog from './pt-BR/catalog.json'
import ptBRCheckout from './pt-BR/checkout.json'
import ptBRAccount from './pt-BR/account.json'
import enCommon from './en/common.json'
import enCatalog from './en/catalog.json'
import enCheckout from './en/checkout.json'
import enAccount from './en/account.json'

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'pt-BR',
    defaultNS: 'common',
    ns: ['common', 'catalog', 'checkout', 'account'],
    resources: {
      'pt-BR': { common: ptBRCommon, catalog: ptBRCatalog, checkout: ptBRCheckout, account: ptBRAccount },
      en: { common: enCommon, catalog: enCatalog, checkout: enCheckout, account: enAccount },
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      lookupLocalStorage: 'utilar-locale',
    },
    interpolation: { escapeValue: false },
  })

export default i18n
