import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Mail, Lock } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { useCartStore } from '@/store/cartStore'
import { Input } from '@/components/ui'
import { authPost, isAuthEnabled } from '@/lib/api'

interface LoginResponse {
  accessToken: string
  refreshToken: string
  user: { id: string; email: string; name: string; role: 'customer' | 'seller' | 'admin'; emailVerified?: boolean }
}

const GIFTHY_HUB_URL = import.meta.env.VITE_GIFTHY_HUB_URL ?? 'https://hub.utilar.com.br'

export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const nextPath = searchParams.get('next') ?? '/'

  const setUser = useAuthStore((s) => s.setUser)
  const mergeCarts = useCartStore((s) => s.mergeCarts)

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      if (!isAuthEnabled) {
        // Stub: accept any credentials em dev (sem auth-service).
        setUser({ id: 'mock-1', email, name: email.split('@')[0], role: 'customer', token: 'mock-token' })
        mergeCarts([])
        navigate(nextPath, { replace: true })
        return
      }

      const data = await authPost<LoginResponse>('/api/v1/auth/login', { email, password })

      if (data.user.role === 'seller' || data.user.role === 'admin') {
        window.location.href = `${GIFTHY_HUB_URL}?from=utilar-customer`
        return
      }

      setUser({ ...data.user, token: data.accessToken, refreshToken: data.refreshToken })
      mergeCarts([])
      navigate(nextPath, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.invalidCredentials'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-[calc(100vh-8rem)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display font-black text-2xl text-gray-900">{t('auth.loginTitle')}</h1>
          <p className="text-sm text-gray-500 mt-1">{t('auth.loginSubtitle')}</p>
        </div>

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
          <Input
            type="password"
            label={t('auth.password')}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
            leftIcon={<Lock className="h-4 w-4" />}
          />

          {error && (
            <p className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded-lg">{error}</p>
          )}

          <div className="flex justify-end">
            <Link to="/esqueci-senha" className="text-sm text-brand-orange hover:underline">
              {t('auth.forgotPassword')}
            </Link>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm transition-colors disabled:opacity-60"
          >
            {loading ? t('loading') : t('auth.login')}
          </button>
        </form>

        <p className="text-center text-sm text-gray-500 mt-6">
          {t('auth.noAccount')}{' '}
          <Link to="/cadastro" className="text-brand-orange font-semibold hover:underline">
            {t('auth.register')}
          </Link>
        </p>
      </div>
    </div>
  )
}
