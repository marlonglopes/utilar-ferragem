import { render, screen } from '@testing-library/react'
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
    </MemoryRouter>,
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

  it('libera também para customer, já que não há autorização a aplicar sem backend', () => {
    setUser('customer')
    renderGuard()
    expect(screen.getByText('conteúdo do painel')).toBeInTheDocument()
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
      </MemoryRouter>,
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
