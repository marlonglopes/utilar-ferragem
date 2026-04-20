import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/store/authStore'
import type { ReactNode } from 'react'

const GIFTHY_HUB_URL = import.meta.env.VITE_GIFTHY_HUB_URL ?? 'https://hub.utilar.com.br'

interface ProtectedRouteProps {
  children: ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const location = useLocation()
  const user = useAuthStore((s) => s.user)

  if (!user) {
    return <Navigate to={`/entrar?next=${encodeURIComponent(location.pathname)}`} replace />
  }

  if (user.role === 'seller' || user.role === 'admin') {
    window.location.href = `${GIFTHY_HUB_URL}?from=utilar-customer`
    return null
  }

  return <>{children}</>
}
