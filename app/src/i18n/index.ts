import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import ptBRCommon from './pt-BR/common.json'
import ptBRCatalog from './pt-BR/catalog.json'
import ptBRCheckout from './pt-BR/checkout.json'
import ptBRAccount from './pt-BR/account.json'
import enCommon from './en/common.json'
import enCatalog from './en/catalog.json'
import enCheckout from './en/checkout.json'
import enAccount from './en/account.json'

const storedLocale = typeof window !== 'undefined'
  ? (() => {
      try {
        const raw = localStorage.getItem('utilar-locale-v2')
        return raw ? (JSON.parse(raw) as { state?: { locale?: string } }).state?.locale : null
      } catch { return null }
    })()
  : null
const initialLng = storedLocale === 'en' ? 'en' : 'pt-BR'

i18n
  .use(initReactI18next)
  .init({
    lng: initialLng,
    fallbackLng: 'pt-BR',
    defaultNS: 'common',
    ns: ['common', 'catalog', 'checkout', 'account'],
    resources: {
      'pt-BR': { common: ptBRCommon, catalog: ptBRCatalog, checkout: ptBRCheckout, account: ptBRAccount },
      en: { common: enCommon, catalog: enCatalog, checkout: enCheckout, account: enAccount },
    },
    interpolation: { escapeValue: false },
  })

export default i18n
