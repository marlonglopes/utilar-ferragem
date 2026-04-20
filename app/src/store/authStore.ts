import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface User {
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
  token: () => string | null
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      setUser: (user) => set({ user }),
      clearUser: () => set({ user: null }),
      token: () => get().user?.token ?? null,
    }),
    { name: 'utilar-auth' }
  )
)
