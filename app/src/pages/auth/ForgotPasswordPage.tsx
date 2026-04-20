import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Mail } from 'lucide-react'
import { Input } from '@/components/ui'
import { apiPost, isApiEnabled } from '@/lib/api'

export default function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      if (isApiEnabled) {
        await apiPost('/auth/forgot-password', { email })
      }
      // Always show success to avoid email enumeration
      setSent(true)
    } catch {
      setSent(true)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-[calc(100vh-8rem)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display font-black text-2xl text-gray-900">{t('auth.forgotPasswordTitle')}</h1>
          <p className="text-sm text-gray-500 mt-1">{t('auth.forgotPasswordHint')}</p>
        </div>

        {sent ? (
          <div className="text-center flex flex-col gap-4">
            <p className="text-sm text-gray-700 bg-green-50 border border-green-200 px-4 py-3 rounded-xl">
              {t('auth.forgotPasswordSent')}
            </p>
            <Link to="/entrar" className="text-sm text-brand-orange font-semibold hover:underline">
              {t('auth.login')}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <Input
              type="email"
              label={t('auth.email')}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="email"
              required
              leftIcon={<Mail className="h-4 w-4" />}
            />
            {error && <p className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded-lg">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm transition-colors disabled:opacity-60"
            >
              {loading ? t('auth.sendingLink') : t('auth.forgotPasswordSubmit')}
            </button>
            <Link to="/entrar" className="text-center text-sm text-gray-500 hover:underline">
              {t('back')}
            </Link>
          </form>
        )}
      </div>
    </div>
  )
}
