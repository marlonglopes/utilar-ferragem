import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, beforeAll, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'

// Login em 2 passos exige o backend (isAuthEnabled) — mockamos @/lib/api para
// controlar as respostas do login e do verify-totp.
vi.mock('@/lib/api', () => ({ isAuthEnabled: true, authPost: vi.fn() }))

import { authPost } from '@/lib/api'
import LoginPage from '@/pages/auth/LoginPage'
import { useAuthStore } from '@/store/authStore'

const mockedPost = vi.mocked(authPost)

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})
beforeEach(() => {
  useAuthStore.setState({ user: null })
  mockedPost.mockReset()
})

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <LoginPage />
      </MemoryRouter>
    </I18nextProvider>
  )
}

async function submitPassword() {
  fireEvent.change(screen.getByLabelText(/e-mail/i), { target: { value: 'admin@utilar.com.br' } })
  fireEvent.change(screen.getByLabelText(/senha/i), { target: { value: 'senha-forte' } })
  fireEvent.click(screen.getByRole('button', { name: /^entrar$/i }))
}

describe('LoginPage — MFA em 2 passos', () => {
  it('senha OK com MFA pede o código; código correto emite a sessão', async () => {
    mockedPost
      .mockResolvedValueOnce({ mfaRequired: true, challenge: 'chal-123' })
      .mockResolvedValueOnce({
        user: { id: 'a1', email: 'admin@utilar.com.br', name: 'Admin', role: 'admin' },
        accessToken: 'acc',
        refreshToken: 'ref',
      })

    renderPage()
    await submitPassword()

    // 1º passo não logou — apareceu o campo de código.
    const codeInput = await screen.findByLabelText(/código de verificação/i)
    expect(useAuthStore.getState().user).toBeNull()
    expect(mockedPost.mock.calls[0][0]).toBe('/api/v1/auth/login')

    fireEvent.change(codeInput, { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: /verificar/i }))

    await waitFor(() => expect(useAuthStore.getState().user?.email).toBe('admin@utilar.com.br'))
    expect(mockedPost.mock.calls[1][0]).toBe('/api/v1/auth/login/verify-totp')
    expect(mockedPost.mock.calls[1][1]).toMatchObject({ challenge: 'chal-123', code: '123456' })
  })

  it('código errado mostra erro e NÃO cria sessão', async () => {
    mockedPost
      .mockResolvedValueOnce({ mfaRequired: true, challenge: 'chal-x' })
      .mockRejectedValueOnce(new Error('invalid'))

    renderPage()
    await submitPassword()
    const codeInput = await screen.findByLabelText(/código de verificação/i)
    fireEvent.change(codeInput, { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: /verificar/i }))

    await waitFor(() => expect(screen.getByText(/código inválido/i)).toBeInTheDocument())
    expect(useAuthStore.getState().user).toBeNull()
  })

  it('sem MFA, o login entra direto (não pede código)', async () => {
    mockedPost.mockResolvedValueOnce({
      user: { id: 'c1', email: 'cliente@x.com', name: 'Cliente', role: 'customer' },
      accessToken: 'acc',
      refreshToken: 'ref',
    })
    renderPage()
    await submitPassword()
    await waitFor(() => expect(useAuthStore.getState().user?.email).toBe('cliente@x.com'))
    expect(screen.queryByLabelText(/código de verificação/i)).not.toBeInTheDocument()
  })
})
