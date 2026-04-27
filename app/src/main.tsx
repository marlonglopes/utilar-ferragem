import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'
import { configureAuthHooks } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

// Bridge entre api.ts (sem React) e authStore (Zustand). Habilita o
// refresh-on-401: quando uma chamada autenticada recebe 401, api.ts pega
// o refreshToken via getRefreshToken, faz POST /auth/refresh, atualiza o
// token no store via setAccessToken e re-tenta a request.
configureAuthHooks({
  getToken: () => useAuthStore.getState().user?.token ?? null,
  getRefreshToken: () => useAuthStore.getState().user?.refreshToken ?? null,
  setAccessToken: (token) => useAuthStore.getState().updateAccessToken(token),
  clearSession: () => useAuthStore.getState().clearUser(),
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>
)
