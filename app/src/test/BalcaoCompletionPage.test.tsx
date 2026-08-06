import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, it, expect } from 'vitest'
import i18n from '@/i18n'
import BalcaoCompletionPage from '@/pages/balcao/BalcaoCompletionPage'

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MemoryRouter>
          <BalcaoCompletionPage />
        </MemoryRouter>
      </I18nextProvider>
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

  it('cartão: mostra o formulário (CardPayment) e confirma pela venda aprovada', async () => {
    renderPage()
    await waitFor(() => expect(screen.getByText('BAL-0042')).toBeInTheDocument())

    // Escolhe Cartão → renderiza o CardPayment (mesmo da web); em mock, "simular".
    fireEvent.click(screen.getByRole('button', { name: /^Cartão$/i }))
    await waitFor(() => expect(screen.getByText(/Simular aprovação/i)).toBeInTheDocument())

    fireEvent.click(screen.getByText(/Simular aprovação/i))
    await waitFor(() => expect(screen.getByText(/Pagamento confirmado/i)).toBeInTheDocument())
  })
})
