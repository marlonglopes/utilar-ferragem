import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { Navbar } from '@/components/layout/Navbar'
import HomePage from '@/pages/home/HomePage'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

const testQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={testQueryClient}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          {children}
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

describe('Smoke test', () => {
  it('renders the home page without crashing', () => {
    render(<HomePage />, { wrapper })
    expect(screen.getByText(/sprint 02/i)).toBeInTheDocument()
  })

  it('Navbar displays the UtiLar brand link', () => {
    render(<Navbar />, { wrapper })
    expect(screen.getByRole('link', { name: /utilar ferragem/i })).toBeInTheDocument()
  })

  it('Navbar displays the tagline in pt-BR', () => {
    render(<Navbar />, { wrapper })
    expect(screen.getByText(/solução em ferragem/i)).toBeInTheDocument()
  })
})
