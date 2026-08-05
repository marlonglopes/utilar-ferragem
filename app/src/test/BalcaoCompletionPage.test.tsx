import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, it, expect } from 'vitest'
import BalcaoCompletionPage from '@/pages/balcao/BalcaoCompletionPage'

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <BalcaoCompletionPage />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('BalcaoCompletionPage — conclui venda aprovada na maquininha', () => {
  it('lista a venda aprovada (mock), exige NSU e conclui', async () => {
    renderPage()

    // A venda aprovada aparece (mock).
    await waitFor(() => expect(screen.getByText('BAL-0042')).toBeInTheDocument())
    expect(screen.getByText('Construtora Aurora')).toBeInTheDocument()

    // Sem NSU, o botão "Concluir" fica desabilitado.
    const btn = screen.getByRole('button', { name: /Concluir na maquininha/i })
    expect(btn).toBeDisabled()

    // Digita o NSU → habilita → conclui.
    fireEvent.change(screen.getByPlaceholderText('Ex: 004512890'), {
      target: { value: '004512890' },
    })
    expect(btn).not.toBeDisabled()
    fireEvent.click(btn)

    // Após concluir (mock), some da fila → estado vazio.
    await waitFor(() =>
      expect(screen.getByText(/Nenhuma venda aprovada aguardando cobrança/i)).toBeInTheDocument()
    )
  })
})
