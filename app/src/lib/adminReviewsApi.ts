// Cliente admin de MODERAÇÃO de avaliações. A triagem automática (link, contato,
// caixa alta) segura o que precisa de olho humano; aqui o dono/vendas aprova ou
// recusa. Consome o catalog. Mock quando VITE_CATALOG_URL vazio.
import { adminGet, adminSend } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
export const isReviewsAdminEnabled = CATALOG_URL !== ''

export type ReviewStatus = 'pending' | 'published' | 'rejected'

export interface AdminReview {
  id: string
  productId: string
  authorName: string
  rating: number
  title?: string
  body?: string
  status: ReviewStatus
  moderationNote?: string
  createdAt: string
}

export const REVIEW_STATUS_LABEL: Record<ReviewStatus, string> = {
  pending: 'Pendentes',
  published: 'Publicadas',
  rejected: 'Recusadas',
}

export async function fetchReviews(status: ReviewStatus): Promise<AdminReview[]> {
  if (!isReviewsAdminEnabled) return mockReviews(status)
  const res = await adminGet<{ data: AdminReview[] }>(
    CATALOG_URL,
    `/api/v1/admin/reviews?status=${status}`
  )
  return res.data ?? []
}

export async function moderateReview(
  id: string,
  action: 'approve' | 'reject',
  note?: string
): Promise<void> {
  if (!isReviewsAdminEnabled) return
  const body = note ? { note } : undefined
  await adminSend<unknown>(CATALOG_URL, `/api/v1/admin/reviews/${id}/${action}`, 'POST', body)
}

// ---------------------------------------------------------------------------
// Mock (modo demo).
// ---------------------------------------------------------------------------

const MOCK: AdminReview[] = [
  {
    id: 'rv-1',
    productId: 'p-100',
    authorName: 'João P.',
    rating: 5,
    title: 'Excelente furadeira',
    body: 'Peguei pra obra de casa, aguenta parede de concreto sem esforço.',
    status: 'pending',
    createdAt: '2026-07-31T11:00:00Z',
  },
  {
    id: 'rv-2',
    productId: 'p-101',
    authorName: 'Maria S.',
    rating: 1,
    title: 'Contato',
    body: 'Chama no whats 9999-9999 que faço mais barato',
    status: 'pending',
    createdAt: '2026-07-31T10:00:00Z',
  },
  {
    id: 'rv-3',
    productId: 'p-102',
    authorName: 'Carlos M.',
    rating: 4,
    title: 'Bom cadeado',
    body: 'Resistente, só a chave que é meio dura no começo.',
    status: 'published',
    createdAt: '2026-07-30T09:00:00Z',
  },
]

function mockReviews(status: ReviewStatus): AdminReview[] {
  return MOCK.filter((r) => r.status === status)
}
