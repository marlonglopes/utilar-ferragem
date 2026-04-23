import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { useAuthStore } from '@/store/authStore'
import type { User } from '@/store/authStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useAuthStore.setState({ user: null })
})

const mockUser: User = {
  id: 'u1',
  email: 'joao@example.com',
  name: 'João',
  role: 'customer',
  token: 'tok',
}

function renderWithRoute(initialPath = '/conta') {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter
        initialEntries={[initialPath]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/conta"
            element={
              <ProtectedRoute>
                <div>Página protegida</div>
              </ProtectedRoute>
            }
          />
          <Route path="/entrar" element={<div>Página de login</div>} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('ProtectedRoute', () => {
  it('redireciona para /entrar quando não está logado', () => {
    renderWithRoute('/conta')
    expect(screen.getByText(/página de login/i)).toBeInTheDocument()
    expect(screen.queryByText(/página protegida/i)).not.toBeInTheDocument()
  })

  it('renderiza o conteúdo quando usuário está logado como customer', () => {
    useAuthStore.setState({ user: mockUser })
    renderWithRoute('/conta')
    expect(screen.getByText(/página protegida/i)).toBeInTheDocument()
  })
})
