import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface User {
  id: string
  email: string
  name: string
  role: 'customer' | 'seller' | 'admin'
  token: string
}

interface AuthState {
  user: User | null
  setUser: (user: User) => void
  clearUser: () => void
  logout: () => void
  token: () => string | null
  isLoggedIn: () => boolean
  isCustomer: () => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      setUser: (user) => set({ user }),
      clearUser: () => set({ user: null }),
      logout: () => set({ user: null }),
      token: () => get().user?.token ?? null,
      isLoggedIn: () => get().user !== null,
      isCustomer: () => get().user?.role === 'customer',
    }),
    { name: 'utilar-auth' }
  )
)
