// Cliente admin de CATEGORIAS. Listar (público) + criar/renomear/excluir (admin).
// Antes só dava pra criar categoria por seed; agora o dono organiza pela tela.
import { adminGet, adminSend } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
export const isCategoriesAdminEnabled = CATALOG_URL !== ''

export interface Category {
  id: string
  name: string
  icon: string
  parentId: string | null
  sortOrder: number
}

export interface CreateCategoryInput {
  id: string
  name: string
  icon?: string
  sortOrder?: number
}

export interface UpdateCategoryInput {
  name?: string
  icon?: string
  sortOrder?: number
}

export async function fetchCategories(): Promise<Category[]> {
  if (!isCategoriesAdminEnabled) return MOCK
  const res = await adminGet<{ data: Category[] } | Category[]>(CATALOG_URL, '/api/v1/categories')
  return Array.isArray(res) ? res : res.data
}

export async function createCategory(input: CreateCategoryInput): Promise<void> {
  if (!isCategoriesAdminEnabled) return
  await adminSend<unknown>(CATALOG_URL, '/api/v1/admin/categories', 'POST', input)
}

export async function updateCategory(id: string, input: UpdateCategoryInput): Promise<void> {
  if (!isCategoriesAdminEnabled) return
  await adminSend<unknown>(CATALOG_URL, `/api/v1/admin/categories/${id}`, 'PATCH', input)
}

export async function deleteCategory(id: string): Promise<void> {
  if (!isCategoriesAdminEnabled) return
  await adminSend<unknown>(CATALOG_URL, `/api/v1/admin/categories/${id}`, 'DELETE')
}

// Valida o slug do lado do cliente antes de mandar (o servidor também valida).
export function isValidCategoryId(id: string): boolean {
  return /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(id)
}

const MOCK: Category[] = [
  { id: 'ferramentas', name: 'Ferramentas', icon: '⚒', parentId: null, sortOrder: 1 },
  { id: 'construcao', name: 'Construção', icon: '◫', parentId: null, sortOrder: 2 },
  { id: 'eletrica', name: 'Elétrica', icon: '⚡', parentId: null, sortOrder: 3 },
  { id: 'hidraulica', name: 'Hidráulica', icon: '◡', parentId: null, sortOrder: 4 },
  { id: 'fixacao', name: 'Fixação', icon: '▣', parentId: null, sortOrder: 5 },
  { id: 'pintura', name: 'Pintura', icon: '▥', parentId: null, sortOrder: 6 },
  { id: 'seguranca', name: 'Segurança', icon: '⚠', parentId: null, sortOrder: 7 },
  { id: 'jardim', name: 'Jardim', icon: '❀', parentId: null, sortOrder: 8 },
]
