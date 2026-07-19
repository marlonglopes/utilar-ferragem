import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { MemoryRouter, RouterProvider, createMemoryRouter } from 'react-router-dom'
import { ErrorBoundary, ErrorScreen } from '@/components/common/ErrorBoundary'

function Boom(): JSX.Element {
  throw new Error('explosão de propósito')
}

describe('ErrorBoundary', () => {
  // O React loga o erro capturado no console; silenciamos para não poluir a saída.
  let consoleError: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleError.mockRestore()
  })

  it('renderiza os filhos quando não há erro', () => {
    render(
      <MemoryRouter>
        <ErrorBoundary>
          <p>conteúdo normal</p>
        </ErrorBoundary>
      </MemoryRouter>
    )
    expect(screen.getByText('conteúdo normal')).toBeInTheDocument()
  })

  it('captura o throw e mostra a tela de erro em vez de quebrar a árvore', () => {
    render(
      <MemoryRouter>
        <ErrorBoundary>
          <Boom />
        </ErrorBoundary>
      </MemoryRouter>
    )

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /algo deu errado/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /recarregar/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /voltar ao início/i })).toBeInTheDocument()
  })

  it('aceita um fallback customizado', () => {
    render(
      <MemoryRouter>
        <ErrorBoundary fallback={<p>fallback customizado</p>}>
          <Boom />
        </ErrorBoundary>
      </MemoryRouter>
    )
    expect(screen.getByText('fallback customizado')).toBeInTheDocument()
  })

  it('o botão recarregar chama window.location.reload', async () => {
    const reload = vi.fn()
    const original = window.location
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { ...original, reload },
    })

    render(
      <MemoryRouter>
        <ErrorScreen title="Erro" message="algo quebrou" />
      </MemoryRouter>
    )
    await userEvent.click(screen.getByRole('button', { name: /recarregar/i }))
    expect(reload).toHaveBeenCalledOnce()

    Object.defineProperty(window, 'location', { configurable: true, value: original })
  })
})

describe('errorElement do router', () => {
  let consoleError: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleError.mockRestore()
  })

  it('mostra a tela de erro quando uma rota lança durante o render', () => {
    // Reproduz a regressão original: antes do errorElement, um throw numa rota
    // derrubava o app inteiro para uma tela branca.
    const router = createMemoryRouter(
      [{ path: '/', element: <Boom />, errorElement: <ErrorScreen title="Algo deu errado" message="falhou" /> }],
      { initialEntries: ['/'] }
    )

    render(<RouterProvider router={router} />)

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /algo deu errado/i })).toBeInTheDocument()
  })
})
