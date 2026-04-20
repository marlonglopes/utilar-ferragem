import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import HomePage from '@/pages/home/HomePage'

const testQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={testQueryClient}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Smoke test', () => {
  it('renders the home page without crashing', () => {
    render(<HomePage />, { wrapper })
    expect(screen.getByText(/sprint 01/i)).toBeInTheDocument()
  })

  it('displays the UtiLar brand', () => {
    render(<HomePage />, { wrapper })
    expect(screen.getByText('Uti')).toBeInTheDocument()
    expect(screen.getByText('Lar')).toBeInTheDocument()
  })

  it('displays the tagline', () => {
    render(<HomePage />, { wrapper })
    expect(screen.getByText(/solução em ferragem/i)).toBeInTheDocument()
  })
})
