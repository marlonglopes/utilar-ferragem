import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { renderHook } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import i18n from '@/i18n'
import { authLogout } from '@/lib/api'
import { AccountMenu } from '@/components/layout/AccountMenu'
import { useLogout } from '@/hooks/useLogout'
import { useAuthStore } from '@/store/authStore'
import { useCartStore } from '@/store/cartStore'

// authLogout é o ponto de contato com o servidor; mockar aqui isola o
// comportamento do hook do transporte HTTP.
vi.mock('@/lib/api', async (orig) => ({
  ...(await orig<typeof import('@/lib/api')>()),
  authLogout: vi.fn(async () => true),
}))
const authLogoutMock = vi.mocked(authLogout)

const USER = {
  id: 'u1',
  email: 'cliente@obra.com',
  name: 'João Pedreiro',
  role: 'customer' as const,
  token: 'jwt-token',
  refreshToken: 'refresh-token',
}

beforeEach(async () => {
  await i18n.changeLanguage('pt-BR')
  localStorage.clear()
  authLogoutMock.mockClear()
  authLogoutMock.mockResolvedValue(true)
  useAuthStore.setState({ user: USER })
  useCartStore.setState({ items: [] })
})

function wrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
            {children}
          </MemoryRouter>
        </I18nextProvider>
      </QueryClientProvider>
    )
  }
}

describe('useLogout — encerramento de sessão', () => {
  it('zera o usuário do store', () => {
    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })

    expect(useAuthStore.getState().isLoggedIn()).toBe(true)
    act(() => result.current())

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().isLoggedIn()).toBe(false)
    expect(useAuthStore.getState().token()).toBeNull()
  })

  it('apaga o usuário persistido no localStorage', () => {
    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    // Sessão que sobrevive no disco é sessão que não foi encerrada: quem abrir
    // o app em seguida no mesmo aparelho voltaria logado.
    const raw = localStorage.getItem('utilar-auth')
    if (raw) {
      const persisted = JSON.parse(raw) as { state: { user: unknown } }
      expect(persisted.state.user).toBeNull()
    }
  })

  it('descarta o refresh token junto com o access token', () => {
    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    // O refreshToken vale 30 dias. Deixá-lo pra trás permitiria renovar a
    // sessão depois do logout.
    expect(useAuthStore.getState().user?.refreshToken).toBeUndefined()
  })

  /**
   * REGRESSÃO: dados do usuário anterior vazando para o próximo login.
   *
   * `authStore.logout()` sozinho só zera o usuário — o cache do TanStack Query
   * continua com pedidos, endereços e nome de quem acabou de sair. Quem
   * entrasse em seguida no MESMO aparelho veria esses dados por um instante,
   * antes do refetch. Numa loja de bairro, aparelho compartilhado é a regra.
   */
  it('limpa o cache de consultas para não vazar dados entre contas', () => {
    const qc = new QueryClient()
    qc.setQueryData(['orders', 'u1'], [{ id: 'pedido-secreto', total: 999 }])
    qc.setQueryData(['product', 'cimento'], { id: '9' })
    expect(qc.getQueryData(['orders', 'u1'])).toBeDefined()

    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    expect(qc.getQueryData(['orders', 'u1'])).toBeUndefined()
    expect(qc.getQueryData(['product', 'cimento'])).toBeUndefined()
    expect(qc.getQueryCache().getAll()).toHaveLength(0)
  })

  it('preserva o carrinho — ele existe para visitante também', () => {
    useCartStore.setState({
      items: [
        {
          productId: '9',
          sellerId: 's',
          sellerName: 'Casa & Obra',
          name: 'Cimento',
          icon: '◫',
          priceSnapshot: 42.9,
          quantity: 10,
          stock: 100,
          addedAt: new Date().toISOString(),
        },
      ],
    })

    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    // Apagar o carrinho no logout faria o cliente perder a compra montada só
    // por trocar de conta.
    expect(useCartStore.getState().items).toHaveLength(1)
  })
})

describe('AccountMenu — sair da conta no cabeçalho', () => {
  function renderMenu() {
    const qc = new QueryClient()
    const utils = render(
      <QueryClientProvider client={qc}>
        <I18nextProvider i18n={i18n}>
          <MemoryRouter
            initialEntries={['/conta']}
            future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
          >
            <Routes>
              <Route path="/conta" element={<AccountMenu />} />
              <Route path="/" element={<div>Home</div>} />
            </Routes>
          </MemoryRouter>
        </I18nextProvider>
      </QueryClientProvider>
    )
    return { ...utils, qc }
  }

  /**
   * REGRESSÃO: não existir nenhum caminho para sair da conta na loja.
   *
   * "Sair" só vivia dentro da página /conta — o cliente tinha que adivinhar
   * que a saída ficava lá. Num aparelho compartilhado isso é privacidade, não
   * conveniência.
   */
  it('expõe "Sair da conta" a partir do cabeçalho', async () => {
    renderMenu()

    fireEvent.click(screen.getByRole('button', { name: /menu da conta/i }))
    expect(screen.getByRole('menuitem', { name: /sair da conta/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('menuitem', { name: /sair da conta/i }))

    await waitFor(() => expect(useAuthStore.getState().user).toBeNull())
  })

  it('redireciona para a home depois de sair', async () => {
    renderMenu()
    fireEvent.click(screen.getByRole('button', { name: /menu da conta/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /sair da conta/i }))

    await waitFor(() => expect(screen.getByText('Home')).toBeInTheDocument())
  })

  it('mostra conta, pedidos e favoritos além de sair', () => {
    renderMenu()
    fireEvent.click(screen.getByRole('button', { name: /menu da conta/i }))

    expect(screen.getByRole('menuitem', { name: /minha conta/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /meus pedidos/i })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /favoritos/i })).toBeInTheDocument()
  })

  it('visitante vê link de entrar, não menu de conta', () => {
    useAuthStore.setState({ user: null })
    renderMenu()

    expect(screen.queryByRole('button', { name: /menu da conta/i })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /entrar/i })).toHaveAttribute('href', '/entrar')
  })

  it('anuncia o estado aberto/fechado do menu', () => {
    renderMenu()
    const trigger = screen.getByRole('button', { name: /menu da conta/i })

    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
  })

  it('fecha com Escape sem sair da conta', () => {
    renderMenu()
    fireEvent.click(screen.getByRole('button', { name: /menu da conta/i }))
    expect(screen.getByRole('menu')).toBeInTheDocument()

    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    expect(useAuthStore.getState().user).not.toBeNull()
  })

})

// ---------------------------------------------------------------------------
// Revogação de sessão no servidor
//
// `authLogout` é mockado no nível do módulo porque o que importa aqui é o
// CONTRATO entre o hook e a camada de API — que ele seja chamado com os tokens
// certos, antes da limpeza, e que a limpeza aconteça de todo jeito. O formato
// da requisição HTTP em si é coberto em apiAuthLogout.test.ts.
// ---------------------------------------------------------------------------
describe('useLogout — revogação no servidor', () => {
  /**
   * REGRESSÃO: "sair" que só limpava o navegador.
   *
   * O refresh token vale 30 dias e é revogável no banco. Sem esta chamada, a
   * sessão continuava VÁLIDA no servidor depois de o cliente sair — e se o
   * token tivesse vazado (extensão maliciosa, backup do navegador, terminal
   * compartilhado da loja), "sair" não protegia absolutamente nada.
   */
  it('revoga a sessão no servidor com os dois tokens', async () => {
    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    expect(authLogoutMock).toHaveBeenCalledTimes(1)
    expect(authLogoutMock).toHaveBeenCalledWith('jwt-token', 'refresh-token')
  })

  it('captura os tokens ANTES de limpar a sessão', () => {
    // Se a ordem invertesse, o hook mandaria (null, null) e a revogação viraria
    // uma chamada vazia — falha silenciosa, do pior tipo.
    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    const [access, refresh] = authLogoutMock.mock.calls[0]
    expect(access).toBe('jwt-token')
    expect(refresh).toBe('refresh-token')
  })

  /**
   * REGRESSÃO: prender o usuário logado quando a rede cai.
   *
   * A limpeza local é o mínimo garantido; a revogação no servidor é reforço.
   * Condicionar uma à outra deixaria a pessoa logada num terminal
   * compartilhado justamente quando a rede está ruim.
   */
  it('encerra a sessão local mesmo com a API fora do ar', async () => {
    authLogoutMock.mockRejectedValueOnce(new Error('Failed to fetch'))

    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    expect(useAuthStore.getState().user).toBeNull()
    expect(qc.getQueryCache().getAll()).toHaveLength(0)
  })

  it('encerra a sessão local mesmo quando o servidor recusa a revogação', () => {
    authLogoutMock.mockResolvedValueOnce(false) // ex.: 401, token já expirado

    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    expect(useAuthStore.getState().user).toBeNull()
  })

  it('não espera a resposta do servidor para sair', () => {
    // Revogação pendurada: o usuário não pode ficar preso na tela por causa
    // de uma requisição lenta.
    authLogoutMock.mockReturnValueOnce(new Promise(() => {}))

    const qc = new QueryClient()
    const { result } = renderHook(() => useLogout(), { wrapper: wrapper(qc) })
    act(() => result.current())

    // Síncrono: já saiu, sem await de nada.
    expect(useAuthStore.getState().user).toBeNull()
  })

  it('sair pelo cabeçalho também revoga', async () => {
    const qc = new QueryClient()
    render(
      <QueryClientProvider client={qc}>
        <I18nextProvider i18n={i18n}>
          <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
            <AccountMenu />
          </MemoryRouter>
        </I18nextProvider>
      </QueryClientProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: /menu da conta/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /sair da conta/i }))

    await waitFor(() => expect(useAuthStore.getState().user).toBeNull())
    expect(authLogoutMock).toHaveBeenCalledWith('jwt-token', 'refresh-token')
  })
})
