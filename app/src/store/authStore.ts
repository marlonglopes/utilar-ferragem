import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface User {
  id: string
  email: string
  name: string
  // Espelha o enum user_role do auth-service. `store_operator` é o VENDEDOR
  // DE BALCÃO — não confundir com `seller`, que é lojista do marketplace.
  // O tipo ficou desatualizado quando o papel foi criado no backend, e o
  // resultado é que o operador caía no ramo genérico do redirecionamento.
  role: 'customer' | 'seller' | 'admin' | 'store_operator'
  token: string           // JWT access token (expires em 15min quando vindo do auth-service)
  refreshToken?: string   // opaco, revogável (30 dias)
  emailVerified?: boolean
  cpf?: string            // 11 dígitos, opcional — usado pra pre-fill checkout (boleto)
  phone?: string
}

interface AuthState {
  user: User | null
  setUser: (user: User) => void
  clearUser: () => void
  logout: () => void
  token: () => string | null
  isLoggedIn: () => boolean
  isCustomer: () => boolean
  // Atualiza apenas o access token (após refresh) preservando o refreshToken
  // e demais dados do user. Usado pelo refresh-on-401 em api.ts.
  updateAccessToken: (token: string) => void
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
      updateAccessToken: (token) => {
        const u = get().user
        if (u) set({ user: { ...u, token } })
      },
    }),
    { name: 'utilar-auth' }
  )
)
