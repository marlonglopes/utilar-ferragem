import { Component, type ErrorInfo, type ReactNode } from 'react'
import { isRouteErrorResponse, useRouteError, Link } from 'react-router-dom'
import { AlertTriangle, RotateCw } from 'lucide-react'

interface ErrorScreenProps {
  title: string
  message: string
  detail?: string
}

/** Tela de erro compartilhada pelo errorElement do router e pelo ErrorBoundary de classe. */
export function ErrorScreen({ title, message, detail }: ErrorScreenProps) {
  return (
    <div
      role="alert"
      className="flex flex-col items-center justify-center min-h-[60vh] gap-6 text-center px-4 py-12"
    >
      <span className="w-16 h-16 rounded-2xl bg-brand-orange-light flex items-center justify-center">
        <AlertTriangle className="h-8 w-8 text-brand-orange" aria-hidden />
      </span>

      <div className="max-w-md">
        <h1 className="font-display font-black text-2xl text-gray-900 mb-2">{title}</h1>
        <p className="text-gray-500 leading-relaxed">{message}</p>
        {detail && (
          <pre className="mt-4 max-h-40 overflow-auto rounded-lg bg-gray-100 p-3 text-left text-xs text-gray-600 whitespace-pre-wrap break-words">
            {detail}
          </pre>
        )}
      </div>

      <div className="flex flex-wrap items-center justify-center gap-3">
        <button
          onClick={() => window.location.reload()}
          className="inline-flex items-center gap-2 bg-brand-orange hover:bg-brand-orange-dark text-white font-semibold px-5 py-2.5 rounded-lg transition-colors"
        >
          <RotateCw className="h-4 w-4" aria-hidden />
          Recarregar a página
        </button>
        <Link
          to="/"
          className="inline-flex items-center gap-2 border border-gray-300 hover:bg-gray-50 text-gray-700 font-semibold px-5 py-2.5 rounded-lg transition-colors"
        >
          Voltar ao início
        </Link>
      </div>
    </div>
  )
}

/**
 * errorElement do router. Captura throws de render/loader em qualquer rota —
 * sem isso, qualquer exceção derruba o app inteiro para uma tela branca.
 */
export function RouteErrorBoundary() {
  const error = useRouteError()

  if (isRouteErrorResponse(error)) {
    return (
      <ErrorScreen
        title={`Erro ${error.status}`}
        message={
          error.status === 404
            ? 'A página que você procura não existe ou foi movida.'
            : error.statusText || 'Não foi possível carregar esta página.'
        }
      />
    )
  }

  const detail = import.meta.env.DEV && error instanceof Error ? error.stack : undefined

  return (
    <ErrorScreen
      title="Algo deu errado"
      message="Tivemos um problema ao carregar esta página. Tente recarregar — se o erro continuar, fale com nosso atendimento."
      detail={detail}
    />
  )
}

interface ErrorBoundaryProps {
  children: ReactNode
  fallback?: ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

/**
 * ErrorBoundary de classe para trechos fora da árvore do router
 * (providers, widgets como a Alice). O router usa RouteErrorBoundary.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // TODO(observabilidade): enviar para o Sentry quando o DSN estiver configurado.
    console.error('[ErrorBoundary]', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback
      return (
        <ErrorScreen
          title="Algo deu errado"
          message="Tivemos um problema inesperado. Tente recarregar a página."
          detail={import.meta.env.DEV ? this.state.error.stack : undefined}
        />
      )
    }
    return this.props.children
  }
}
