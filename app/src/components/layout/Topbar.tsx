import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

export function Topbar() {
  const { t } = useTranslation()

  return (
    <div className="bg-brand-blue text-white text-xs">
      <div className="container flex items-center justify-between h-8 gap-4">
        <span className="truncate">
          {t('topbar.promo')}{' '}
          <strong className="text-brand-gold">{t('topbar.cashback')}</strong>
        </span>
        <div className="hidden sm:flex items-center gap-3 flex-shrink-0">
          <Link to="/vender" className="text-blue-200 hover:text-white transition-colors">
            {t('topbar.becomeSeller')}
          </Link>
          <span className="text-white/30">·</span>
          <Link to="/ajuda" className="text-blue-200 hover:text-white transition-colors">
            {t('topbar.help')}
          </Link>
        </div>
      </div>
    </div>
  )
}
