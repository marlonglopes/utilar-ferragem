import { useState, useCallback, useRef } from 'react'
import { orderPostWithJWT, isApiEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

/**
 * Cotação de frete por CEP, para o carrinho.
 *
 * PORQUÊ existe: o carrinho mostrava o texto fixo "Calculado no checkout".
 * Frete no carrinho é table stakes no Brasil — o cliente decide se compra
 * olhando o total com entrega, e descobrir o frete só no fim do funil é uma das
 * maiores causas de abandono.
 *
 * O backend já servia isso (`POST /api/v1/shipping/quote`, contrato em
 * docs/shipping-api.md); só faltava alguém chamar.
 */

export interface ShippingOption {
  serviceCode: string
  serviceName: string
  zoneName: string
  cost: number
  deliveryDays: number
  free: boolean
}

interface QuoteResponse {
  cep: string
  options: ShippingOption[]
}

/** Só dígitos, no máximo 8. */
export function normalizeCep(raw: string): string {
  return raw.replace(/\D/g, '').slice(0, 8)
}

export function formatCep(raw: string): string {
  const d = normalizeCep(raw)
  return d.length > 5 ? `${d.slice(0, 5)}-${d.slice(5)}` : d
}

export function isValidCep(raw: string): boolean {
  return normalizeCep(raw).length === 8
}

// Mock: faixas plausíveis pra loja em São Paulo, pra demonstração sem backend.
function mockQuote(cep: string, subtotal: number): QuoteResponse {
  const capital = cep.startsWith('0')
  const gratis = subtotal >= 299
  return {
    cep,
    options: [
      {
        serviceCode: 'standard',
        serviceName: 'Entrega padrão',
        zoneName: capital ? 'São Paulo - Capital' : 'Interior/Outros',
        cost: gratis ? 0 : capital ? 24.9 : 39.9,
        deliveryDays: capital ? 2 : 5,
        free: gratis,
      },
      {
        serviceCode: 'express',
        serviceName: 'Entrega expressa',
        zoneName: capital ? 'São Paulo - Capital' : 'Interior/Outros',
        cost: capital ? 47.9 : 74.9,
        deliveryDays: capital ? 1 : 3,
        free: false,
      },
    ],
  }
}

export function useShippingQuote() {
  const token = useAuthStore((s) => s.user?.token ?? null)
  const [options, setOptions] = useState<ShippingOption[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Descarta resposta de cotação antiga: o usuário digita o CEP e dispara
  // várias chamadas; sem isso a resposta de um CEP anterior pode chegar depois
  // e sobrescrever a do CEP atual.
  const reqId = useRef(0)

  const quote = useCallback(
    async (cepRaw: string, subtotal: number, itemCount: number) => {
      const cep = normalizeCep(cepRaw)
      if (!isValidCep(cep)) {
        setError('CEP inválido')
        setOptions(null)
        return null
      }

      const id = ++reqId.current
      setLoading(true)
      setError('')

      try {
        if (!isApiEnabled) {
          await new Promise((r) => setTimeout(r, 350))
          if (id !== reqId.current) return null
          const mock = mockQuote(cep, subtotal)
          setOptions(mock.options)
          return mock.options
        }

        const data = await orderPostWithJWT<QuoteResponse>(
          '/api/v1/shipping/quote',
          token ?? '',
          { cep, subtotal, itemCount },
        )
        if (id !== reqId.current) return null
        setOptions(data.options ?? [])
        return data.options ?? []
      } catch (err) {
        if (id !== reqId.current) return null
        const msg = err instanceof Error ? err.message : ''
        // O backend distingue CEP malformado de região sem cobertura; a
        // diferença importa pro cliente (corrigir o CEP vs. não atendemos aí).
        setError(
          /no_shipping_coverage|coverage/i.test(msg)
            ? 'Ainda não entregamos neste CEP'
            : /bad_request|inválido|invalid/i.test(msg)
              ? 'CEP inválido'
              : 'Não foi possível calcular o frete agora',
        )
        setOptions(null)
        return null
      } finally {
        if (id === reqId.current) setLoading(false)
      }
    },
    [token],
  )

  const reset = useCallback(() => {
    reqId.current++
    setOptions(null)
    setError('')
    setLoading(false)
  }, [])

  return { options, loading, error, quote, reset }
}
