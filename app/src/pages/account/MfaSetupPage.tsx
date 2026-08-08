import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { QRCodeSVG } from 'qrcode.react'
import { ShieldCheck, ShieldAlert, Loader2 } from 'lucide-react'
import { Input } from '@/components/ui'
import { useAuthStore } from '@/store/authStore'
import { authGet, authPost, isAuthEnabled } from '@/lib/api'

/**
 * Ativação do 2º fator (TOTP) da conta. Fica FORA do ProtectedRoute de propósito:
 * quem mais precisa de MFA é o admin/operador, e o ProtectedRoute só deixa
 * `customer` entrar. Aqui a barreira é só estar logado.
 *
 * Fluxo: status → (não ativo) enroll (mostra QR + segredo) → confirmar código →
 * ativo. O segredo/QR vêm do backend; o app do usuário lê o QR e gera o código.
 */
type Phase = 'loading' | 'enabled' | 'idle' | 'enrolling' | 'error-load'

interface EnrollData {
  secret: string
  otpauthUri: string
}

export default function MfaSetupPage() {
  const user = useAuthStore((s) => s.user)
  const token = user?.token ?? null

  const [phase, setPhase] = useState<Phase>('loading')
  const [enroll, setEnroll] = useState<EnrollData | null>(null)
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token || !isAuthEnabled) {
      setPhase('idle')
      return
    }
    let alive = true
    authGet<{ mfaEnabled: boolean }>('/api/v1/auth/mfa/status', token)
      .then((r) => alive && setPhase(r.mfaEnabled ? 'enabled' : 'idle'))
      .catch(() => alive && setPhase('error-load'))
    return () => {
      alive = false
    }
  }, [token])

  if (!user) return <Navigate to="/entrar?next=/seguranca" replace />

  async function startEnroll() {
    setError('')
    setBusy(true)
    try {
      const data = await authPost<EnrollData>('/api/v1/auth/mfa/enroll', {}, token ?? undefined)
      setEnroll(data)
      setPhase('enrolling')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Não foi possível iniciar a ativação.')
    } finally {
      setBusy(false)
    }
  }

  async function confirm(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await authPost('/api/v1/auth/mfa/confirm', { code: code.trim() }, token ?? undefined)
      setPhase('enabled')
      setEnroll(null)
      setCode('')
    } catch {
      setError('Código inválido. Confira o app e tente de novo.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-md px-4 py-10">
      <h1 className="font-display text-2xl font-bold text-gray-900">Segurança da conta</h1>
      <p className="mt-1 text-sm text-gray-600">
        Verificação em duas etapas (2FA) com um app autenticador (Google Authenticator, Authy,
        1Password…).
      </p>

      <div className="mt-6 rounded-xl border border-gray-200 bg-white p-5">
        {phase === 'loading' && (
          <p className="flex items-center gap-2 text-sm text-gray-500">
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" /> Carregando…
          </p>
        )}

        {phase === 'error-load' && (
          <p className="flex items-center gap-2 text-sm text-red-600">
            <ShieldAlert className="h-4 w-4" aria-hidden="true" /> Não foi possível verificar o
            status do 2FA.
          </p>
        )}

        {phase === 'enabled' && (
          <p className="flex items-center gap-2 font-semibold text-emerald-700">
            <ShieldCheck className="h-5 w-5" aria-hidden="true" /> 2FA ativo nesta conta.
          </p>
        )}

        {phase === 'idle' && (
          <div className="flex flex-col gap-3">
            <p className="text-sm text-gray-700">
              O 2FA ainda não está ativo. Ative para exigir um código do app a cada login —
              recomendado para contas de administração.
            </p>
            {!isAuthEnabled && (
              <p className="text-xs text-amber-700">
                Modo demonstração (sem backend de autenticação): a ativação real acontece com os
                serviços no ar.
              </p>
            )}
            {error && <p className="text-sm font-semibold text-red-600">{error}</p>}
            <button
              type="button"
              onClick={startEnroll}
              disabled={busy || !isAuthEnabled}
              className="h-11 rounded-xl bg-brand-orange px-4 font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-60"
            >
              {busy ? 'Gerando…' : 'Ativar 2FA'}
            </button>
          </div>
        )}

        {phase === 'enrolling' && enroll && (
          <form onSubmit={confirm} className="flex flex-col items-center gap-4">
            <p className="text-sm text-gray-700">
              Escaneie o QR no seu app autenticador e digite o código de 6 dígitos para confirmar.
            </p>
            <div className="rounded-lg border border-gray-200 bg-white p-3">
              <QRCodeSVG value={enroll.otpauthUri} size={176} aria-label="QR Code do 2FA" />
            </div>
            <p className="text-center text-xs text-gray-500">
              Não consegue escanear? Digite este código no app:
              <br />
              <span className="mt-1 inline-block break-all font-mono text-gray-700">
                {enroll.secret}
              </span>
            </p>
            <Input
              label="Código de confirmação"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              inputMode="numeric"
              autoComplete="one-time-code"
              className="w-full text-center text-lg tracking-widest"
            />
            {error && <p className="text-sm font-semibold text-red-600">{error}</p>}
            <button
              type="submit"
              disabled={busy || code.length !== 6}
              className="h-11 w-full rounded-xl bg-brand-orange font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-60"
            >
              {busy ? 'Confirmando…' : 'Confirmar e ativar'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
