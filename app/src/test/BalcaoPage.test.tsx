import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BalcaoPage from '@/pages/balcao/BalcaoPage'
import { createComanda, useBalcaoStore, selectActiveComanda } from '@/store/balcaoStore'

function wrapper({ children }: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        {children}
      </MemoryRouter>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  const fresh = createComanda('Comanda 1')
  useBalcaoStore.setState({ comandas: [fresh], activeId: fresh.id, role: 'operator' })
})

describe('BalcaoPage', () => {
  it('renderiza o chrome do PDV: badge, busca e painel do pedido', async () => {
    render(<BalcaoPage />, { wrapper })

    expect(screen.getByText('Balcão')).toBeInTheDocument()
    expect(
      screen.getByRole('searchbox', { name: /buscar produto por nome, sku ou código de barras/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /pedido do balcão/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /escanear/i })).toBeInTheDocument()
  })

  it('mostra o estado vazio e o botão Cobrar desabilitado', () => {
    render(<BalcaoPage />, { wrapper })

    expect(screen.getByText('Nenhum item')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^cobrar/i })).toBeDisabled()
  })

  it('lista produtos do catálogo (modo mock) e adiciona um à comanda', async () => {
    const user = userEvent.setup()
    render(<BalcaoPage />, { wrapper })

    const tile = await screen.findByRole('button', { name: /furadeira de impacto bosch gsb 13 re/i })
    await user.click(tile)

    await waitFor(() => {
      expect(selectActiveComanda(useBalcaoStore.getState()).items).toHaveLength(1)
    })

    const panel = screen.getByRole('heading', { name: /pedido do balcão/i }).closest('div')!
      .parentElement!
    expect(within(panel).getByText(/1 item/i)).toBeInTheDocument()
  })

  it('o bloco de negociação reage ao desconto e avisa acima do teto do cargo', async () => {
    render(<BalcaoPage />, { wrapper })

    // Um item de 100 com custo 60 → margem base 40%.
    useBalcaoStore.getState().addItem({
      productId: 'p1',
      sku: 'FER-00001',
      name: 'Item teste',
      icon: '⚒',
      unit: 'un',
      unitPrice: 100,
      unitCost: 60,
      quantity: 1,
      stock: 5,
    })

    // Dentro do teto (12%) — sem aviso de aprovação.
    useBalcaoStore.getState().setDiscountPct(10)
    await waitFor(() => {
      expect(screen.getByText(/dentro do seu limite de desconto/i)).toBeInTheDocument()
    })

    // Acima do teto — avisa aprovação do gerente, mas não bloqueia.
    useBalcaoStore.getState().setDiscountPct(20)
    await waitFor(() => {
      expect(screen.getByText(/pendente de aprovação do gerente/i)).toBeInTheDocument()
    })

    // Abaixo do custo — alerta bloqueante.
    useBalcaoStore.getState().setDiscountPct(60)
    await waitFor(() => {
      expect(screen.getByText(/vende abaixo do custo/i)).toBeInTheDocument()
    })
  })

  it('busca por SKU encontra o produto via filtro no dispositivo', async () => {
    const user = userEvent.setup()
    render(<BalcaoPage />, { wrapper })

    const search = screen.getByRole('searchbox', {
      name: /buscar produto por nome, sku ou código de barras/i,
    })
    await user.type(search, 'FER-00001')

    expect(await screen.findByText(/buscando por sku/i)).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getAllByText('FER-00001').length).toBeGreaterThan(0)
    })
  })

  it('permite abrir uma segunda comanda', async () => {
    const user = userEvent.setup()
    render(<BalcaoPage />, { wrapper })

    await user.click(screen.getByRole('button', { name: /nova comanda/i }))

    expect(useBalcaoStore.getState().comandas).toHaveLength(2)
    // `name` exato: "Fechar Comanda 2" também casaria com um regex.
    expect(screen.getByRole('button', { name: 'Comanda 2' })).toBeInTheDocument()
  })
})
