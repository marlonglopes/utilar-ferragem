import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { apiPost, orderPostWithJWT, __resetIdempotencyMemo } from '@/lib/api'

// Regressão: `grep -rn "Idempotency-Key" app/src/` não retornava nada — o SPA
// nunca mandava o header, então o middleware pkg/idempotency montado no
// order-service e no payment-service era código morto e duplo clique no
// checkout criava pedido + cobrança duplicados.
//
// Estes testes simulam o servidor real: o middleware dedupe por chave, então
// aqui contamos criações por chave distinta vista pelo "servidor".

type Created = { id: string }

/** Fake do middleware pkg/idempotency: mesma chave → replay, sem criar de novo. */
function idempotentServer() {
  const cache = new Map<string, Created>()
  const seenKeys: (string | null)[] = []
  let creations = 0

  const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
    const headers = (init?.headers ?? {}) as Record<string, string>
    const key = headers['Idempotency-Key'] ?? null
    seenKeys.push(key)

    if (key && cache.has(key)) {
      // replay: o servidor devolve a MESMA resposta, sem reexecutar o handler
      return new Response(JSON.stringify(cache.get(key)), { status: 200 })
    }
    creations += 1
    const body: Created = { id: `order-${creations}` }
    if (key) cache.set(key, body)
    return new Response(JSON.stringify(body), { status: 200 })
  })

  return {
    fetchMock,
    seenKeys,
    get creations() {
      return creations
    },
  }
}

describe('Idempotency-Key no SPA', () => {
  let server: ReturnType<typeof idempotentServer>

  beforeEach(() => {
    __resetIdempotencyMemo()
    server = idempotentServer()
    vi.stubGlobal('fetch', server.fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  const payload = { items: [{ sku: 'martelo', qty: 1 }], total: 4990 }

  it('manda o header nos POSTs de criação de pedido', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)
    expect(server.seenKeys[0]).toBeTruthy()
  })

  it('manda o header nos POSTs de criação de pagamento', async () => {
    await apiPost('/api/v1/payments', { order_id: 'o1', method: 'pix', amount: 4990 }, 'jwt')
    expect(server.seenKeys[0]).toBeTruthy()
  })

  // O caso que motivou tudo: duplo clique no botão de finalizar compra.
  it('duplo clique reusa a chave — servidor não cria um segundo pedido', async () => {
    const [a, b] = await Promise.all([
      orderPostWithJWT<Created>('/api/v1/orders', 'jwt', payload),
      orderPostWithJWT<Created>('/api/v1/orders', 'jwt', payload),
    ])

    expect(server.seenKeys[0]).toBe(server.seenKeys[1])
    expect(server.creations).toBe(1)
    expect(a.id).toBe(b.id)
  })

  // Chave nova a cada chamada (o jeito ingênuo) não protegeria nada — este
  // teste trava o comportamento oposto: operações DIFERENTES têm chaves
  // diferentes e devem criar dois recursos.
  it('carrinhos diferentes geram chaves diferentes — duas criações', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)
    await orderPostWithJWT('/api/v1/orders', 'jwt', { ...payload, total: 9990 })

    expect(server.seenKeys[0]).not.toBe(server.seenKeys[1])
    expect(server.creations).toBe(2)
  })

  it('pedido e pagamento não compartilham chave', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)
    await apiPost('/api/v1/payments', payload, 'jwt')

    expect(server.seenKeys[0]).not.toBe(server.seenKeys[1])
  })

  it('a identidade não depende da ordem das chaves do objeto', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', { a: 1, b: 2 })
    await orderPostWithJWT('/api/v1/orders', 'jwt', { b: 2, a: 1 })

    expect(server.seenKeys[0]).toBe(server.seenKeys[1])
    expect(server.creations).toBe(1)
  })

  // A chave expira: recompra deliberada do mesmo carrinho mais tarde precisa
  // passar, senão o servidor (que guarda a chave por 24h) engoliria o pedido.
  it('depois do TTL do memo, o mesmo carrinho vira uma nova compra', async () => {
    vi.useFakeTimers()
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)
    vi.advanceTimersByTime(3 * 60 * 1000)
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)

    expect(server.seenKeys[0]).not.toBe(server.seenKeys[1])
    expect(server.creations).toBe(2)
  })

  // O header é opt-in por rota: POSTs que não criam recurso continuam sem ele.
  it('não manda o header em POSTs que não criam recurso', async () => {
    await apiPost('/auth/forgot-password', { email: 'a@b.com' })
    expect(server.seenKeys[0]).toBeNull()
  })

  it('chave explícita do caller tem prioridade', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload, 'chave-explicita-123')
    expect(server.seenKeys[0]).toBe('chave-explicita-123')
  })

  // pkg/idempotency/store.go rejeita chave fora de 8..128 chars.
  it('a chave derivada respeita o formato aceito pelo middleware', async () => {
    await orderPostWithJWT('/api/v1/orders', 'jwt', payload)
    const key = server.seenKeys[0] as string
    expect(key.length).toBeGreaterThanOrEqual(8)
    expect(key.length).toBeLessThanOrEqual(128)
  })
})
