import { createBrowserRouter } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/PublicLayout'
import HomePage from '@/pages/home/HomePage'
import CategoryPage from '@/pages/category/CategoryPage'
import ProductDetailPage from '@/pages/product/ProductDetailPage'
import SearchPage from '@/pages/search/SearchPage'
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
