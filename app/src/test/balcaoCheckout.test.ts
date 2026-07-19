import { renderHook, act, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { BalcaoItem, BalcaoPricing } from '@/store/balcaoStore'
import { computeBalcaoPricing } from '@/store/balcaoStore'

/**
 * Criação do pedido de balcão.
 *
 * O que estes testes travam é o contrato novo do order-service:
 *
 *   - `channel: 'balcao'` — sem ele o pedido cai na trilha web;
 *   - SEM `address` — o backend RECUSA (400) endereço em pedido de balcão, e o
 *     PDV mandava um endereço falso ("Retirada no balcão", CEP 00000-000) só
 *     para passar num `binding:"required"` que já não existe;
 *   - dados do cliente no corpo (a Appmax exige o telefone do pagador);
 *   - `approvalStatus` lido da RESPOSTA, não do cálculo local — o servidor
 *     reavalia o desconto contra o teto atual do vínculo.
 */

const orderPostWithJWT = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  // ApiError, isApiError e apiErrorDetails são puros e são justamente o que
  // describeOrderError usa pra classificar o erro pelo `code`. Mockar isso
  // esvaziaria o teste — importamos o original e substituímos só o transporte.
  ...(await importOriginal<typeof import('@/lib/api')>()),
  isOrderEnabled: true,
  isApiEnabled: false,
  isAuthEnabled: true,
  isCatalogEnabled: false,
  orderPostWithJWT: (...args: unknown[]) => orderPostWithJWT(...args),
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  configureAuthHooks: vi.fn(),
}))

const { useBalcaoCheckout, describeOrderError } = await import('@/hooks/useBalcaoCheckout')
const { useAuthStore } = await import('@/store/authStore')

function item(overrides: Partial<BalcaoItem> = {}): BalcaoItem {
  return {
    productId: 'p1',
    sku: 'FER-00001',
    name: 'Furadeira',
    icon: '⚒',
    unit: 'un',
    unitPrice: 100,
    unitCost: 60,
    costIsEstimated: true,
    quantity: 2,
    stock: 10,
    addedAt: new Date().toISOString(),
    ...overrides,
  }
}

function pricingFor(items: BalcaoItem[], discountPct = 0, ceilingPct = 12): BalcaoPricing {
  return computeBalcaoPricing({ items, discountPct, ceilingPct })
}

const CUSTOMER = {
  id: 'cust-1',
  name: 'Construtora Aurora',
  document: '111.444.777-35',
  phone: '(11) 98888-7777',
  segment: 'construtora' as const,
}

beforeEach(() => {
  orderPostWithJWT.mockReset()
  useAuthStore.setState({
    user: { id: 'op-1', email: 'v@loja.com', name: 'Vendedor', role: 'admin', token: 'jwt-token' },
  })
})

describe('useBalcaoCheckout — payload do pedido', () => {
  it('manda channel=balcao, desconto e cliente, e NÃO manda endereço', async () => {
    orderPostWithJWT.mockResolvedValue({
      id: 'ord-1',
      number: 'BAL-0001',
      approvalStatus: 'not_required',
      discountPct: 10,
      discountAmount: 20,
    })

    const { result } = renderHook(() => useBalcaoCheckout())
    const items = [item()]

    await act(async () => {
      await result.current.charge({
        items,
        pricing: pricingFor(items, 10),
        customer: CUSTOMER,
        method: 'external',
        nsu: '004512890',
      })
    })

    expect(orderPostWithJWT).toHaveBeenCalledTimes(1)
    const [path, token, payload] = orderPostWithJWT.mock.calls[0] as [string, string, Record<string, unknown>]

    expect(path).toBe('/api/v1/orders')
    expect(token).toBe('jwt-token')
    expect(payload.channel).toBe('balcao')
    expect(payload.discountPct).toBe(10)

    // O endereço falso não pode voltar: hoje o backend responde 400.
    expect(payload).not.toHaveProperty('address')

    // Documento e telefone vão só em dígitos — o backend faz lookup por
    // igualdade exata e gravar mascarado quebraria o casamento.
    expect(payload.customerName).toBe('Construtora Aurora')
    expect(payload.customerDocument).toBe('11144477735')
    expect(payload.customerPhone).toBe('11988887777')
    expect(payload.customerId).toBe('cust-1')
  })

  it('cliente avulso (sem cadastro) vai sem customerId, só com o snapshot', async () => {
    orderPostWithJWT.mockResolvedValue({ id: 'ord-2', number: 'BAL-0002' })

    const { result } = renderHook(() => useBalcaoCheckout())
    const items = [item()]

    await act(async () => {
      await result.current.charge({
        items,
        pricing: pricingFor(items),
        customer: { ...CUSTOMER, id: undefined },
        method: 'external',
        nsu: '1',
      })
    })

    const payload = orderPostWithJWT.mock.calls[0][2] as Record<string, unknown>
    expect(payload).not.toHaveProperty('customerId')
    expect(payload.customerName).toBe('Construtora Aurora')
  })

  it('o approvalStatus do SERVIDOR vence a previsão local', async () => {
    // Local diria "dentro do teto" (5% <= 12%); o servidor segurou assim mesmo,
    // porque o teto do vínculo mudou no banco desde o login.
    orderPostWithJWT.mockResolvedValue({
      id: 'ord-3',
      number: 'BAL-0003',
      approvalStatus: 'pending',
      discountPct: 5,
      discountAmount: 10,
    })

    const { result } = renderHook(() => useBalcaoCheckout())
    const items = [item()]
    const pricing = pricingFor(items, 5)
    expect(pricing.requiresApproval).toBe(false) // previsão local

    await act(async () => {
      await result.current.charge({
        items,
        pricing,
        customer: CUSTOMER,
        method: 'external',
        nsu: '1',
      })
    })

    await waitFor(() => {
      expect(result.current.outcome?.requiresApproval).toBe(true)
    })
    expect(result.current.outcome?.approvalStatus).toBe('pending')
  })

  it('bloqueia a cobrança abaixo do custo antes de tocar o backend', async () => {
    const { result } = renderHook(() => useBalcaoCheckout())
    const items = [item()]

    await act(async () => {
      await result.current.charge({
        items,
        pricing: pricingFor(items, 60), // total 80 < custo 120
        customer: CUSTOMER,
        method: 'external',
        nsu: '1',
      })
    })

    expect(orderPostWithJWT).not.toHaveBeenCalled()
    expect(result.current.error).toMatch(/abaixo do custo/i)
  })

  it('erro do backend vira mensagem acionável no caixa', async () => {
    orderPostWithJWT.mockRejectedValue(
      new Error('estoque insuficiente para o produto p1: pedido 2, disponível 1'),
    )

    const { result } = renderHook(() => useBalcaoCheckout())
    const items = [item()]

    await act(async () => {
      await result.current.charge({
        items,
        pricing: pricingFor(items),
        customer: CUSTOMER,
        method: 'external',
        nsu: '1',
      })
    })

    await waitFor(() => {
      expect(result.current.error).toMatch(/estoque insuficiente/i)
    })
    expect(result.current.error).toMatch(/ajuste a quantidade/i)
  })
})

describe('describeOrderError', () => {
  it('reconhece estoque insuficiente e preserva o detalhe do backend', () => {
    const msg = describeOrderError(
      new Error('estoque insuficiente para o produto p1: pedido 2, disponível 1'),
    )
    expect(msg).toMatch(/ajuste a quantidade/i)
    expect(msg).toMatch(/disponível 1/)
  })

  it('traduz falta de vínculo de loja em instrução, não em jargão', () => {
    expect(describeOrderError(new Error('store operator role required'))).toMatch(
      /não está vinculada a uma loja/i,
    )
  })

  it('reconhece queda de rede', () => {
    expect(describeOrderError(new TypeError('Failed to fetch'))).toMatch(/sem conexão/i)
  })

  it('explica a recusa de endereço em pedido de balcão', () => {
    expect(
      describeOrderError(
        new Error('balcao orders are pickup-only and must not carry a shipping address'),
      ),
    ).toMatch(/não aceita endereço/i)
  })

  it('não engole um erro desconhecido', () => {
    expect(describeOrderError(new Error('algo bem específico quebrou'))).toBe(
      'algo bem específico quebrou',
    )
  })
})
