import { render, screen, cleanup } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AdminRoute } from '@/components/admin/AdminRoute'
import { useAuthStore, type User } from '@/store/authStore'

/**
 * O guard tem dois modos e ambos importam:
 *
 * - **mock/dev** (`isAuthEnabled === false` ou `DEV`): libera, para o painel ser
 *   demonstrável sem backend — mesma decisão do balcão.
 * - **auth real em produção**: só `admin` passa.
 *
 * Como `isAuthEnabled` é derivado de `import.meta.env` no carregamento do
 * módulo, o teste do modo restritivo precisa mockar `@/lib/api` e reimportar.
 */

function renderGuard() {
  return render(
    <MemoryRouter
      initialEntries={['/admin']}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/admin"
          element={
            <AdminRoute>
              <div>conteúdo do painel</div>
            </AdminRoute>
          }
        />
        <Route path="/entrar" element={<div>tela de login</div>} />
      </Routes>
    </MemoryRouter>
  )
}

function setUser(role: User['role'] | null) {
  if (role === null) {
    useAuthStore.setState({ user: null })
    return
  }
  useAuthStore.setState({
    user: { id: 'u1', email: 'a@b.c', name: 'Teste', role, token: 't' },
  })
}

beforeEach(() => {
  setUser(null)
})

afterEach(() => {
  vi.resetModules()
  vi.unstubAllEnvs()
})

describe('AdminRoute — modo mock (sem auth-service)', () => {
  it('libera o painel sem usuário logado, para dar para demonstrar', () => {
    renderGuard()
    expect(screen.getByText('conteúdo do painel')).toBeInTheDocument()
  })

  it('mas se uma persona logou, aplica a matriz mesmo em mock (demo fiel de papéis)', () => {
    // customer não é papel de operação → barrado, mesmo sem backend. É o que
    // torna a demonstração de "cada um vê só o seu" fiel na apresentação.
    setUser('customer')
    renderGuard()
    expect(screen.getByRole('heading', { name: /acesso restrito ao painel/i })).toBeInTheDocument()
    expect(screen.queryByText('conteúdo do painel')).not.toBeInTheDocument()
  })
})

describe('AdminRoute — auth real em produção', () => {
  /**
   * Recarrega o módulo com `isAuthEnabled: true` e `DEV: false`, que é o estado
   * do bundle que vai para produção.
   */
  async function loadStrictGuard(role: User['role'] | null) {
    vi.resetModules()
    vi.stubEnv('DEV', false)
    vi.doMock('@/lib/api', () => ({
      isAuthEnabled: true,
      isApiEnabled: true,
      isOrderEnabled: true,
      isCatalogEnabled: true,
    }))
    const mod = await import('@/components/admin/AdminRoute')
    // `resetModules` cria uma instância NOVA do authStore para este grafo de
    // módulos. Escrever no store do escopo externo não teria efeito nenhum
    // sobre o guard recarregado — daí o usuário ser semeado aqui dentro.
    const { useAuthStore: freshStore } = await import('@/store/authStore')
    freshStore.setState({
      user: role === null ? null : { id: 'u1', email: 'a@b.c', name: 'Teste', role, token: 't' },
    })
    return mod.AdminRoute
  }

  function renderWith(Guard: typeof AdminRoute) {
    return render(
      <MemoryRouter
        initialEntries={['/admin']}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/admin"
            element={
              <Guard>
                <div>conteúdo do painel</div>
              </Guard>
            }
          />
          <Route path="/entrar" element={<div>tela de login</div>} />
        </Routes>
      </MemoryRouter>
    )
  }

  it('manda quem não está logado para o login', async () => {
    const Guard = await loadStrictGuard(null)
    renderWith(Guard)
    expect(screen.getByText('tela de login')).toBeInTheDocument()
    expect(screen.queryByText('conteúdo do painel')).not.toBeInTheDocument()
  })

  it('barra customer com tela de acesso restrito', async () => {
    const Guard = await loadStrictGuard('customer')
    renderWith(Guard)
    expect(screen.getByRole('heading', { name: /acesso restrito ao painel/i })).toBeInTheDocument()
    expect(screen.queryByText('conteúdo do painel')).not.toBeInTheDocument()
  })

  it('barra seller — seller é lojista do marketplace, não administrador', async () => {
    const Guard = await loadStrictGuard('seller')
    renderWith(Guard)
    expect(screen.getByRole('heading', { name: /acesso restrito ao painel/i })).toBeInTheDocument()
  })

  it('deixa admin passar', async () => {
    const Guard = await loadStrictGuard('admin')
    renderWith(Guard)
    expect(screen.getByText('conteúdo do painel')).toBeInTheDocument()
  })
})

describe('AdminRoute — roteamento por persona (produção)', () => {
  async function loadGuard(role: User['role']) {
    vi.resetModules()
    vi.stubEnv('DEV', false)
    vi.doMock('@/lib/api', () => ({
      isAuthEnabled: true,
      isApiEnabled: true,
      isOrderEnabled: true,
      isCatalogEnabled: true,
    }))
    const mod = await import('@/components/admin/AdminRoute')
    const { useAuthStore: freshStore } = await import('@/store/authStore')
    freshStore.setState({
      user: { id: 'u1', email: 'a@b.c', name: 'Teste', role, token: 't' },
    })
    return mod.AdminRoute
  }

  // Monta várias seções do painel para os redirects de "home" resolverem.
  function renderAt(Guard: typeof AdminRoute, path: string) {
    const page = (label: string) => (
      <Guard>
        <div>{label}</div>
      </Guard>
    )
    return render(
      <MemoryRouter
        initialEntries={[path]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/admin" element={page('visão geral')} />
          <Route path="/admin/contabil" element={page('contábil')} />
          <Route path="/admin/pedidos" element={page('pedidos')} />
          <Route path="/admin/produtos" element={page('produtos')} />
        </Routes>
      </MemoryRouter>
    )
  }

  it('contador em /admin (visão geral) cai no home dele (contábil)', async () => {
    const Guard = await loadGuard('contador')
    renderAt(Guard, '/admin')
    expect(screen.getByText('contábil')).toBeInTheDocument()
    expect(screen.queryByText('visão geral')).not.toBeInTheDocument()
  })

  it('vendas abre produtos (vê custo) mas é desviado do contábil', async () => {
    let Guard = await loadGuard('vendas')
    renderAt(Guard, '/admin/produtos')
    expect(screen.getByText('produtos')).toBeInTheDocument()
    cleanup()

    Guard = await loadGuard('vendas')
    renderAt(Guard, '/admin/contabil')
    // vendas não entra no contábil → vai pro home dele (pedidos)
    expect(screen.getByText('pedidos')).toBeInTheDocument()
    expect(screen.queryByText('contábil')).not.toBeInTheDocument()
  })

  it('almoxarife abre pedidos mas é desviado de produtos (não vê custo)', async () => {
    let Guard = await loadGuard('almoxarife')
    renderAt(Guard, '/admin/pedidos')
    expect(screen.getByText('pedidos')).toBeInTheDocument()
    cleanup()

    Guard = await loadGuard('almoxarife')
    renderAt(Guard, '/admin/produtos')
    expect(screen.getByText('pedidos')).toBeInTheDocument()
    expect(screen.queryByText('produtos')).not.toBeInTheDocument()
  })
})
