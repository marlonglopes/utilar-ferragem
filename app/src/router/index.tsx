import { createBrowserRouter } from 'react-router-dom'
import { PublicLayout } from '@/components/layout/PublicLayout'
import HomePage from '@/pages/home/HomePage'
import UiPage from '@/pages/_dev/UiPage'

const router = createBrowserRouter([
  {
    path: '/',
    element: <PublicLayout />,
    children: [
      { index: true, element: <HomePage /> },
    ],
  },
  {
    path: '/_dev/ui',
    element: <UiPage />,
  },
])

export default router
