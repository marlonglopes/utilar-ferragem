import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import { configureAuthHooks, authLogout } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

// Bridge entre api.ts (sem React) e authStore (Zustand). Habilita o
// refresh-on-401: quando uma chamada autenticada recebe 401, api.ts pega
// o refreshToken via getRefreshToken, faz POST /auth/refresh, atualiza o
// token no store via setAccessToken e re-tenta a request.
configureAuthHooks({
  getToken: () => useAuthStore.getState().user?.token ?? null,
  getRefreshToken: () => useAuthStore.getState().user?.refreshToken ?? null,
  setAccessToken: (token) => useAuthStore.getState().updateAccessToken(token),

  // Sessão derrubada pelo próprio sistema (o refresh falhou), não pelo usuário.
  //
  // Tenta revogar mesmo assim, por defesa em profundidade: o refresh pode ter
  // falhado por 500 ou queda de rede, e nesse caso os tokens continuam VIVOS no
  // servidor. Se falhou porque já estavam revogados/expirados, a chamada
  // simplesmente não faz nada — `authLogout` é best-effort e nunca lança.
  //
  // A limpeza local vem primeiro na intenção e não depende disso: os tokens são
  // lidos antes, e `clearUser()` roda de qualquer jeito.
  clearSession: () => {
    const user = useAuthStore.getState().user
    void authLogout(user?.token ?? null, user?.refreshToken ?? null)
    useAuthStore.getState().clearUser()
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>
)
