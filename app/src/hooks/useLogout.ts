import { useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store/authStore'
import { authLogout } from '@/lib/api'

/**
 * Encerramento de sessão do cliente, em um lugar só.
 *
 * Três coisas, nesta ordem, e a ordem importa:
 *
 * 1. **Revogar no servidor** (`POST /api/v1/auth/logout`). Antes disso, sair
 *    limpava só o navegador e o refresh token seguia VÁLIDO no banco por até 30
 *    dias — se tivesse vazado (extensão maliciosa, backup do navegador,
 *    terminal compartilhado da loja), "sair" não protegia nada: a sessão
 *    continuava viva do outro lado. O servidor revoga o refresh pelo hash e
 *    ainda põe o access token numa deny-list, encurtando a janela de 15 minutos
 *    que sobraria.
 *
 * 2. **Limpar o cache do TanStack Query.** `authStore.logout()` sozinho só zera
 *    o usuário; o cache continua com pedidos, endereços e nome de quem acabou
 *    de sair, e quem entrasse em seguida no MESMO aparelho veria esses dados por
 *    um instante antes do refetch. `clear()` e não `invalidateQueries()`:
 *    invalidar mantém o dado antigo em memória e o entrega enquanto revalida —
 *    exatamente o vazamento que se quer evitar.
 *
 * 3. **Zerar a sessão local e navegar.**
 *
 * REGRA INVIOLÁVEL: a limpeza local acontece SEMPRE, mesmo se a revogação
 * falhar. Rede caída não pode prender a pessoa logada num terminal
 * compartilhado — a segurança local é o mínimo garantido, a revogação no
 * servidor é o reforço, nunca uma condição para sair. Por isso os tokens são
 * capturados ANTES de limpar, a revogação é disparada sem `await`, e nada aqui
 * depende do resultado dela.
 *
 * O CARRINHO é preservado de propósito: existe para visitante também, e apagar
 * no logout faria o cliente perder a compra montada ao trocar de conta.
 * Favoritos, pelo mesmo motivo, também ficam (ver favoritesStore).
 */
export function useLogout() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const logout = useAuthStore((s) => s.logout)

  return useCallback(
    (redirectTo = '/') => {
      // Captura os tokens ANTES de limpar — depois do `logout()` eles somem e
      // não há mais o que mandar o servidor revogar.
      const user = useAuthStore.getState().user
      const accessToken = user?.token ?? null
      const refreshToken = user?.refreshToken ?? null

      // Dispara sem await: a saída do usuário não espera a rede. `authLogout`
      // trata o próprio erro e nunca lança, então não fica promessa rejeitada
      // solta — o `void` é explícito sobre isso ser intencional.
      void authLogout(accessToken, refreshToken)

      logout()
      queryClient.clear()
      navigate(redirectTo, { replace: true })
    },
    [logout, queryClient, navigate]
  )
}
