import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  createOperator,
  fetchOperators,
  fetchStores,
  fetchUsers,
  updateOperator,
  type AdminUser,
  type CreateOperatorInput,
  type Operator,
  type Store,
  type UpdateOperatorInput,
} from '@/lib/adminStaffApi'

const keys = {
  all: ['admin-staff'] as const,
  operators: ['admin-staff', 'operators'] as const,
  stores: ['admin-staff', 'stores'] as const,
  users: (q: string) => ['admin-staff', 'users', q] as const,
}

export function useOperators(): UseQueryResult<Operator[]> {
  return useQuery({ queryKey: keys.operators, queryFn: fetchOperators, staleTime: 15_000 })
}

export function useStores(): UseQueryResult<Store[]> {
  return useQuery({ queryKey: keys.stores, queryFn: fetchStores, staleTime: 60_000 })
}

// Busca só dispara com 2+ caracteres — evita listar a base inteira a cada tecla.
export function useUsersSearch(q: string): UseQueryResult<AdminUser[]> {
  return useQuery({
    queryKey: keys.users(q),
    queryFn: () => fetchUsers(q),
    enabled: q.trim().length >= 2,
    staleTime: 15_000,
  })
}

export function useCreateOperator() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateOperatorInput) => createOperator(input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.all }),
  })
}

export function useUpdateOperator() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, input }: { userId: string; input: UpdateOperatorInput }) =>
      updateOperator(userId, input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.operators }),
  })
}
