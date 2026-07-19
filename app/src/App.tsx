import { RouterProvider } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { HelmetProvider } from 'react-helmet-async'
import { queryClient } from '@/lib/queryClient'
import { ToastProvider } from '@/components/ui/Toast'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import router from '@/router'
import '@/i18n'

export default function App() {
  return (
    // ErrorBoundary externo cobre o que está fora da árvore do router
    // (providers). Dentro das rotas, quem captura é o errorElement.
    <ErrorBoundary>
      <HelmetProvider>
        <QueryClientProvider client={queryClient}>
          <ToastProvider>
            <RouterProvider router={router} />
          </ToastProvider>
        </QueryClientProvider>
      </HelmetProvider>
    </ErrorBoundary>
  )
}
