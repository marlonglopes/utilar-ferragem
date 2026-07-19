import { render, screen, within } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { OrderTimeline } from '@/components/orders/OrderTimeline'
import type { Order } from '@/lib/mockOrders'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function order(over: Partial<Order> = {}): Order {
  return {
    id: 'o1',
    number: '2026-100',
    status: 'paid',
    paymentMethod: 'pix',
    paymentInfo: 'Pix',
    items: [],
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
    ...over,
  }
}

function renderTimeline(o: Order) {
  return render(
    <I18nextProvider i18n={i18n}>
      <OrderTimeline order={o} />
    </I18nextProvider>
  )
}

describe('OrderTimeline — etapas e datas', () => {
  it('mostra as cinco etapas do pedido', () => {
    renderTimeline(order({ status: 'shipped' }))
    const steps = screen.getAllByRole('listitem')
    expect(steps).toHaveLength(5)

    expect(screen.getByText(/Pedido feito/)).toBeInTheDocument()
    expect(screen.getByText(/Pagamento confirmado/)).toBeInTheDocument()
    expect(screen.getByText(/Separando pedido/)).toBeInTheDocument()
    expect(screen.getByText(/A caminho/)).toBeInTheDocument()
    expect(screen.getByText(/Entregue/)).toBeInTheDocument()
  })

  it('exibe a data de cada etapa concluída', () => {
    renderTimeline(
      order({
        status: 'shipped',
        createdAt: '2026-04-10T13:00:00Z',
        paidAt: '2026-04-10T14:30:00Z',
        pickedAt: '2026-04-11T12:00:00Z',
        shippedAt: '2026-04-12T09:00:00Z',
      })
    )

    // Data legível por etapa é o que responde "cadê meu pedido" sem chamado.
    // "Pedido feito" e "Pagamento confirmado" caem no mesmo dia — daí getAll.
    expect(screen.getAllByText(/10\/04\/2026/).length).toBe(2)
    expect(screen.getByText(/11\/04\/2026/)).toBeInTheDocument()
    expect(screen.getByText(/12\/04\/2026/)).toBeInTheDocument()

    // E cada etapa concluída carrega a SUA data, não uma data única do pedido.
    const steps = screen.getAllByRole('listitem')
    expect(within(steps[2]).getByText(/11\/04\/2026/)).toBeInTheDocument()
    expect(within(steps[3]).getByText(/12\/04\/2026/)).toBeInTheDocument()
  })

  it('etapa futura não mostra data nenhuma', () => {
    renderTimeline(order({ status: 'paid', createdAt: '2026-04-10T13:00:00Z', paidAt: '2026-04-10T14:00:00Z' }))

    const entregue = screen.getAllByRole('listitem')[4]
    expect(within(entregue).getByText(/Entregue/)).toBeInTheDocument()
    expect(within(entregue).queryByText(/\d{2}\/\d{2}\/\d{4}/)).not.toBeInTheDocument()
  })
})

describe('OrderTimeline — etapas faltando', () => {
  /**
   * REGRESSÃO: sumir com uma etapa que não tem carimbo de data.
   *
   * Pedido importado de sistema antigo, webhook perdido ou migração de schema
   * chegam com `pickedAt`/`paidAt` nulos mesmo tendo passado pela etapa. Se a
   * UI escondesse a etapa sem data, um pedido JÁ ENVIADO apareceria pulando a
   * separação — como se a loja tivesse despachado sem separar. A etapa fica,
   * com "Data não informada" no lugar do carimbo.
   */
  it('mantém a etapa e avisa que a data não veio', () => {
    renderTimeline(
      order({
        status: 'shipped',
        createdAt: '2026-04-10T13:00:00Z',
        paidAt: undefined, // webhook perdido
        pickedAt: undefined, // pedido migrado de sistema antigo
        shippedAt: '2026-04-12T09:00:00Z',
      })
    )

    expect(screen.getAllByRole('listitem')).toHaveLength(5)
    expect(screen.getByText(/Pagamento confirmado/)).toBeInTheDocument()
    expect(screen.getByText(/Separando pedido/)).toBeInTheDocument()
    // Duas etapas concluídas sem carimbo → dois avisos.
    expect(screen.getAllByText('Data não informada')).toHaveLength(2)
  })

  it('pedido sem nenhuma data além da criação ainda renderiza a linha inteira', () => {
    renderTimeline(order({ status: 'delivered', createdAt: '2026-04-10T13:00:00Z' }))

    expect(screen.getAllByRole('listitem')).toHaveLength(5)
    // 4 etapas concluídas sem data (pago, separando, enviado, entregue)
    expect(screen.getAllByText('Data não informada')).toHaveLength(4)
  })
})

describe('OrderTimeline — acessibilidade', () => {
  it('é uma lista ordenada com rótulo', () => {
    renderTimeline(order({ status: 'paid' }))
    expect(screen.getByRole('list', { name: /andamento do pedido/i })).toBeInTheDocument()
  })

  it('marca a etapa atual com aria-current para o leitor de tela', () => {
    renderTimeline(order({ status: 'picking' }))

    const current = screen.getAllByRole('listitem').filter(
      (li) => li.getAttribute('aria-current') === 'step'
    )
    expect(current).toHaveLength(1)
    expect(within(current[0]).getByText(/Separando pedido/)).toBeInTheDocument()
  })

  it('o estado de cada etapa existe em texto, não só na cor da bolinha', () => {
    renderTimeline(order({ status: 'picking' }))

    // Quem não enxerga a bolinha verde precisa ouvir "concluído".
    expect(screen.getAllByText(/— Concluído/).length).toBeGreaterThan(0)
    expect(screen.getByText(/— Etapa atual/)).toBeInTheDocument()
    expect(screen.getAllByText(/— Ainda não iniciado/).length).toBeGreaterThan(0)
  })
})

describe('OrderTimeline — rastreio', () => {
  it('mostra o código de rastreio ancorado no passo de envio', () => {
    renderTimeline(
      order({ status: 'shipped', shippedAt: '2026-04-12T09:00:00Z', trackingCode: 'BR123456789SP' })
    )

    expect(screen.getByText('BR123456789SP')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /rastrear/i })
    expect(link).toHaveAttribute('href', expect.stringContaining('BR123456789SP'))
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'))
  })

  it('não mostra rastreio antes do envio', () => {
    renderTimeline(order({ status: 'paid', trackingCode: 'BR123456789SP' }))
    expect(screen.queryByText('BR123456789SP')).not.toBeInTheDocument()
  })

  it('pedido enviado sem código não mostra bloco de rastreio vazio', () => {
    renderTimeline(order({ status: 'shipped', shippedAt: '2026-04-12T09:00:00Z' }))
    expect(screen.queryByRole('link', { name: /rastrear/i })).not.toBeInTheDocument()
  })
})

describe('OrderTimeline — cancelado', () => {
  it('substitui a linha do tempo pelo aviso de cancelamento', () => {
    renderTimeline(order({ status: 'cancelled', cancelledAt: '2026-04-11T10:00:00Z' }))

    expect(screen.getByText('Pedido cancelado')).toBeInTheDocument()
    expect(screen.getByText(/11\/04\/2026/)).toBeInTheDocument()
    // Não faz sentido mostrar "A caminho" num pedido cancelado.
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('cancelado sem data usa a última atualização em vez de ficar em branco', () => {
    renderTimeline(
      order({ status: 'cancelled', cancelledAt: undefined, updatedAt: '2026-04-15T08:00:00Z' })
    )
    expect(screen.getByText(/15\/04\/2026/)).toBeInTheDocument()
  })
})
