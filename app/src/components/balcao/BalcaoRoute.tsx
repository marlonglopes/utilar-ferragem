import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { isAuthEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import { ShieldAlert } from 'lucide-react'

/**
 * Guard de acesso ao PDV de balcão.
 *
 * Existe porque `components/auth/ProtectedRoute.tsx` faz o oposto do que o
 * balcão precisa: ele expulsa qualquer usuário que não seja `customer` para uma
 * URL externa (o hub). Um operador de loja logado seria chutado para fora do
 * próprio PDV. Aquele arquivo é de outro dono, então o balcão traz o seu.
 *
 * Papéis liberados espelham o `RequireRole("store_operator", "admin")` do
 * auth-service em `/api/v1/store/*`:
 *
 *   store_operator → o vendedor no caixa, com vínculo a uma loja física.
 *   admin          → entra junto, para suporte.
 *
 * `seller` continua FORA e isso é intencional: no auth-service `seller` é o
 * lojista do marketplace (quem anuncia no site), não o vendedor de balcão.
 * Liberar `seller` aqui daria o PDV a todo lojista do marketplace.
 *
 * Em dev/mock o acesso é liberado, para o PDV ser demonstrável sem backend.
 */

const BALCAO_ROLES_ALLOWED = ['store_operator', 'admin'] as const

/** Sem auth-service configurado (modo mock) ou build de dev → libera. */
function isDevBypass(): boolean {
  return !isAuthEnabled || import.meta.env.DEV
}

interface BalcaoRouteProps {
  children: ReactNode
}

export function BalcaoRoute({ children }: BalcaoRouteProps) {
  const location = useLocation()
  const user = useAuthStore((s) => s.user)

  if (isDevBypass()) {
    return <>{children}</>
  }

  if (!user) {
    return <Navigate to={`/entrar?next=${encodeURIComponent(location.pathname)}`} replace />
  }

  const allowed = (BALCAO_ROLES_ALLOWED as readonly string[]).includes(user.role)
  if (!allowed) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 p-6">
        <div className="max-w-md text-center">
          <ShieldAlert className="mx-auto h-12 w-12 text-brand-orange" aria-hidden="true" />
          <h1 className="mt-4 font-display text-2xl font-bold text-gray-900">
            Acesso restrito ao balcão
          </h1>
          <p className="mt-2 text-gray-600">
            Sua conta não tem permissão de operador de loja. Fale com o gerente para liberar o
            acesso ao PDV.
          </p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}
