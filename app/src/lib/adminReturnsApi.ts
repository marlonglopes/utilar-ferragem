// Cliente admin de DEVOLUÇÕES — a fila da loja. Ver, aprovar/recusar, receber a
// mercadoria (físico) e estornar (dinheiro, só admin). Consome o order-service.
// Mock quando VITE_ORDER_URL vazio. Ver docs/devolucao-e-troca.md.
import { adminGet, adminSend } from '@/lib/adminApi'

const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
export const isReturnsAdminEnabled = ORDER_URL !== ''

// Estados da máquina de devolução (internal/returns). A ordem é imposta pelo
// servidor; aqui só decidimos qual ação faz sentido mostrar.
export type ReturnStatus =
  'requested' | 'approved' | 'in_transit' | 'received' | 'refunded' | 'rejected'

export interface ReturnItem {
  id: string
  orderId: string
  userId: string
  kind: string
  status: ReturnStatus
  refundTotal: number
  refundAmount: number
  refundShipping: number
  paymentMethod: string
  fullReturn: boolean
}

// O backend serializa em PascalCase (struct sem json tags) e embrulha em
// {returns:[...]}. Normalizamos para camelCase aqui, para o resto do front não
// carregar essa peculiaridade.
interface RawReturn {
  ID: string
  OrderID: string
  UserID: string
  Kind: string
  Status: ReturnStatus
  RefundTotal: number
  RefundAmount: number
  RefundShipping: number
  PaymentMethod: string
  FullReturn: boolean
}

function normalize(r: RawReturn): ReturnItem {
  return {
    id: r.ID,
    orderId: r.OrderID,
    userId: r.UserID,
    kind: r.Kind,
    status: r.Status,
    refundTotal: r.RefundTotal,
    refundAmount: r.RefundAmount,
    refundShipping: r.RefundShipping,
    paymentMethod: r.PaymentMethod,
    fullReturn: r.FullReturn,
  }
}

export async function fetchReturns(status?: string): Promise<ReturnItem[]> {
  if (!isReturnsAdminEnabled) return mockReturns(status)
  const qs = status ? `?status=${encodeURIComponent(status)}` : ''
  const res = await adminGet<{ returns: RawReturn[] }>(ORDER_URL, `/api/v1/admin/returns${qs}`)
  return (res.returns ?? []).map(normalize)
}

type Action = 'approve' | 'reject' | 'receive' | 'refund'

export async function actOnReturn(id: string, action: Action, note?: string): Promise<void> {
  if (!isReturnsAdminEnabled) return
  const body = note ? { note } : undefined
  await adminSend<unknown>(ORDER_URL, `/api/v1/admin/returns/${id}/${action}`, 'PATCH', body)
}

// Rótulos e a próxima ação por estado.
export const RETURN_STATUS_LABEL: Record<ReturnStatus, string> = {
  requested: 'Solicitada',
  approved: 'Aprovada',
  in_transit: 'Em trânsito',
  received: 'Recebida',
  refunded: 'Estornada',
  rejected: 'Recusada',
}

// ---------------------------------------------------------------------------
// Mock (modo demo).
// ---------------------------------------------------------------------------

const MOCK: ReturnItem[] = [
  {
    id: 'r-1',
    orderId: 'ord-1001',
    userId: 'u-1',
    kind: 'defect',
    status: 'requested',
    refundTotal: 249.9,
    refundAmount: 249.9,
    refundShipping: 0,
    paymentMethod: 'pix',
    fullReturn: true,
  },
  {
    id: 'r-2',
    orderId: 'ord-0990',
    userId: 'u-2',
    kind: 'regret',
    status: 'approved',
    refundTotal: 89.5,
    refundAmount: 74.5,
    refundShipping: 15,
    paymentMethod: 'credit_card',
    fullReturn: false,
  },
  {
    id: 'r-3',
    orderId: 'ord-0975',
    userId: 'u-3',
    kind: 'defect',
    status: 'received',
    refundTotal: 129.9,
    refundAmount: 129.9,
    refundShipping: 0,
    paymentMethod: 'boleto',
    fullReturn: true,
  },
]

function mockReturns(status?: string): ReturnItem[] {
  if (status) return MOCK.filter((r) => r.status === status)
  // Sem filtro: a fila ABERTA (o backend esconde refunded/rejected por padrão).
  return MOCK.filter((r) => !['refunded', 'rejected'].includes(r.status))
}
