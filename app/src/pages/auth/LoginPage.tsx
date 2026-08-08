import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Mail, Lock, ShieldCheck } from 'lucide-react'
import type { User as AuthUser } from '@/store/authStore'
import { useAuthStore } from '@/store/authStore'
import { useCartStore } from '@/store/cartStore'
import { Input } from '@/components/ui'
import { authPost, isAuthEnabled } from '@/lib/api'

interface LoginResponse {
  accessToken?: string
  refreshToken?: string
  user?: {
    id: string
    email: string
    name: string
    role: AuthUser['role']
    emailVerified?: boolean
  }
  // Login em 2 passos: quando a conta tem MFA, o 1º passo não traz tokens — só
  // o desafio, que o 2º passo (código TOTP) troca pelos tokens.
  mfaRequired?: boolean
  challenge?: string
}

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
  // 2º passo do MFA: quando setado, mostramos o campo de código em vez de e-mail/senha.
  const [mfaChallenge, setMfaChallenge] = useState<string | null>(null)
  const [code, setCode] = useState('')

  // finishLogin grava a sessão e redireciona por papel. Compartilhado pelo login
  // direto e pelo 2º passo do MFA.
  function finishLogin(data: LoginResponse) {
    if (!data.user || !data.accessToken) return
    setUser({ ...data.user, token: data.accessToken, refreshToken: data.refreshToken })
    mergeCarts([])

    // Cada papel vai para a SUA área, dentro da própria Utilar. `next` explícito
    // na URL ganha: quem clicou em algo e caiu no login volta para onde queria.
    if (nextPath && nextPath !== '/') {
      navigate(nextPath, { replace: true })
      return
    }
    if (data.user.role === 'admin') {
      navigate('/admin', { replace: true })
      return
    }
    if (data.user.role === 'store_operator') {
      navigate('/balcao', { replace: true })
      return
    }
    navigate(nextPath, { replace: true })
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      if (!isAuthEnabled) {
        // Stub: accept any credentials em dev (sem auth-service).
        setUser({
          id: 'mock-1',
          email,
          name: email.split('@')[0],
          role: 'customer',
          token: 'mock-token',
        })
        mergeCarts([])
        navigate(nextPath, { replace: true })
        return
      }

      const data = await authPost<LoginResponse>('/api/v1/auth/login', { email, password })

      // Conta com MFA: o 1º passo só entrega o desafio. Troca para o campo de código.
      if (data.mfaRequired && data.challenge) {
        setMfaChallenge(data.challenge)
        return
      }
      finishLogin(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.invalidCredentials'))
    } finally {
      setLoading(false)
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const data = await authPost<LoginResponse>('/api/v1/auth/login/verify-totp', {
        challenge: mfaChallenge,
        code: code.trim(),
      })
      finishLogin(data)
    } catch {
      setError('Código inválido ou expirado. Tente de novo.')
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

        {/* 2º passo do MFA: senha OK, falta o código do autenticador. */}
        {mfaChallenge ? (
          <form onSubmit={handleVerify} className="flex flex-col gap-4">
            <div className="flex flex-col items-center gap-2 text-center">
              <ShieldCheck className="h-8 w-8 text-brand-orange" aria-hidden="true" />
              <p className="text-sm text-gray-600">
                Digite o código de 6 dígitos do seu app autenticador.
              </p>
            </div>
            <Input
              label="Código de verificação"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
              className="text-center text-lg tracking-widest"
            />
            {error && (
              <p className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded-lg">{error}</p>
            )}
            <button
              type="submit"
              disabled={loading || code.length !== 6}
              className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-gray-900 font-semibold text-sm transition-colors disabled:opacity-60"
            >
              {loading ? t('loading') : 'Verificar'}
            </button>
            <button
              type="button"
              onClick={() => {
                setMfaChallenge(null)
                setCode('')
                setError('')
              }}
              className="text-sm text-gray-500 hover:underline"
            >
              Voltar
            </button>
          </form>
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
              className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-gray-900 font-semibold text-sm transition-colors disabled:opacity-60"
            >
              {loading ? t('loading') : t('auth.login')}
            </button>
          </form>
        )}

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
