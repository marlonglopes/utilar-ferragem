import { render, screen } from '@testing-library/react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { BalcaoRoute as BalcaoRouteType } from '@/components/balcao/BalcaoRoute'

/**
 * Guard do PDV — quem entra no balcão.
 *
 * O caso que estes testes existem para travar é `seller`: no auth-service
 * `seller` é o LOJISTA DO MARKETPLACE (quem anuncia no site), não o vendedor de
 * balcão. Liberá-lo aqui daria o caixa da loja física a todo lojista cadastrado
 * — e é um erro fácil de cometer, porque a palavra é a mesma.
 *
 * Em mock/dev o guard libera de propósito (o PDV precisa ser demonstrável sem
 * backend), então o modo restritivo só aparece recarregando o módulo com
 * `isAuthEnabled: true` e `DEV: false` — o estado do bundle de produção.
 */

type Role = 'customer' | 'seller' | 'admin' | 'store_operator'

async function loadStrictGuard(role: Role | null) {
  vi.resetModules()
  vi.stubEnv('DEV', false)
  vi.doMock('@/lib/api', () => ({
    isAuthEnabled: true,
    isApiEnabled: true,
    isOrderEnabled: true,
    isCatalogEnabled: true,
  }))
  const mod = await import('@/components/balcao/BalcaoRoute')
  // `resetModules` cria uma instância NOVA do authStore para este grafo de
  // módulos: semear o store de fora não teria efeito sobre o guard recarregado.
  const { useAuthStore } = await import('@/store/authStore')
  useAuthStore.setState({
    user:
      role === null
        ? null
        : {
            id: 'u1',
            email: 'a@b.c',
            name: 'Teste',
            // `store_operator` ainda não está no union de `User['role']`
            // (authStore.ts é de outro dono); o guard compara como string.
            role: role as 'customer' | 'seller' | 'admin',
            token: 't',
          },
  })
  return mod.BalcaoRoute
}

function renderWith(Guard: typeof BalcaoRouteType) {
  return render(
    <MemoryRouter
      initialEntries={['/balcao']}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/balcao"
          element={
            <Guard>
              <div>PDV do balcão</div>
            </Guard>
          }
        />
        <Route path="/entrar" element={<div>tela de login</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

afterEach(() => {
  vi.resetModules()
  vi.unstubAllEnvs()
  vi.doUnmock('@/lib/api')
})

describe('BalcaoRoute — auth real em produção', () => {
  it('libera store_operator: é o papel do vendedor no caixa', async () => {
    const Guard = await loadStrictGuard('store_operator')
    renderWith(Guard)
    expect(screen.getByText('PDV do balcão')).toBeInTheDocument()
  })

  it('libera admin, que entra junto para suporte', async () => {
    const Guard = await loadStrictGuard('admin')
    renderWith(Guard)
    expect(screen.getByText('PDV do balcão')).toBeInTheDocument()
  })

  it('BARRA seller — lojista do marketplace não é vendedor de balcão', async () => {
    const Guard = await loadStrictGuard('seller')
    renderWith(Guard)
    expect(screen.getByRole('heading', { name: /acesso restrito ao balcão/i })).toBeInTheDocument()
    expect(screen.queryByText('PDV do balcão')).not.toBeInTheDocument()
  })

  it('barra customer com tela de acesso restrito', async () => {
    const Guard = await loadStrictGuard('customer')
    renderWith(Guard)
    expect(screen.getByRole('heading', { name: /acesso restrito ao balcão/i })).toBeInTheDocument()
    expect(screen.queryByText('PDV do balcão')).not.toBeInTheDocument()
  })

  it('manda quem não está logado para o login, preservando o destino', async () => {
    const Guard = await loadStrictGuard(null)
    renderWith(Guard)
    expect(screen.getByText('tela de login')).toBeInTheDocument()
    expect(screen.queryByText('PDV do balcão')).not.toBeInTheDocument()
  })
})
