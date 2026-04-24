import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Mail, Lock, User, Phone } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { Input } from '@/components/ui'
import { authPost, isAuthEnabled } from '@/lib/api'
import { validateCPF } from '@/lib/cpf'
import { formatCPF, formatPhone } from '@/lib/format'

interface RegisterResponse {
  accessToken: string
  refreshToken: string
  user: { id: string; email: string; name: string; role: 'customer' | 'seller' | 'admin'; emailVerified?: boolean }
}

export default function RegisterPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const setUser = useAuthStore((s) => s.setUser)

  const [form, setForm] = useState({ name: '', email: '', cpf: '', phone: '', password: '', passwordConfirm: '' })
  const [lgpd, setLgpd] = useState(false)
  const [errors, setErrors] = useState<Partial<typeof form> & { lgpd?: string; general?: string }>({})
  const [loading, setLoading] = useState(false)

  function set(field: keyof typeof form, value: string) {
    setForm((prev) => ({ ...prev, [field]: value }))
    setErrors((prev) => ({ ...prev, [field]: undefined }))
  }

  function validate() {
    const e: typeof errors = {}
    if (!form.name.trim()) e.name = 'Campo obrigatório.'
    if (!form.email.trim()) e.email = 'Campo obrigatório.'
    if (!validateCPF(form.cpf)) e.cpf = t('auth.cpfInvalid')
    if (form.password.length < 10) e.password = t('auth.passwordMinLength')
    if (form.password !== form.passwordConfirm) e.passwordConfirm = t('auth.passwordMismatch')
    if (!lgpd) e.lgpd = t('auth.lgpdRequired')
    return e
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const errs = validate()
    if (Object.keys(errs).length > 0) { setErrors(errs); return }
    setLoading(true)

    try {
      if (!isAuthEnabled) {
        setUser({ id: 'mock-1', email: form.email, name: form.name, role: 'customer', token: 'mock-token' })
        navigate('/', { replace: true })
        return
      }

      const data = await authPost<RegisterResponse>('/api/v1/auth/register', {
        name: form.name,
        email: form.email,
        cpf: form.cpf.replace(/\D/g, ''),
        phone: form.phone.replace(/\D/g, ''),
        password: form.password,
      })
      setUser({ ...data.user, token: data.accessToken, refreshToken: data.refreshToken })
      navigate('/', { replace: true })
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('error')
      if (msg.toLowerCase().includes('email')) {
        setErrors({ email: t('auth.emailInUse') })
      } else {
        setErrors({ general: msg })
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-[calc(100vh-8rem)] flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="font-display font-black text-2xl text-gray-900">{t('auth.registerTitle')}</h1>
          <p className="text-sm text-gray-500 mt-1">{t('auth.registerSubtitle')}</p>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            label={t('auth.name')}
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
            autoComplete="name"
            required
            error={errors.name}
            leftIcon={<User className="h-4 w-4" />}
          />
          <Input
            type="email"
            label={t('auth.email')}
            value={form.email}
            onChange={(e) => set('email', e.target.value)}
            autoComplete="email"
            required
            error={errors.email}
            leftIcon={<Mail className="h-4 w-4" />}
          />
          <Input
            label={t('auth.cpf')}
            value={form.cpf}
            onChange={(e) => set('cpf', formatCPF(e.target.value))}
            inputMode="numeric"
            placeholder="000.000.000-00"
            maxLength={14}
            error={errors.cpf}
          />
          <Input
            label={t('auth.phone')}
            value={form.phone}
            onChange={(e) => set('phone', formatPhone(e.target.value))}
            inputMode="tel"
            autoComplete="tel"
            placeholder="(11) 99999-9999"
            maxLength={15}
            leftIcon={<Phone className="h-4 w-4" />}
          />
          <Input
            type="password"
            label={t('auth.password')}
            value={form.password}
            onChange={(e) => set('password', e.target.value)}
            autoComplete="new-password"
            required
            error={errors.password}
            hint={t('auth.passwordMinLength')}
            leftIcon={<Lock className="h-4 w-4" />}
          />
          <Input
            type="password"
            label={t('auth.passwordConfirm')}
            value={form.passwordConfirm}
            onChange={(e) => set('passwordConfirm', e.target.value)}
            autoComplete="new-password"
            required
            error={errors.passwordConfirm}
            leftIcon={<Lock className="h-4 w-4" />}
          />

          <label className="flex items-start gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={lgpd}
              onChange={(e) => { setLgpd(e.target.checked); setErrors((p) => ({ ...p, lgpd: undefined })) }}
              className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-orange focus:ring-brand-orange"
            />
            <span className="text-xs text-gray-600 leading-relaxed">{t('auth.lgpdConsent')}</span>
          </label>
          {errors.lgpd && <p className="text-xs text-red-600">{errors.lgpd}</p>}

          {errors.general && (
            <p className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded-lg">{errors.general}</p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="h-11 rounded-xl bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold text-sm transition-colors disabled:opacity-60"
          >
            {loading ? t('loading') : t('auth.register')}
          </button>
        </form>

        <p className="text-center text-sm text-gray-500 mt-6">
          {t('auth.hasAccount')}{' '}
          <Link to="/entrar" className="text-brand-orange font-semibold hover:underline">
            {t('auth.login')}
          </Link>
        </p>
      </div>
    </div>
  )
}
