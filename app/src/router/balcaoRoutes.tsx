import type { RouteObject } from 'react-router-dom'
import { BalcaoRoute } from '@/components/balcao/BalcaoRoute'
import BalcaoPage from '@/pages/balcao/BalcaoPage'
import BalcaoSuccessPage from '@/pages/balcao/BalcaoSuccessPage'
import BalcaoApprovalsPage from '@/pages/balcao/BalcaoApprovalsPage'

/**
 * Rotas do PDV de balcão, prontas para serem espalhadas no router raiz.
 *
 * Ficam FORA do `PublicLayout`: o balcão tem chrome próprio (BalcaoTopBar,
 * tela cheia, sem navbar/footer do e-commerce) e o vendedor não deve ter
 * caminho de um toque para a loja pública no meio de um atendimento.
 *
 * Costura em `src/router/index.tsx`:
 *
 *   import { balcaoRoutes } from '@/router/balcaoRoutes'
 *
 *   const router = createBrowserRouter([
 *     { path: '/', element: <PublicLayout />, children: [ ... ] },
 *     ...balcaoRoutes,
 *     { path: '/_dev/ui', element: <UiPage /> },
 *   ])
 *
 * Atenção: o catch-all `{ path: '*' }` hoje vive DENTRO dos children de '/', então
 * ele não intercepta essas rotas de topo. Se algum dia for promovido para o nível
 * raiz, `...balcaoRoutes` precisa vir antes dele.
 */
export const balcaoRoutes: RouteObject[] = [
  {
    path: '/balcao',
    element: (
      <BalcaoRoute>
        <BalcaoPage />
      </BalcaoRoute>
    ),
  },
  {
    // Fila do gerente. O guard é o mesmo do PDV (papel de operador); o poder de
    // HOMOLOGAR é checado dentro da página, contra `canApproveDiscount` do
    // vínculo — cargo não é papel, e só o backend sabe o cargo.
    path: '/balcao/aprovacoes',
    element: (
      <BalcaoRoute>
        <BalcaoApprovalsPage />
      </BalcaoRoute>
    ),
  },
  {
    path: '/balcao/venda-concluida',
    element: (
      <BalcaoRoute>
        <BalcaoSuccessPage />
      </BalcaoRoute>
    ),
  },
]

export default balcaoRoutes
