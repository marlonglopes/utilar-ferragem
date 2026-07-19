import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'
import type { ReactNode } from 'react'


interface ProtectedRouteProps {
  children: ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const location = useLocation()
  const user = useAuthStore((s) => s.user)

  if (!user) {
    return <Navigate to={`/entrar?next=${encodeURIComponent(location.pathname)}`} replace />
  }

  // Admin e operador de balcão têm ÁREA PRÓPRIA dentro da Utilar — mandamos
  // para ela, em vez de deixá-los numa tela de cliente que não é a deles.
  //
  // Antes daqui, os dois eram EXPULSOS para um hub externo
  // (hub.utilar.com.br, que não existe) — resíduo da arquitetura do gifthy,
  // onde o admin morava em outro aplicativo. Entrar como administrador levava
  // a pessoa para fora do sistema, numa página que não carrega.
  if (user.role === 'admin') {
    return <Navigate to="/admin" replace />
  }
  if (user.role === 'store_operator') {
    return <Navigate to="/balcao" replace />
  }
  // `seller` (lojista do marketplace) ainda não tem área própria. Deixamos
  // navegar como cliente em vez de bloquear: ele compra na loja como qualquer
  // pessoa, e expulsá-lo para lugar nenhum seria pior.

  return <>{children}</>
}
