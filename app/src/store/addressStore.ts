import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface Address {
  id: string
  label: string
  cep: string
  street: string
  number: string
  complement: string
  neighborhood: string
  city: string
  state: string
  isDefault: boolean
}

export type AddressInput = Omit<Address, 'id' | 'isDefault'>

interface AddressState {
  addresses: Address[]
  addAddress: (input: AddressInput, makeDefault?: boolean) => Address
  updateAddress: (id: string, input: AddressInput) => void
  removeAddress: (id: string) => void
  setDefault: (id: string) => void
  getDefault: () => Address | null
}

export const useAddressStore = create<AddressState>()(
  persist(
    (set, get) => ({
      addresses: [],

      addAddress: (input, makeDefault) => {
        const id = crypto.randomUUID()
        const current = get().addresses
        const shouldBeDefault = makeDefault || current.length === 0
        const created: Address = { ...input, id, isDefault: shouldBeDefault }
        set({
          addresses: shouldBeDefault
            ? [...current.map((a) => ({ ...a, isDefault: false })), created]
            : [...current, created],
        })
        return created
      },

      updateAddress: (id, input) =>
        set((state) => ({
          addresses: state.addresses.map((a) => (a.id === id ? { ...a, ...input } : a)),
        })),

      removeAddress: (id) =>
        set((state) => {
          const next = state.addresses.filter((a) => a.id !== id)
          if (next.length > 0 && !next.some((a) => a.isDefault)) next[0].isDefault = true
          return { addresses: next }
        }),

      setDefault: (id) =>
        set((state) => ({
          addresses: state.addresses.map((a) => ({ ...a, isDefault: a.id === id })),
        })),

      getDefault: () => {
        const list = get().addresses
        return list.find((a) => a.isDefault) ?? list[0] ?? null
      },
    }),
    { name: 'utilar-addresses' }
  )
)
