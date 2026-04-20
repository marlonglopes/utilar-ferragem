import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

export default function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] gap-6 text-center px-4">
      <span className="text-7xl select-none">🔩</span>
      <div>
        <h1 className="font-display font-black text-3xl text-gray-900 mb-2">404</h1>
        <p className="text-gray-500">{t('notFound.message', 'Página não encontrada.')}</p>
      </div>
      <Link
        to="/"
        className="bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold px-5 py-2.5 rounded-lg transition-colors"
      >
        {t('notFound.cta', 'Voltar ao início')}
      </Link>
    </div>
  )
}
