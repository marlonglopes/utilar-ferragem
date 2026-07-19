import { useCallback, useState } from 'react'
import { authGet, authPost, isAuthEnabled } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'
import type { BalcaoCustomer, CustomerSegment } from '@/store/balcaoStore'

/**
 * Cadastro leve de cliente de balcão — `/api/v1/store/customers` (auth-service).
 *
 * O contrato do backend é "um ou nenhum": o documento é chave EXATA e um
 * documento desconhecido devolve 404, não lista vazia. Isso é deliberado (LGPD:
 * não existe busca por nome nem enumeração da base), e a UI segue o mesmo
 * desenho — 404 não é erro, é o caminho normal para "cliente novo, abre o
 * cadastro".
 */

/** Resposta de `model.StoreCustomer`. */
interface StoreCustomerResponse {
  id: string
  document: string
  documentType: 'cpf' | 'cnpj'
  name: string
  phone: string
  email?: string
  segment: string
  createdAt: string
}

const SEGMENTS: CustomerSegment[] = ['varejo', 'atacado', 'construtora']

function toSegment(value: string): CustomerSegment {
  return (SEGMENTS as string[]).includes(value) ? (value as CustomerSegment) : 'varejo'
}

function toCustomer(res: StoreCustomerResponse): BalcaoCustomer {
  return {
    id: res.id,
    name: res.name,
    document: res.document,
    phone: res.phone,
    segment: toSegment(res.segment),
  }
}

/**
 * Base de demonstração para o modo mock. Existe para que o fluxo
 * "busca → achou" seja demonstrável sem backend; qualquer outro documento cai
 * no caminho "não encontrado → cadastro rápido", que é o caso mais comum.
 */
const MOCK_DIRECTORY: Record<string, BalcaoCustomer> = {
  '12345678909': {
    id: 'mock-cust-1',
    name: 'Marcos Andrade (demonstração)',
    document: '12345678909',
    phone: '11999990000',
    segment: 'construtora',
  },
}

/** Resultado da busca: achou, ou não achou. Erro de verdade é exceção. */
export type CustomerLookupResult =
  | { found: true; customer: BalcaoCustomer }
  | { found: false; document: string }

export interface UseBalcaoCustomersResult {
  lookup: (document: string) => Promise<CustomerLookupResult>
  create: (input: {
    document: string
    name: string
    phone: string
    segment: CustomerSegment
  }) => Promise<BalcaoCustomer>
  searching: boolean
  saving: boolean
}

export function useBalcaoCustomers(): UseBalcaoCustomersResult {
  const token = useAuthStore((s) => s.user?.token ?? null)
  const [searching, setSearching] = useState(false)
  const [saving, setSaving] = useState(false)

  const live = isAuthEnabled && !!token

  const lookup = useCallback(
    async (rawDocument: string): Promise<CustomerLookupResult> => {
      const document = rawDocument.replace(/\D/g, '')
      if (!live) {
        const hit = MOCK_DIRECTORY[document]
        return hit ? { found: true, customer: hit } : { found: false, document }
      }
      setSearching(true)
      try {
        const res = await authGet<StoreCustomerResponse>(
          `/api/v1/store/customers?document=${encodeURIComponent(document)}`,
          token!
        )
        return { found: true, customer: toCustomer(res) }
      } catch (err) {
        // `authGet` normaliza 404 em Error('not_found'). Cliente novo não é
        // falha: devolve `found: false` para a UI abrir o cadastro.
        if (err instanceof Error && err.message === 'not_found') {
          return { found: false, document }
        }
        throw err
      } finally {
        setSearching(false)
      }
    },
    [live, token]
  )

  const create = useCallback(
    async (input: {
      document: string
      name: string
      phone: string
      segment: CustomerSegment
    }): Promise<BalcaoCustomer> => {
      const document = input.document.replace(/\D/g, '')
      const phone = input.phone.replace(/\D/g, '')
      const draft: BalcaoCustomer = {
        name: input.name.trim(),
        document,
        phone,
        segment: input.segment,
      }
      if (!live) {
        // Demonstração: passa a existir na base local, então uma segunda busca
        // pelo mesmo documento já encontra.
        const created = { ...draft, id: `mock-cust-${document.slice(-4)}` }
        MOCK_DIRECTORY[document] = created
        return created
      }
      setSaving(true)
      try {
        const res = await authPost<StoreCustomerResponse>(
          '/api/v1/store/customers',
          { document, name: draft.name, phone, segment: input.segment },
          token!
        )
        return toCustomer(res)
      } finally {
        setSaving(false)
      }
    },
    [live, token]
  )

  return { lookup, create, searching, saving }
}
