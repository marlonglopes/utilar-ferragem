import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface CartItem {
  productId: string
  sellerId: string
  sellerName: string
  name: string
  icon: string
  priceSnapshot: number
  quantity: number
  stock: number
  addedAt: string
}

interface CartState {
  items: CartItem[]
  addItem: (item: Omit<CartItem, 'addedAt'>) => void
  removeItem: (productId: string) => void
  updateQuantity: (productId: string, quantity: number) => void
  clearCart: () => void
  mergeCarts: (incoming: CartItem[]) => void
}

export const useCartStore = create<CartState>()(
  persist(
    (set) => ({
      items: [],

      addItem: (newItem) =>
        set((state) => {
          const existing = state.items.find((i) => i.productId === newItem.productId)
          if (existing) {
            return {
              items: state.items.map((i) =>
                i.productId === newItem.productId
                  ? { ...i, quantity: Math.min(i.stock, i.quantity + newItem.quantity) }
                  : i
              ),
            }
          }
          return {
            items: [...state.items, { ...newItem, addedAt: new Date().toISOString() }],
          }
        }),

      removeItem: (productId) =>
        set((state) => ({ items: state.items.filter((i) => i.productId !== productId) })),

      updateQuantity: (productId, quantity) =>
        set((state) => ({
          items: state.items.map((i) =>
            i.productId === productId
              ? { ...i, quantity: Math.max(1, Math.min(i.stock, quantity)) }
              : i
          ),
        })),

      clearCart: () => set({ items: [] }),

      mergeCarts: (incoming) =>
        set((state) => {
          const merged = state.items.map((i) => ({ ...i }))
          for (const item of incoming) {
            const existing = merged.find((i) => i.productId === item.productId)
            if (!existing) {
              merged.push(item)
            } else {
              existing.quantity = Math.min(existing.stock, existing.quantity + item.quantity)
            }
          }
          return { items: merged }
        }),
    }),
    { name: 'utilar-cart' }
  )
)
