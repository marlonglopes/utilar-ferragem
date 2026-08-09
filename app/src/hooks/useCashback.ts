import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store/authStore'
import {
  fetchMyCashback,
  fetchCashbackConfig,
  updateCashbackConfig,
  type CashbackConfig,
} from '@/lib/cashbackApi'

// Saldo/extrato do cliente logado. Chaveado pelo token pra trocar de conta
// invalidar o cache.
export function useMyCashback() {
  const token = useAuthStore((s) => s.user?.token ?? null)
  return useQuery({
    queryKey: ['me', 'cashback', token],
    queryFn: () => fetchMyCashback(token),
    staleTime: 30_000,
  })
}

const CFG_KEY = ['admin', 'cashback']

export function useCashbackConfig() {
  return useQuery({ queryKey: CFG_KEY, queryFn: fetchCashbackConfig })
}

export function useUpdateCashbackConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (cfg: CashbackConfig) => updateCashbackConfig(cfg),
    onSuccess: () => qc.invalidateQueries({ queryKey: CFG_KEY }),
  })
}
