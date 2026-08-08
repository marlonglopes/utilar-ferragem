import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@/lib/api', () => ({ isAuthEnabled: true, authGet: vi.fn(), authPost: vi.fn() }))

import { authGet, authPost } from '@/lib/api'
import MfaSetupPage from '@/pages/account/MfaSetupPage'
import { useAuthStore } from '@/store/authStore'

const mockedGet = vi.mocked(authGet)
const mockedPost = vi.mocked(authPost)

beforeEach(() => {
  mockedGet.mockReset()
  mockedPost.mockReset()
  useAuthStore.setState({
    user: { id: 'a1', email: 'admin@utilar.com.br', name: 'Admin', role: 'admin', token: 'tok' },
  })
})

function renderPage() {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <MfaSetupPage />
    </MemoryRouter>
  )
}

describe('MfaSetupPage', () => {
  it('conta com 2FA ativo mostra o estado ativo', async () => {
    mockedGet.mockResolvedValueOnce({ mfaEnabled: true })
    renderPage()
    await waitFor(() => expect(screen.getByText(/2FA ativo/i)).toBeInTheDocument())
  })

  it('ativa o 2FA: enroll mostra o QR + segredo, confirmar ativa', async () => {
    mockedGet.mockResolvedValueOnce({ mfaEnabled: false })
    renderPage()

    const ativar = await screen.findByRole('button', { name: /ativar 2fa/i })
    mockedPost.mockResolvedValueOnce({
      secret: 'ABC234ZZ',
      otpauthUri: 'otpauth://totp/Utilar%20Ferragem:admin@utilar.com.br?secret=ABC234ZZ',
    })
    fireEvent.click(ativar)

    // QR + segredo aparecem.
    await waitFor(() => expect(screen.getByText('ABC234ZZ')).toBeInTheDocument())
    expect(screen.getByLabelText(/QR Code do 2FA/i)).toBeInTheDocument()

    // Confirma com um código de 6 dígitos.
    mockedPost.mockResolvedValueOnce({ mfaEnabled: true })
    fireEvent.change(screen.getByLabelText(/código de confirmação/i), {
      target: { value: '123456' },
    })
    fireEvent.click(screen.getByRole('button', { name: /confirmar e ativar/i }))

    await waitFor(() => expect(screen.getByText(/2FA ativo/i)).toBeInTheDocument())
    expect(mockedPost.mock.calls.at(-1)?.[0]).toBe('/api/v1/auth/mfa/confirm')
    expect(mockedPost.mock.calls.at(-1)?.[1]).toMatchObject({ code: '123456' })
  })

  it('sem usuário redireciona para /entrar', () => {
    useAuthStore.setState({ user: null })
    const { container } = renderPage()
    // Navigate substitui o conteúdo — a página do 2FA não renderiza.
    expect(container.querySelector('h1')).toBeNull()
  })
})
