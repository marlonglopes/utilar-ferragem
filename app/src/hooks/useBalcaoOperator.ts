import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { authGet, isAuthEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import {
  MOCK_OPERATOR,
  useBalcaoStore,
  type BalcaoLevel,
  type BalcaoOperator,
} from '@/store/balcaoStore'

/**
 * Contexto de loja do operador logado — `GET /api/v1/store/me` (auth-service).
 *
 * É daqui que vem o TETO DE DESCONTO. Antes ele era uma constante no front
 * (`DISCOUNT_CEILING_BY_ROLE`), o que significava que mudar o poder de desconto
 * de alguém exigia um deploy — e que qualquer um com o devtools aberto podia
 * reescrever o próprio limite. Agora o número mora no banco e o servidor
 * recalcula tudo de novo na criação do pedido; o valor daqui é só o que o
 * vendedor VÊ antes de fechar a venda.
 *
 * FAIL-CLOSED: erro de rede, 403 ou vínculo revogado deixam o teto em 0 (via
 * `setOperator(null)`), então todo desconto sai pendente de aprovação. Mesma
 * política do `actorFromContext` no order-service.
 */

/** Resposta de `GET /api/v1/store/me` — `model.StoreOperator` do auth-service. */
interface StoreMeResponse {
  userId: string
  name: string
  email: string
  storeId: string
  storeCode: string
  storeName: string
  level: BalcaoLevel
  discountCeilingPct: number
  canApproveDiscount: boolean
  active: boolean
}

function toOperator(res: StoreMeResponse): BalcaoOperator {
  return {
    userId: res.userId,
    name: res.name,
    storeId: res.storeId,
    storeCode: res.storeCode,
    storeName: res.storeName,
    level: res.level,
    // Number() defensivo: um `null` vindo do JSON viraria NaN nas comparações
    // de teto e faria todo desconto parecer dentro do limite.
    discountCeilingPct: Number(res.discountCeilingPct) || 0,
    canApproveDiscount: res.canApproveDiscount === true,
    fromBackend: true,
  }
}

export interface UseBalcaoOperatorResult {
  operator: BalcaoOperator
  isLoading: boolean
  isError: boolean
  /** Autenticado, mas sem vínculo de operador (404) ou sem permissão (403). */
  notOperator: boolean
  errorMessage: string
  refetch: () => void
}

export function useBalcaoOperator(): UseBalcaoOperatorResult {
  const token = useAuthStore((s) => s.user?.token ?? null)
  const operator = useBalcaoStore((s) => s.operator)
  const setOperator = useBalcaoStore((s) => s.setOperator)

  const enabled = isAuthEnabled && !!token

  const query = useQuery({
    queryKey: ['balcao', 'store-me', token],
    enabled,
    // O teto é dinheiro: não vale cache longo. 60s cobre a rajada de renders de
    // uma venda sem transformar o PDV num cliente de polling.
    staleTime: 60_000,
    retry: 1,
    queryFn: async () => toOperator(await authGet<StoreMeResponse>('/api/v1/store/me', token!)),
  })

  // `operator.userId === ''` é o FAIL_CLOSED_OPERATOR intocado — ninguém
  // resolveu o contexto ainda.
  const unresolved = operator.userId === ''

  useEffect(() => {
    if (!enabled) {
      // Modo mock/demonstração: contexto plausível para o PDV rodar sem backend.
      // Semeia UMA vez: sobrescrever a cada render descartaria um contexto que
      // alguém já tenha definido (demonstração de outro cargo, teste).
      if (unresolved) setOperator(MOCK_OPERATOR)
      return
    }
    if (query.data) {
      setOperator(query.data)
      return
    }
    if (query.isError) {
      setOperator(null) // fail-closed: teto 0
    }
  }, [enabled, unresolved, query.data, query.isError, setOperator])

  const rawMessage = query.error instanceof Error ? query.error.message : ''
  // `authGet` normaliza 404 para 'not_found'; 403 chega com a mensagem do
  // envelope do backend.
  const notOperator = rawMessage === 'not_found' || /operator|permiss|forbidden/i.test(rawMessage)

  return {
    operator,
    isLoading: enabled && query.isLoading,
    isError: query.isError,
    notOperator: query.isError && notOperator,
    errorMessage:
      rawMessage === 'not_found'
        ? 'Sua conta não tem vínculo de operador com nenhuma loja.'
        : rawMessage,
    refetch: () => void query.refetch(),
  }
}
