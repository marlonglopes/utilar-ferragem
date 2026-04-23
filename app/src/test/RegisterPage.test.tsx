import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import RegisterPage from '@/pages/auth/RegisterPage'
import { useAuthStore } from '@/store/authStore'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
  useAuthStore.setState({ user: null })
})

function renderRegisterPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <RegisterPage />
      </MemoryRouter>
    </I18nextProvider>
  )
}

describe('RegisterPage', () => {
  it('renders all required fields', () => {
    renderRegisterPage()
    expect(screen.getByLabelText(/nome completo/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/e-mail/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^cpf$/i)).toBeInTheDocument()
    expect(screen.getAllByLabelText(/senha/i).length).toBeGreaterThan(0)
  })

  it('shows error when passwords do not match', async () => {
    renderRegisterPage()
    fillForm({ password: 'minhasenha123', passwordConfirm: 'outrasenha123' })
    fireEvent.click(screen.getByRole('button', { name: /criar conta/i }))
    await waitFor(() => {
      expect(screen.getByText(/senhas não coincidem/i)).toBeInTheDocument()
    })
  })

  it('shows error when password is too short', async () => {
    renderRegisterPage()
    fillForm({ password: 'curta', passwordConfirm: 'curta' })
    fireEvent.click(screen.getByRole('button', { name: /criar conta/i }))
    await waitFor(() => {
      expect(screen.getByText(/mínimo de 10/i)).toBeInTheDocument()
    })
  })

  it('shows error for invalid CPF', async () => {
    renderRegisterPage()
    fillForm({ cpf: '000.000.000-00' })
    fireEvent.click(screen.getByRole('button', { name: /criar conta/i }))
    await waitFor(() => {
      expect(screen.getByText(/cpf inválido/i)).toBeInTheDocument()
    })
  })

  it('shows LGPD error if checkbox not checked', async () => {
    renderRegisterPage()
    fillForm()
    fireEvent.click(screen.getByRole('button', { name: /criar conta/i }))
    await waitFor(() => {
      expect(screen.getByText(/precisa aceitar/i)).toBeInTheDocument()
    })
  })

  it('submits successfully with valid data (stub/no API)', async () => {
    renderRegisterPage()
    fillForm()
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: /criar conta/i }))
    await waitFor(() => {
      expect(useAuthStore.getState().user?.email).toBe('teste@example.com')
    })
  })
})

function fillForm(overrides: Record<string, string> = {}) {
  const vals = {
    name: 'Maria Souza',
    email: 'teste@example.com',
    cpf: '529.982.247-25',
    phone: '11999998888',
    password: 'minhasenha123',
    passwordConfirm: 'minhasenha123',
    ...overrides,
  }
  const byLabel = (label: RegExp) => screen.getByLabelText(label)
  if (vals.name) fireEvent.change(byLabel(/nome completo/i), { target: { value: vals.name } })
  if (vals.email) fireEvent.change(byLabel(/e-mail/i), { target: { value: vals.email } })
  if (vals.cpf) fireEvent.change(byLabel(/^cpf$/i), { target: { value: vals.cpf } })
  const passwordFields = screen.getAllByLabelText(/senha/i)
  if (vals.password) fireEvent.change(passwordFields[0], { target: { value: vals.password } })
  if (vals.passwordConfirm) fireEvent.change(passwordFields[1], { target: { value: vals.passwordConfirm } })
}
