import { createBrowserRouter } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/PublicLayout'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import HomePage from '@/pages/home/HomePage'
import CategoryPage from '@/pages/category/CategoryPage'
import ProductDetailPage from '@/pages/product/ProductDetailPage'
import SearchPage from '@/pages/search/SearchPage'
import CartPage from '@/pages/cart/CartPage'
import LoginPage from '@/pages/auth/LoginPage'
import RegisterPage from '@/pages/auth/RegisterPage'
import ForgotPasswordPage from '@/pages/auth/ForgotPasswordPage'
import AccountPage from '@/pages/account/AccountPage'
import NotFoundPage from '@/pages/NotFoundPage'
import UiPage from '@/pages/_dev/UiPage'

const router = createBrowserRouter([
  {
    path: '/',
    element: <PublicLayout />,
    children: [
      { index: true, element: <HomePage /> },
      { path: 'categoria/:slug', element: <CategoryPage /> },
      { path: 'produto/:slug', element: <ProductDetailPage /> },
      { path: 'busca', element: <SearchPage /> },
      { path: 'carrinho', element: <CartPage /> },
      { path: 'entrar', element: <LoginPage /> },
      { path: 'cadastro', element: <RegisterPage /> },
      { path: 'esqueci-senha', element: <ForgotPasswordPage /> },
      {
        path: 'conta',
        element: <ProtectedRoute><AccountPage /></ProtectedRoute>,
      },
      { path: '404', element: <NotFoundPage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
  {
    path: '/_dev/ui',
    element: <UiPage />,
  },
])

export default router
