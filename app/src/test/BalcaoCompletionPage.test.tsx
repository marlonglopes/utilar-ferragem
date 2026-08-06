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

describe('BalcaoCompletionPage — conclui venda aprovada', () => {
  it('maquininha: exige NSU e conclui (some da fila)', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('BAL-0042')).toBeInTheDocument())
    expect(screen.getByText('Construtora Aurora')).toBeInTheDocument()

    // Escolhe maquininha → campo de NSU aparece.
    fireEvent.click(screen.getByRole('button', { name: /Maquininha/i }))
    const concluir = screen.getByRole('button', { name: /^Concluir$/i })
    expect(concluir).toBeDisabled()

    // NSU habilita e conclui → venda sai da fila (mock).
    fireEvent.change(screen.getByPlaceholderText('Ex: 004512890'), {
      target: { value: '004512890' },
    })
    expect(concluir).not.toBeDisabled()
    fireEvent.click(concluir)

    await waitFor(() =>
      expect(screen.getByText(/Nenhuma venda aprovada aguardando cobrança/i)).toBeInTheDocument()
    )
  })

  it('pix: gera o QR e confirma', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('BAL-0042')).toBeInTheDocument())

    // Escolhe Pix → mostra o QR / instrução.
    fireEvent.click(screen.getByRole('button', { name: /^Pix$/i }))
    await waitFor(() => expect(screen.getByText(/Mostre o QR ao cliente/i)).toBeInTheDocument())

    // Em mock, dá pra simular a confirmação → "Pagamento confirmado".
    fireEvent.click(screen.getByText(/simular confirmação/i))
    await waitFor(() => expect(screen.getByText(/Pagamento confirmado/i)).toBeInTheDocument())
  })
})
