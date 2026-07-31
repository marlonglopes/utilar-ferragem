import { useMutation, useQuery, useQueryClient, type UseQueryResult } from '@tanstack/react-query'
import {
  createCategory,
  deleteCategory,
  fetchCategories,
  updateCategory,
  type Category,
  type CreateCategoryInput,
  type UpdateCategoryInput,
} from '@/lib/adminCategoriesApi'

const keys = { all: ['admin-categories'] as const }

export function useAdminCategories(): UseQueryResult<Category[]> {
  return useQuery({ queryKey: keys.all, queryFn: fetchCategories, staleTime: 30_000 })
}

export function useCreateCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateCategoryInput) => createCategory(input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.all }),
  })
}

export function useUpdateCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateCategoryInput }) =>
      updateCategory(id, input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.all }),
  })
}

export function useDeleteCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteCategory(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.all }),
  })
}
