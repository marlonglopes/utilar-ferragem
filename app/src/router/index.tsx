import { lazy, Suspense, type ReactNode } from 'react'
import { createBrowserRouter } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/PublicLayout'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { RouteErrorBoundary } from '@/components/common/ErrorBoundary'
import { PageFallback } from '@/components/common/PageFallback'
import { balcaoRoutes } from '@/router/balcaoRoutes'
import { adminRoutes } from '@/router/adminRoutes'

// Todas as rotas são carregadas sob demanda (React.lazy). Sem isso o bundle
// vira um chunk único de ~520 kB, penalizando o LCP de quem cai na home vindo
// do Google. Cada página vira um chunk próprio pedido só quando visitada.
const HomePage = lazy(() => import('@/pages/home/HomePage'))
const CategoryPage = lazy(() => import('@/pages/category/CategoryPage'))
const ProductDetailPage = lazy(() => import('@/pages/product/ProductDetailPage'))
const SearchPage = lazy(() => import('@/pages/search/SearchPage'))
const CartPage = lazy(() => import('@/pages/cart/CartPage'))
const LoginPage = lazy(() => import('@/pages/auth/LoginPage'))
const RegisterPage = lazy(() => import('@/pages/auth/RegisterPage'))
const ForgotPasswordPage = lazy(() => import('@/pages/auth/ForgotPasswordPage'))
const AccountPage = lazy(() => import('@/pages/account/AccountPage'))
const CheckoutPage = lazy(() => import('@/pages/checkout/CheckoutPage'))
const OrderConfirmationPage = lazy(() => import('@/pages/checkout/OrderConfirmationPage'))
const OrderDetailPage = lazy(() => import('@/pages/orders/OrderDetailPage'))
const NotFoundPage = lazy(() => import('@/pages/NotFoundPage'))

// Institucionais e legais
const AboutPage = lazy(() => import('@/pages/institutional/AboutPage'))
const ContactPage = lazy(() => import('@/pages/institutional/ContactPage'))
const HelpPage = lazy(() => import('@/pages/institutional/HelpPage'))
const CategoriesPage = lazy(() => import('@/pages/institutional/CategoriesPage'))
const SellPage = lazy(() => import('@/pages/institutional/SellPage'))
const PrivacyPage = lazy(() => import('@/pages/institutional/PrivacyPage'))
const TermsPage = lazy(() => import('@/pages/institutional/TermsPage'))

/** Envolve o elemento da rota no Suspense com o skeleton de carregamento. */
function page(element: ReactNode) {
  return <Suspense fallback={<PageFallback />}>{element}</Suspense>
}

const router = createBrowserRouter([
  {
    path: '/',
    element: <PublicLayout />,
    // Captura qualquer throw de render nas rotas filhas — antes disso, uma
    // exceção em qualquer página derrubava o app para uma tela branca.
    errorElement: <RouteErrorBoundary />,
    children: [
      { index: true, element: page(<HomePage />) },
      { path: 'categoria/:slug', element: page(<CategoryPage />) },
      { path: 'produto/:slug', element: page(<ProductDetailPage />) },
      { path: 'busca', element: page(<SearchPage />) },
      { path: 'carrinho', element: page(<CartPage />) },
      { path: 'entrar', element: page(<LoginPage />) },
      { path: 'cadastro', element: page(<RegisterPage />) },
      { path: 'esqueci-senha', element: page(<ForgotPasswordPage />) },
      {
        path: 'conta',
        element: <ProtectedRoute>{page(<AccountPage />)}</ProtectedRoute>,
      },
      {
        path: 'checkout',
        element: <ProtectedRoute>{page(<CheckoutPage />)}</ProtectedRoute>,
      },
      { path: 'pedido/:id', element: page(<OrderConfirmationPage />) },
      {
        path: 'conta/pedidos/:id',
        element: <ProtectedRoute>{page(<OrderDetailPage />)}</ProtectedRoute>,
      },

      // Institucionais e legais (slugs pt-BR, linkados no topbar e no rodapé)
      { path: 'sobre', element: page(<AboutPage />) },
      { path: 'contato', element: page(<ContactPage />) },
      { path: 'ajuda', element: page(<HelpPage />) },
      { path: 'categorias', element: page(<CategoriesPage />) },
      { path: 'vender', element: page(<SellPage />) },
      { path: 'privacidade', element: page(<PrivacyPage />) },
      { path: 'termos', element: page(<TermsPage />) },

      { path: '404', element: page(<NotFoundPage />) },
      { path: '*', element: page(<NotFoundPage />) },
    ],
  },
  // Balcão (PDV da loja física) — fica no topo, fora do PublicLayout, porque tem
  // chrome próprio de tela cheia: sem navbar, sem rodapé, sem barra de categorias.
  // O catch-all '*' acima é filho de '/', então não intercepta estas rotas; se um
  // dia ele subir para a raiz, `...balcaoRoutes` precisa vir antes dele.
  ...balcaoRoutes,
  // Painel do dono: contábil, vendedores, trilha de auditoria, importação.
  ...adminRoutes,
  // Showcase do design system: só existe em desenvolvimento. `import.meta.env.DEV`
  // vira `false` no build de produção, então o Rollup elimina o ramo inteiro e o
  // chunk da UiPage nunca é gerado.
  ...(import.meta.env.DEV
    ? (() => {
        const UiPage = lazy(() => import('@/pages/_dev/UiPage'))
        return [
          {
            path: '/_dev/ui',
            element: page(<UiPage />),
            errorElement: <RouteErrorBoundary />,
          },
        ]
      })()
    : []),
])

export default router
