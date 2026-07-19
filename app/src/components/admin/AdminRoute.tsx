import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ShieldAlert } from 'lucide-react'
import { isAuthEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

/**
 * Guard de acesso ao painel administrativo. Segue o padrão de
 * `components/balcao/BalcaoRoute.tsx` — e pelo mesmo motivo:
 * `components/auth/ProtectedRoute.tsx` expulsa todo usuário que não seja
 * `customer`, ou seja, chutaria o próprio dono para fora do painel dele.
 * Aquele arquivo é de outro dono; o admin traz o seu.
 *
 * ⚠️ ESTE COMPONENTE NÃO É UMA FRONTEIRA DE SEGURANÇA.
 *
 * Ele decide o que *renderizar*, não o que o servidor *entrega*. Qualquer
 * pessoa consegue editar o `utilar-auth` do localStorage e colocar
 * `role: "admin"` — e o painel abriria. O que impede o vazamento é o
 * payment/order/auth-service recusarem as rotas `/api/v1/admin/*` para um JWT
 * sem papel `admin` (o papel vem da claim assinada, nunca do corpo da request).
 * Ver `docs/admin-dashboard-api.md` § Autorização.
 *
 * Esconder o botão não protege nada. Este guard existe para não mostrar uma
 * tela cheia de erro 403 para quem não deveria estar ali — só isso.
 */

const ADMIN_ROLES_ALLOWED = ['admin'] as const

/**
 * Sem auth-service configurado (modo mock) ou build de dev → libera, para o
 * painel ser demonstrável sem backend. Mesma decisão do balcão.
 * Em produção com auth real, `isAuthEnabled` é true e `DEV` é false, então
 * este bypass não existe no bundle que vai pro ar.
 */
function isDevBypass(): boolean {
  return !isAuthEnabled || import.meta.env.DEV
}

interface AdminRouteProps {
  children: ReactNode
}

export function AdminRoute({ children }: AdminRouteProps) {
  const location = useLocation()
  const user = useAuthStore((s) => s.user)

  if (isDevBypass()) {
    return <>{children}</>
  }

  if (!user) {
    return <Navigate to={`/entrar?next=${encodeURIComponent(location.pathname)}`} replace />
  }

  const allowed = (ADMIN_ROLES_ALLOWED as readonly string[]).includes(user.role)
  if (!allowed) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50 p-6">
        <div className="max-w-md text-center">
          <ShieldAlert className="mx-auto h-12 w-12 text-brand-orange" aria-hidden="true" />
          <h1 className="mt-4 font-display text-2xl font-bold text-gray-900">
            Acesso restrito ao painel
          </h1>
          <p className="mt-2 text-gray-600">
            Sua conta não tem permissão de administrador. Fale com o responsável pela Utilar para
            liberar o acesso.
          </p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}

export default AdminRoute
