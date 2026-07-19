import { lazy, Suspense, type ReactNode } from 'react'
import type { RouteObject } from 'react-router-dom'
import { AdminRoute } from '@/components/admin/AdminRoute'
import { RouteErrorBoundary } from '@/components/common/ErrorBoundary'
import { PageFallback } from '@/components/common/PageFallback'

/**
 * Rotas do painel administrativo, prontas para serem espalhadas no router raiz.
 *
 * Ficam FORA do `PublicLayout`: o painel tem chrome próprio (`AdminShell`, tela
 * cheia, navegação própria) e o dono, no meio de uma conferência contábil, não
 * deve ter um caminho de um toque para a vitrine da loja.
 *
 * Todas carregadas sob demanda — cada tela vira um chunk próprio e o bundle da
 * home não paga nada por elas. Quem nunca abre `/admin` nunca baixa o painel.
 *
 * Costura em `src/router/index.tsx`:
 *
 *   import { adminRoutes } from '@/router/adminRoutes'
 *
 *   const router = createBrowserRouter([
 *     { path: '/', element: <PublicLayout />, children: [ ... ] },
 *     ...balcaoRoutes,
 *     ...adminRoutes,
 *     ...
 *   ])
 *
 * Atenção (mesma nota do balcão): o catch-all `{ path: '*' }` hoje vive DENTRO
 * dos children de '/', então não intercepta estas rotas de topo. Se um dia for
 * promovido para a raiz, `...adminRoutes` precisa vir antes dele.
 */

const OverviewPage = lazy(() => import('@/pages/admin/OverviewPage'))
const AccountingPage = lazy(() => import('@/pages/admin/AccountingPage'))
const SellersPage = lazy(() => import('@/pages/admin/SellersPage'))
const AuditTrailPage = lazy(() => import('@/pages/admin/AuditTrailPage'))
const ObservabilityPage = lazy(() => import('@/pages/admin/ObservabilityPage'))
const ImportPage = lazy(() => import('@/pages/admin/ImportPage'))

/** Envolve a página no guard de papel + Suspense + fronteira de erro. */
function adminPage(element: ReactNode): ReactNode {
  return (
    <AdminRoute>
      <Suspense fallback={<PageFallback />}>{element}</Suspense>
    </AdminRoute>
  )
}

export const adminRoutes: RouteObject[] = [
  {
    path: '/admin',
    element: adminPage(<OverviewPage />),
    errorElement: <RouteErrorBoundary />,
  },
  {
    path: '/admin/contabil',
    element: adminPage(<AccountingPage />),
    errorElement: <RouteErrorBoundary />,
  },
  {
    path: '/admin/vendedores',
    element: adminPage(<SellersPage />),
    errorElement: <RouteErrorBoundary />,
  },
  {
    // Rota em pt-BR como o resto do app. `/admin/importar` é verbo: é uma ação
    // que o dono executa, não um relatório que ele consulta.
    path: '/admin/importar',
    element: adminPage(<ImportPage />),
    errorElement: <RouteErrorBoundary />,
  },
  {
    path: '/admin/trilha',
    element: adminPage(<AuditTrailPage />),
    errorElement: <RouteErrorBoundary />,
  },
  {
    path: '/admin/observabilidade',
    element: adminPage(<ObservabilityPage />),
    errorElement: <RouteErrorBoundary />,
  },
]

export default adminRoutes
