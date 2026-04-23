import { describe, it, expect, beforeEach } from 'vitest'
import { useCartStore } from '@/store/cartStore'
import type { CartItem } from '@/store/cartStore'

function makeItem(overrides: Partial<CartItem> = {}): Omit<CartItem, 'addedAt'> {
  return {
    productId: 'prod-1',
    sellerId: 'seller-1',
    sellerName: 'Ferragens SP',
    name: 'Furadeira Bosch',
    icon: '🔧',
    priceSnapshot: 299.9,
    quantity: 1,
    stock: 10,
    ...overrides,
  }
}

beforeEach(() => {
  useCartStore.setState({ items: [] })
})

describe('cartStore.addItem', () => {
  it('adds a new item', () => {
    useCartStore.getState().addItem(makeItem())
    expect(useCartStore.getState().items).toHaveLength(1)
    expect(useCartStore.getState().items[0].productId).toBe('prod-1')
  })

  it('increments quantity when same product added twice', () => {
    useCartStore.getState().addItem(makeItem({ quantity: 2 }))
    useCartStore.getState().addItem(makeItem({ quantity: 3 }))
    const items = useCartStore.getState().items
    expect(items).toHaveLength(1)
    expect(items[0].quantity).toBe(5)
  })

  it('respects stock limit when incrementing', () => {
    useCartStore.getState().addItem(makeItem({ quantity: 8, stock: 10 }))
    useCartStore.getState().addItem(makeItem({ quantity: 5, stock: 10 }))
    expect(useCartStore.getState().items[0].quantity).toBe(10)
  })

  it('adds addedAt timestamp', () => {
    useCartStore.getState().addItem(makeItem())
    expect(useCartStore.getState().items[0].addedAt).toBeDefined()
  })
})

describe('cartStore.removeItem', () => {
  it('removes an item by productId', () => {
    useCartStore.getState().addItem(makeItem({ productId: 'a' }))
    useCartStore.getState().addItem(makeItem({ productId: 'b' }))
    useCartStore.getState().removeItem('a')
    const ids = useCartStore.getState().items.map((i) => i.productId)
    expect(ids).toEqual(['b'])
  })

  it('no-ops when productId not in cart', () => {
    useCartStore.getState().addItem(makeItem())
    useCartStore.getState().removeItem('nonexistent')
    expect(useCartStore.getState().items).toHaveLength(1)
  })
})

describe('cartStore.updateQuantity', () => {
  it('updates quantity within stock bounds', () => {
    useCartStore.getState().addItem(makeItem({ stock: 10 }))
    useCartStore.getState().updateQuantity('prod-1', 7)
    expect(useCartStore.getState().items[0].quantity).toBe(7)
  })

  it('clamps quantity to minimum 1', () => {
    useCartStore.getState().addItem(makeItem())
    useCartStore.getState().updateQuantity('prod-1', 0)
    expect(useCartStore.getState().items[0].quantity).toBe(1)
  })

  it('clamps quantity to stock max', () => {
    useCartStore.getState().addItem(makeItem({ stock: 5 }))
    useCartStore.getState().updateQuantity('prod-1', 99)
    expect(useCartStore.getState().items[0].quantity).toBe(5)
  })
})

describe('cartStore.clearCart', () => {
  it('empties all items', () => {
    useCartStore.getState().addItem(makeItem({ productId: 'a' }))
    useCartStore.getState().addItem(makeItem({ productId: 'b' }))
    useCartStore.getState().clearCart()
    expect(useCartStore.getState().items).toHaveLength(0)
  })
})

describe('cartStore.mergeCarts', () => {
  it('adds items not already in cart', () => {
    const incoming: CartItem[] = [
      { ...makeItem({ productId: 'new-1' }), addedAt: new Date().toISOString() },
    ]
    useCartStore.getState().mergeCarts(incoming)
    expect(useCartStore.getState().items).toHaveLength(1)
    expect(useCartStore.getState().items[0].productId).toBe('new-1')
  })

  it('merges quantity for existing items without exceeding stock', () => {
    useCartStore.getState().addItem(makeItem({ productId: 'prod-1', quantity: 4, stock: 10 }))
    const incoming: CartItem[] = [
      { ...makeItem({ productId: 'prod-1', quantity: 3, stock: 10 }), addedAt: new Date().toISOString() },
    ]
    useCartStore.getState().mergeCarts(incoming)
    expect(useCartStore.getState().items[0].quantity).toBe(7)
  })

  it('does not exceed stock on merge', () => {
    useCartStore.getState().addItem(makeItem({ productId: 'prod-1', quantity: 8, stock: 10 }))
    const incoming: CartItem[] = [
      { ...makeItem({ productId: 'prod-1', quantity: 5, stock: 10 }), addedAt: new Date().toISOString() },
    ]
    useCartStore.getState().mergeCarts(incoming)
    expect(useCartStore.getState().items[0].quantity).toBe(10)
  })
})
