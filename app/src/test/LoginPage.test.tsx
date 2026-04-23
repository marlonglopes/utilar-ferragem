import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import LoginPage from '@/pages/auth/LoginPage'
import { useAuthStore } from '@/store/authStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  useAuthStore.setState({ user: null })
})

function renderLoginPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <LoginPage />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('LoginPage', () => {
  it('renders the login form', () => {
    renderLoginPage()
    expect(screen.getByRole('heading', { name: /bem-vindo/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/e-mail/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/senha/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /entrar/i })).toBeInTheDocument()
  })

  it('shows a link to register page', () => {
    renderLoginPage()
    expect(screen.getByRole('link', { name: /criar conta/i })).toBeInTheDocument()
  })

  it('shows a link to forgot password', () => {
    renderLoginPage()
    expect(screen.getByRole('link', { name: /esqueci/i })).toBeInTheDocument()
  })

  it('logs in with stub (no API) and sets user in store', async () => {
    renderLoginPage()
    fireEvent.change(screen.getByLabelText(/e-mail/i), { target: { value: 'joao@example.com' } })
    fireEvent.change(screen.getByLabelText(/senha/i), { target: { value: 'qualquersenha' } })
    fireEvent.click(screen.getByRole('button', { name: /entrar/i }))

    await waitFor(() => {
      expect(useAuthStore.getState().user?.email).toBe('joao@example.com')
    })
  })
})
