import { describe, it, expect, beforeEach } from 'vitest'
import {
  useBalcaoStore,
  createComanda,
  selectActiveComanda,
  balcaoPricingOf,
  type NewBalcaoItem,
  type BalcaoCustomer,
} from '@/store/balcaoStore'

function makeItem(overrides: Partial<NewBalcaoItem> = {}): NewBalcaoItem {
  return {
    productId: 'prod-1',
    sku: 'FER-00001',
    barcode: '7890000010000',
    name: 'Furadeira Bosch GSB 13 RE',
    icon: '⚒',
    unit: 'un',
    unitPrice: 100,
    unitCost: 60,
    quantity: 1,
    stock: 10,
    ...overrides,
  }
}

const CUSTOMER: BalcaoCustomer = {
  name: 'João da Obra',
  document: '12345678901',
  phone: '11999990000',
  segment: 'atacado',
}

function active() {
  return selectActiveComanda(useBalcaoStore.getState())
}

beforeEach(() => {
  const fresh = createComanda('Comanda 1')
  useBalcaoStore.setState({ comandas: [fresh], activeId: fresh.id, role: 'operator' })
})

describe('balcaoStore — itens', () => {
  it('adiciona um item novo', () => {
    useBalcaoStore.getState().addItem(makeItem())
    expect(active().items).toHaveLength(1)
    expect(active().items[0].name).toBe('Furadeira Bosch GSB 13 RE')
    expect(active().items[0].addedAt).toBeTruthy()
  })

  it('somar o mesmo produto acumula a quantidade em vez de duplicar a linha', () => {
    useBalcaoStore.getState().addItem(makeItem({ quantity: 2 }))
    useBalcaoStore.getState().addItem(makeItem({ quantity: 3 }))
    expect(active().items).toHaveLength(1)
    expect(active().items[0].quantity).toBe(5)
  })

  it('nunca passa do estoque ao acumular', () => {
    useBalcaoStore.getState().addItem(makeItem({ quantity: 8, stock: 10 }))
    useBalcaoStore.getState().addItem(makeItem({ quantity: 8, stock: 10 }))
    expect(active().items[0].quantity).toBe(10)
  })

  it('remove um item', () => {
    useBalcaoStore.getState().addItem(makeItem())
    useBalcaoStore.getState().addItem(makeItem({ productId: 'prod-2' }))
    useBalcaoStore.getState().removeItem('prod-1')
    expect(active().items).toHaveLength(1)
    expect(active().items[0].productId).toBe('prod-2')
  })

  it('setQuantity respeita o teto de estoque', () => {
    useBalcaoStore.getState().addItem(makeItem({ stock: 4 }))
    useBalcaoStore.getState().setQuantity('prod-1', 99)
    expect(active().items[0].quantity).toBe(4)
  })

  it('setQuantity com 0 ou menos remove a linha', () => {
    useBalcaoStore.getState().addItem(makeItem())
    useBalcaoStore.getState().setQuantity('prod-1', 0)
    expect(active().items).toHaveLength(0)
  })

  it('increment e decrement andam de um em um', () => {
    useBalcaoStore.getState().addItem(makeItem({ quantity: 2 }))
    useBalcaoStore.getState().incrementItem('prod-1')
    expect(active().items[0].quantity).toBe(3)
    useBalcaoStore.getState().decrementItem('prod-1')
    expect(active().items[0].quantity).toBe(2)
  })

  it('decrement até zero remove o item', () => {
    useBalcaoStore.getState().addItem(makeItem({ quantity: 1 }))
    useBalcaoStore.getState().decrementItem('prod-1')
    expect(active().items).toHaveLength(0)
  })

  it('ignora operações em produto inexistente', () => {
    useBalcaoStore.getState().incrementItem('nao-existe')
    useBalcaoStore.getState().removeItem('nao-existe')
    expect(active().items).toHaveLength(0)
  })
})

describe('balcaoStore — negociação e cliente', () => {
  it('define e limita o desconto', () => {
    useBalcaoStore.getState().setDiscountPct(15)
    expect(active().discountPct).toBe(15)
    useBalcaoStore.getState().setDiscountPct(-10)
    expect(active().discountPct).toBe(0)
    useBalcaoStore.getState().setDiscountPct(500)
    expect(active().discountPct).toBe(100)
  })

  it('define e remove o cliente', () => {
    useBalcaoStore.getState().setCustomer(CUSTOMER)
    expect(active().customer?.name).toBe('João da Obra')
    useBalcaoStore.getState().setCustomer(null)
    expect(active().customer).toBeNull()
  })

  it('clearComanda zera itens, desconto e cliente', () => {
    useBalcaoStore.getState().addItem(makeItem())
    useBalcaoStore.getState().setDiscountPct(10)
    useBalcaoStore.getState().setCustomer(CUSTOMER)

    useBalcaoStore.getState().clearComanda()

    expect(active().items).toHaveLength(0)
    expect(active().discountPct).toBe(0)
    expect(active().customer).toBeNull()
  })

  it('precificação da comanda ativa reflete itens e desconto', () => {
    useBalcaoStore.getState().addItem(makeItem({ quantity: 2 })) // 200, custo 120
    useBalcaoStore.getState().setDiscountPct(10)

    const pricing = balcaoPricingOf(useBalcaoStore.getState())
    expect(pricing.subtotal).toBe(200)
    expect(pricing.total).toBe(180)
    expect(pricing.cost).toBe(120)
    expect(pricing.ceilingPct).toBe(12) // cargo operator
  })

  it('o cargo do store alimenta o teto usado no cálculo', () => {
    useBalcaoStore.getState().addItem(makeItem())
    useBalcaoStore.getState().setDiscountPct(18)
    expect(balcaoPricingOf(useBalcaoStore.getState()).requiresApproval).toBe(true)

    useBalcaoStore.getState().setRole('supervisor')
    expect(balcaoPricingOf(useBalcaoStore.getState()).requiresApproval).toBe(false)
  })
})

describe('balcaoStore — comandas', () => {
  it('abre uma nova comanda e a torna ativa', () => {
    const firstId = useBalcaoStore.getState().activeId
    const newId = useBalcaoStore.getState().openComanda()

    expect(useBalcaoStore.getState().comandas).toHaveLength(2)
    expect(useBalcaoStore.getState().activeId).toBe(newId)
    expect(newId).not.toBe(firstId)
  })

  it('comandas são isoladas entre si', () => {
    useBalcaoStore.getState().addItem(makeItem())
    const firstId = useBalcaoStore.getState().activeId

    useBalcaoStore.getState().openComanda()
    expect(active().items).toHaveLength(0)

    useBalcaoStore.getState().addItem(makeItem({ productId: 'prod-9' }))
    expect(active().items[0].productId).toBe('prod-9')

    useBalcaoStore.getState().setActiveComanda(firstId)
    expect(active().items[0].productId).toBe('prod-1')
  })

  it('renumera os rótulos ao abrir', () => {
    useBalcaoStore.getState().openComanda()
    expect(useBalcaoStore.getState().comandas.map((c) => c.label)).toEqual([
      'Comanda 1',
      'Comanda 2',
    ])
  })

  it('fechar a comanda ativa move o foco para outra', () => {
    const firstId = useBalcaoStore.getState().activeId
    const secondId = useBalcaoStore.getState().openComanda()

    useBalcaoStore.getState().closeComanda(secondId)

    expect(useBalcaoStore.getState().comandas).toHaveLength(1)
    expect(useBalcaoStore.getState().activeId).toBe(firstId)
  })

  it('fechar a última comanda abre uma vazia no lugar', () => {
    const onlyId = useBalcaoStore.getState().activeId
    useBalcaoStore.getState().addItem(makeItem())

    useBalcaoStore.getState().closeComanda(onlyId)

    expect(useBalcaoStore.getState().comandas).toHaveLength(1)
    expect(active().items).toHaveLength(0)
    expect(useBalcaoStore.getState().activeId).not.toBe(onlyId)
  })

  it('setActiveComanda ignora id inexistente', () => {
    const current = useBalcaoStore.getState().activeId
    useBalcaoStore.getState().setActiveComanda('nao-existe')
    expect(useBalcaoStore.getState().activeId).toBe(current)
  })
})
