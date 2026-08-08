import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BalcaoPage from '@/pages/balcao/BalcaoPage'
import {
  createComanda,
  useBalcaoStore,
  selectActiveComanda,
  MOCK_OPERATOR,
} from '@/store/balcaoStore'

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
  useBalcaoStore.setState({
    comandas: [fresh],
    activeId: fresh.id,
    // Teto de 12% como `GET /api/v1/store/me` entregaria para um operador.
    operator: { ...MOCK_OPERATOR, fromBackend: true },
  })
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

    const tile = await screen.findByRole('button', {
      name: /furadeira de impacto bosch gsb 13 re/i,
    })
    await user.click(tile)

    await waitFor(() => {
      expect(selectActiveComanda(useBalcaoStore.getState()).items).toHaveLength(1)
    })

    const panel = screen
      .getByRole('heading', { name: /pedido do balcão/i })
      .closest('div')!.parentElement!
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
      costIsEstimated: false,
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

  // MOBILE: a barra inferior (que abre a gaveta do pedido) precisa ser óbvia e
  // acionável — o vendedor reclamou que "não achava" como fechar a venda no
  // celular. Rótulo muda conforme há itens e o clique abre a gaveta.
  it('a barra inferior mostra o rótulo certo e abre a gaveta do pedido', async () => {
    const user = userEvent.setup()
    render(<BalcaoPage />, { wrapper })

    const bar = screen.getByRole('button', { name: /abrir o pedido do balcão/i })
    // Vazia: rótulo curto "Pedido"; não há diálogo aberto ainda.
    expect(bar).toHaveTextContent(/pedido/i)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    // Com item, o rótulo vira ação.
    useBalcaoStore.getState().addItem({
      productId: 'p1',
      sku: 'FER-00001',
      name: 'Item teste',
      icon: '⚒',
      unit: 'un',
      unitPrice: 100,
      unitCost: 60,
      costIsEstimated: false,
      quantity: 1,
      stock: 5,
    })
    await waitFor(() => expect(bar).toHaveTextContent(/ver pedido e cobrar/i))

    // Tocar na barra abre a gaveta (diálogo) com o pedido.
    await user.click(bar)
    expect(await screen.findByRole('dialog', { name: /pedido do balcão/i })).toBeInTheDocument()
  })

  // Regressão da queixa do vendedor: no celular, adicionar o 1º item tem que
  // ABRIR a comanda sozinha (senão o pedido nasce escondido na gaveta). Só em
  // tela estreita — no desktop o painel já fica fixo ao lado.
  it('no celular, adicionar o 1º item abre a comanda automaticamente', async () => {
    const realMatchMedia = window.matchMedia
    // Simula tela estreita (< lg): matchMedia('(max-width: 1023px)') = match.
    window.matchMedia = ((q: string) => ({
      matches: true,
      media: q,
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent() {
        return false
      },
    })) as unknown as typeof window.matchMedia
    try {
      render(<BalcaoPage />, { wrapper })
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

      useBalcaoStore.getState().addItem({
        productId: 'p1',
        sku: 'FER-00001',
        name: 'Item teste',
        icon: '⚒',
        unit: 'un',
        unitPrice: 100,
        unitCost: 60,
        costIsEstimated: false,
        quantity: 1,
        stock: 5,
      })

      expect(await screen.findByRole('dialog', { name: /pedido do balcão/i })).toBeInTheDocument()
    } finally {
      window.matchMedia = realMatchMedia
    }
  })

  // O vendedor precisa gerenciar os itens da comanda: remover pelo botão claro
  // "Remover" (antes era só uma lixeira cinza que dependia de hover — invisível
  // no toque).
  it('remove um item da comanda pelo botão Remover', async () => {
    const user = userEvent.setup()
    render(<BalcaoPage />, { wrapper })

    useBalcaoStore.getState().addItem({
      productId: 'p1',
      sku: 'FER-00001',
      name: 'Item teste',
      icon: '⚒',
      unit: 'un',
      unitPrice: 100,
      unitCost: 60,
      costIsEstimated: false,
      quantity: 1,
      stock: 5,
    })
    await waitFor(() => {
      expect(selectActiveComanda(useBalcaoStore.getState()).items).toHaveLength(1)
    })

    await user.click(screen.getByRole('button', { name: /remover item teste/i }))

    await waitFor(() => {
      expect(selectActiveComanda(useBalcaoStore.getState()).items).toHaveLength(0)
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
