export type OrderStatus =
  | 'pending_payment'
  | 'paid'
  | 'picking'
  | 'shipped'
  | 'delivered'
  | 'cancelled'

export type PaymentMethod = 'pix' | 'boleto' | 'card'

export interface OrderItem {
  productId: string
  name: string
  icon: string
  sellerId: string
  sellerName: string
  quantity: number
  unitPrice: number
}

export interface OrderAddress {
  street: string
  number: string
  complement?: string
  neighborhood: string
  city: string
  state: string
  cep: string
}

export interface Order {
  id: string
  number: string
  status: OrderStatus
  paymentMethod: PaymentMethod
  paymentInfo: string
  items: OrderItem[]
  subtotal: number
  shippingCost: number
  total: number
  address: OrderAddress
  trackingCode?: string
  createdAt: string
  paidAt?: string
  pickedAt?: string
  shippedAt?: string
  deliveredAt?: string
  cancelledAt?: string
  updatedAt: string
}

export const MOCK_ORDERS: Order[] = [
  {
    id: 'pay-mock-001',
    number: '2024-001',
    status: 'delivered',
    paymentMethod: 'pix',
    paymentInfo: 'Pix · pago em 10/04 14:32',
    items: [
      {
        productId: 'p-furadeira',
        name: 'Furadeira Bosch GSB 13 RE',
        icon: '🔧',
        sellerId: 's1',
        sellerName: 'Ferragens SP',
        quantity: 1,
        unitPrice: 299.9,
      },
      {
        productId: 'p-broca',
        name: 'Jogo de Brocas 19 peças',
        icon: '🪛',
        sellerId: 's1',
        sellerName: 'Ferragens SP',
        quantity: 2,
        unitPrice: 49.9,
      },
    ],
    subtotal: 399.7,
    shippingCost: 0,
    total: 399.7,
    address: {
      street: 'Av. Paulista',
      number: '1000',
      complement: 'Apto 42',
      neighborhood: 'Bela Vista',
      city: 'São Paulo',
      state: 'SP',
      cep: '01310-100',
    },
    trackingCode: 'BR123456789SP',
    createdAt: '2026-04-10T10:00:00Z',
    paidAt: '2026-04-10T10:02:00Z',
    pickedAt: '2026-04-10T14:00:00Z',
    shippedAt: '2026-04-11T09:00:00Z',
    deliveredAt: '2026-04-14T15:30:00Z',
    updatedAt: '2026-04-14T15:30:00Z',
  },
  {
    id: 'pay-mock-002',
    number: '2024-002',
    status: 'shipped',
    paymentMethod: 'card',
    paymentInfo: 'Cartão de crédito · 3x de R$ 43,30',
    items: [
      {
        productId: 'p-martelo',
        name: 'Martelo de Borracha 500g',
        icon: '🔨',
        sellerId: 's2',
        sellerName: 'Casa das Ferramentas',
        quantity: 1,
        unitPrice: 89.9,
      },
      {
        productId: 'p-nivel',
        name: 'Nível de Bolha 60cm',
        icon: '📏',
        sellerId: 's2',
        sellerName: 'Casa das Ferramentas',
        quantity: 1,
        unitPrice: 39.9,
      },
    ],
    subtotal: 129.8,
    shippingCost: 15.9,
    total: 145.7,
    address: {
      street: 'Rua das Flores',
      number: '250',
      neighborhood: 'Jardim Europa',
      city: 'Campinas',
      state: 'SP',
      cep: '13025-000',
    },
    trackingCode: 'BR987654321SP',
    createdAt: '2026-04-18T08:30:00Z',
    paidAt: '2026-04-18T08:32:00Z',
    pickedAt: '2026-04-18T16:00:00Z',
    shippedAt: '2026-04-19T10:00:00Z',
    updatedAt: '2026-04-19T10:00:00Z',
  },
  {
    id: 'pay-mock-003',
    number: '2024-003',
    status: 'paid',
    paymentMethod: 'boleto',
    paymentInfo: 'Boleto bancário · pago em 20/04',
    items: [
      {
        productId: 'p-fita',
        name: 'Fita Isolante Profissional 10m',
        icon: '🔌',
        sellerId: 's3',
        sellerName: 'Elétrica Total',
        quantity: 5,
        unitPrice: 8.9,
      },
      {
        productId: 'p-disjuntor',
        name: 'Disjuntor Bipolar 20A',
        icon: '⚡',
        sellerId: 's3',
        sellerName: 'Elétrica Total',
        quantity: 2,
        unitPrice: 34.9,
      },
    ],
    subtotal: 114.3,
    shippingCost: 15.9,
    total: 130.2,
    address: {
      street: 'Rua Marechal Deodoro',
      number: '88',
      neighborhood: 'Centro',
      city: 'Santos',
      state: 'SP',
      cep: '11010-000',
    },
    createdAt: '2026-04-20T11:00:00Z',
    paidAt: '2026-04-21T09:00:00Z',
    updatedAt: '2026-04-21T09:00:00Z',
  },
  {
    id: 'pay-mock-004',
    number: '2024-004',
    status: 'pending_payment',
    paymentMethod: 'pix',
    paymentInfo: 'Pix · aguardando pagamento',
    items: [
      {
        productId: 'p-cimento',
        name: 'Cimento CP II 50kg',
        icon: '🧱',
        sellerId: 's4',
        sellerName: 'Construção Fácil',
        quantity: 3,
        unitPrice: 38.5,
      },
    ],
    subtotal: 115.5,
    shippingCost: 38.9,
    total: 154.4,
    address: {
      street: 'Av. Brasil',
      number: '500',
      neighborhood: 'Jardim América',
      city: 'São Paulo',
      state: 'SP',
      cep: '01430-001',
    },
    createdAt: '2026-04-23T09:00:00Z',
    updatedAt: '2026-04-23T09:00:00Z',
  },
]
