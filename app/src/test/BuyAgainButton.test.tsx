import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { describe, it, expect, beforeEach, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { BuyAgainButton } from '@/components/orders/BuyAgainButton'
import { useCartStore } from '@/store/cartStore'
import { MOCK_PRODUCTS } from '@/lib/mockProducts'
import type { Order, OrderItem } from '@/lib/mockOrders'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

beforeEach(() => {
  localStorage.clear()
  useCartStore.setState({ items: [] })
})

/** Produto que existe no catálogo mock, com estoque de sobra. */
const CIMENTO = MOCK_PRODUCTS.find((p) => p.id === '9')!
/** Últimas 3 unidades — usado pro caso de quantidade reduzida. */
const ESMERILHADEIRA = MOCK_PRODUCTS.find((p) => p.id === '5')!

function item(over: Partial<OrderItem> = {}): OrderItem {
  return {
    productId: '9',
    name: 'Cimento CP II 50kg',
    icon: '◫',
    sellerId: 's1',
    sellerName: 'Casa & Obra',
    quantity: 2,
    unitPrice: 38.5,
    ...over,
  }
}

function order(items: OrderItem[]): Order {
  return {
    id: 'o1',
    number: '2026-500',
    status: 'delivered',
    paymentMethod: 'pix',
    paymentInfo: 'Pix',
    items,
    subtotal: 100,
    shippingCost: 0,
    total: 100,
    address: {
      street: 'Rua A',
      number: '1',
      neighborhood: 'Centro',
      city: 'São Paulo',
      state: 'SP',
      cep: '01000-000',
    },
    createdAt: '2026-04-10T10:00:00Z',
    updatedAt: '2026-04-10T10:00:00Z',
  }
}

function renderButton(o: Order) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <BuyAgainButton order={o} />
      </MemoryRouter>
    </I18nextProvider>
  )
}

async function clickBuyAgain() {
  fireEvent.click(screen.getByRole('button', { name: /comprar novamente/i }))
  await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument(), { timeout: 3000 })
  return screen.getByRole('dialog')
}

describe('BuyAgainButton — caminho feliz', () => {
  it('repõe os itens no carrinho e confirma', async () => {
    renderButton(order([item({ productId: '9', quantity: 3 })]))
    const dialog = await clickBuyAgain()

    expect(within(dialog).getByText(/tudo certo/i)).toBeInTheDocument()
    expect(useCartStore.getState().items).toHaveLength(1)
    expect(useCartStore.getState().items[0].quantity).toBe(3)
  }, 8000)

  it('usa o preço ATUAL do catálogo, nunca o preço antigo do pedido', async () => {
    // Repor o preço histórico mostraria um total que o servidor recusa no
    // checkout (o order-service resolve preço do lado dele) e o cliente veria
    // o valor "subir sozinho" na última tela.
    renderButton(order([item({ productId: '9', unitPrice: 1 })]))
    await clickBuyAgain()

    expect(useCartStore.getState().items[0].priceSnapshot).toBe(CIMENTO.price)
  }, 8000)

  it('leva o cliente ao carrinho', async () => {
    renderButton(order([item({ productId: '9' })]))
    const dialog = await clickBuyAgain()

    expect(within(dialog).getByRole('link', { name: /ir para o carrinho/i })).toHaveAttribute(
      'href',
      '/carrinho'
    )
  }, 8000)
})

describe('BuyAgainButton — item indisponível', () => {
  /**
   * REGRESSÃO: adicionar menos itens do que o cliente pediu, em silêncio.
   *
   * Era o comportamento antigo: o botão jogava todos os itens do pedido no
   * carrinho com `stock: 99` fixo e quantidade 1, sem consultar o catálogo, e
   * navegava direto pro carrinho. Produto fora de linha entrava do mesmo jeito.
   * O cliente só descobria a falta na obra.
   */
  it('mostra "X de Y adicionados" e discrimina o que faltou', async () => {
    renderButton(
      order([
        item({ productId: '9', name: 'Cimento' }),
        item({ productId: '31', name: 'Fita veda rosca' }),
        item({ productId: 'p-broca-antiga', name: 'Broca fora de linha' }),
      ])
    )
    const dialog = await clickBuyAgain()

    expect(within(dialog).getByText(/2 de 3 itens adicionados/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/1 fora de linha/i)).toBeInTheDocument()

    // Só os disponíveis entraram no carrinho.
    expect(useCartStore.getState().items.map((i) => i.productId).sort()).toEqual(['31', '9'])
  }, 8000)

  it('lista o resultado item a item, nomeando o que não entrou', async () => {
    renderButton(
      order([
        item({ productId: '9', name: 'Cimento' }),
        item({ productId: 'sumiu', name: 'Broca fora de linha' }),
      ])
    )
    const dialog = await clickBuyAgain()

    // O resumo diz QUANTOS; a lista diz QUAIS — sem isso o cliente não sabe o
    // que precisa procurar de novo.
    expect(within(dialog).getByText('Broca fora de linha')).toBeInTheDocument()
    expect(within(dialog).getByText('Fora de linha')).toBeInTheDocument()
    expect(within(dialog).getByText('Adicionado')).toBeInTheDocument()
  }, 8000)

  it('reduz a quantidade ao estoque e explica a redução', async () => {
    renderButton(order([item({ productId: '5', name: 'Esmerilhadeira', quantity: 10 })]))
    const dialog = await clickBuyAgain()

    expect(
      within(dialog).getByText(new RegExp(`só ${ESMERILHADEIRA.stock} de 10`, 'i'))
    ).toBeInTheDocument()
    expect(useCartStore.getState().items[0].quantity).toBe(ESMERILHADEIRA.stock)
  }, 8000)

  it('quando nada pode ser reposto, diz isso e não oferece o carrinho', async () => {
    renderButton(order([item({ productId: 'sumiu-1' }), item({ productId: 'sumiu-2' })]))
    const dialog = await clickBuyAgain()

    expect(within(dialog).getByText(/nenhum item deste pedido/i)).toBeInTheDocument()
    expect(within(dialog).queryByRole('link', { name: /ir para o carrinho/i })).not.toBeInTheDocument()
    expect(useCartStore.getState().items).toHaveLength(0)
  }, 8000)

  it('avisa quando o preço mudou desde a compra', async () => {
    // Pedido antigo com cimento a R$ 20; catálogo mock cobra outro valor.
    renderButton(order([item({ productId: '9', unitPrice: 20 })]))
    const dialog = await clickBuyAgain()

    expect(within(dialog).getByText(/mudou de preço desde a sua compra/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/Era R\$ 20,00/)).toBeInTheDocument()
  }, 8000)
})

describe('BuyAgainButton — interação', () => {
  it('mostra estado de carregamento enquanto consulta o catálogo', async () => {
    renderButton(order([item({ productId: '9' })]))
    const btn = screen.getByRole('button', { name: /comprar novamente/i })
    fireEvent.click(btn)

    expect(screen.getByRole('button', { name: /verificando disponibilidade/i })).toBeDisabled()
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument(), { timeout: 3000 })
  }, 8000)

  it('o resumo é bloqueante, não some sozinho', async () => {
    renderButton(order([item({ productId: '9' }), item({ productId: 'sumiu' })]))
    const dialog = await clickBuyAgain()

    // Um toast de 4 segundos não serve: a informação de que faltou item precisa
    // ser reconhecida pelo cliente antes de ele seguir.
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    fireEvent.click(within(dialog).getByRole('button', { name: /continuar aqui/i }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  }, 8000)

  it('não navega ao ser clicado dentro de um card com link esticado', async () => {
    let navegou = false
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <div onClick={() => { navegou = true }}>
            <BuyAgainButton order={order([item({ productId: '9' })])} variant="compact" />
          </div>
        </MemoryRouter>
      </I18nextProvider>
    )

    fireEvent.click(screen.getByRole('button', { name: /comprar novamente/i }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument(), { timeout: 3000 })

    // Na lista de pedidos o card inteiro é clicável. Sem stopPropagation,
    // recomprar abriria o detalhe do pedido no mesmo clique.
    expect(navegou).toBe(false)
  }, 8000)
})
