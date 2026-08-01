// Cliente admin da CONFIG de pagamento (leitura). Mostra qual PSP está ativo,
// os métodos e a saúde — NUNCA segredo. Trocar PSP/credencial continua sendo
// variável de ambiente (PSP_PROVIDER + *_SECRET); segredo não se edita por tela.
import { adminGet } from '@/lib/adminApi'

const PAYMENT_URL = import.meta.env.VITE_API_URL ?? ''
export const isPaymentConfigEnabled = PAYMENT_URL !== ''

export interface PaymentConfig {
  provider: string
  methods: string[]
  healthy: boolean
  status: 'ok' | 'degraded'
}

export const PROVIDER_LABEL: Record<string, string> = {
  stripe: 'Stripe',
  mercadopago: 'Mercado Pago',
  appmax: 'Appmax (v3 admin)',
  'appmax-v1': 'Appmax (AppStore v1)',
}

export const METHOD_LABEL: Record<string, string> = {
  pix: 'Pix',
  credit_card: 'Cartão de crédito',
  boleto: 'Boleto',
}

export async function fetchPaymentConfig(): Promise<PaymentConfig> {
  if (!isPaymentConfigEnabled) return MOCK
  return adminGet<PaymentConfig>(PAYMENT_URL, '/api/v1/admin/payment/config')
}

const MOCK: PaymentConfig = {
  provider: 'appmax-v1',
  methods: ['pix', 'credit_card', 'boleto'],
  healthy: true,
  status: 'ok',
}
