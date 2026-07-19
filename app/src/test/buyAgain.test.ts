import { describe, it, expect, vi } from 'vitest'
import { resolveBuyAgain, isNotFoundError, type ProductLookup } from '@/lib/buyAgain'
import { ApiError } from '@/lib/api'
import type { Product } from '@/types/product'
import type { OrderItem } from '@/lib/mockOrders'

function product(over: Partial<Product> = {}): Product {
  return {
    id: 'p1',
    name: 'Cimento CP II 50kg',
    slug: 'cimento-cp-ii-50kg',
    category: 'construcao',
    price: 42.9,
    currency: 'BRL',
    icon: '◫',
    seller: 'Casa & Obra',
    stock: 100,
    rating: 4,
    reviewCount: 10,
    ...over,
  }
}

function item(over: Partial<OrderItem> = {}): OrderItem {
  return {
    productId: 'p1',
    name: 'Cimento CP II 50kg',
    icon: '◫',
    sellerId: 's1',
    sellerName: 'Casa & Obra',
    quantity: 2,
    unitPrice: 38.5,
    ...over,
  }
}

/** Lookup de mentira: mapa id → produto, `null` explícito = fora de linha. */
function lookupFrom(map: Record<string, Product | null | Error>): ProductLookup {
  return async (id) => {
    const hit = map[id]
    if (hit instanceof Error) throw hit
    return hit ?? null
  }
}

describe('resolveBuyAgain — tudo disponível', () => {
  it('repõe todos os itens na quantidade original', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'a', quantity: 3 }), item({ productId: 'b', quantity: 1 })],
      lookupFrom({ a: product({ id: 'a', stock: 50 }), b: product({ id: 'b', stock: 50 }) })
    )

    expect(res.addedLines).toBe(2)
    expect(res.totalLines).toBe(2)
    expect(res.partial).toBe(false)
    expect(res.nothingAdded).toBe(false)
    expect(res.lines.map((l) => l.addedQty)).toEqual([3, 1])
    expect(res.lines.every((l) => l.outcome === 'added')).toBe(true)
  })
})

describe('resolveBuyAgain — item indisponível', () => {
  /**
   * REGRESSÃO: adicionar menos itens em silêncio.
   *
   * Modo de falha que isto previne: a recompra repunha o que dava e navegava
   * direto pro carrinho sem dizer nada. O cliente que pediu 5 itens fechava o
   * pedido achando que levou 5 e descobria a falta na obra. O resultado precisa
   * carregar o que NÃO entrou, item a item, e sinalizar `partial`.
   */
  it('marca parcial e discrimina esgotado de fora de linha', async () => {
    const res = await resolveBuyAgain(
      [
        item({ productId: 'ok1', name: 'Cimento', quantity: 2 }),
        item({ productId: 'ok2', name: 'Areia', quantity: 1 }),
        item({ productId: 'ok3', name: 'Cal', quantity: 1 }),
        item({ productId: 'zero', name: 'Argamassa', quantity: 4 }),
        item({ productId: 'sumiu', name: 'Broca antiga', quantity: 1 }),
      ],
      lookupFrom({
        ok1: product({ id: 'ok1', stock: 10 }),
        ok2: product({ id: 'ok2', stock: 10 }),
        ok3: product({ id: 'ok3', stock: 10 }),
        zero: product({ id: 'zero', stock: 0 }),
        sumiu: null,
      })
    )

    // O resumo que a UI mostra: "3 de 5 itens adicionados; 1 esgotado, 1 fora de linha"
    expect(res.addedLines).toBe(3)
    expect(res.totalLines).toBe(5)
    expect(res.partial).toBe(true)
    expect(res.outOfStock).toHaveLength(1)
    expect(res.outOfStock[0].name).toBe('Argamassa')
    expect(res.discontinued).toHaveLength(1)
    expect(res.discontinued[0].name).toBe('Broca antiga')
  })

  it('estoque zero não entra no carrinho', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'zero', quantity: 3 })],
      lookupFrom({ zero: product({ id: 'zero', stock: 0 }) })
    )

    expect(res.lines[0].outcome).toBe('out_of_stock')
    expect(res.lines[0].addedQty).toBe(0)
    expect(res.nothingAdded).toBe(true)
    expect(res.partial).toBe(false) // nada entrou: não é parcial, é falha total
  })

  it('reduz a quantidade ao estoque disponível em vez de estourar no checkout', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'p', quantity: 10 })],
      lookupFrom({ p: product({ id: 'p', stock: 4 }) })
    )

    expect(res.lines[0].outcome).toBe('reduced')
    expect(res.lines[0].addedQty).toBe(4)
    expect(res.lines[0].requestedQty).toBe(10)
    expect(res.reduced).toHaveLength(1)
    expect(res.addedLines).toBe(1)
  })

  it('produto fora de linha (404) vira discontinued, não erro da operação inteira', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'a' }), item({ productId: 'sumiu' })],
      lookupFrom({
        a: product({ id: 'a' }),
        sumiu: new ApiError({ error: 'produto não encontrado', code: 'not_found' }, 404),
      })
    )

    expect(res.lines[1].outcome).toBe('discontinued')
    expect(res.addedLines).toBe(1)
  })
})

describe('resolveBuyAgain — falha de consulta', () => {
  /**
   * REGRESSÃO: tratar erro de rede como "acabou o estoque".
   *
   * Um timeout do catálogo NÃO é informação sobre estoque. Se virasse
   * `out_of_stock`, a tela diria ao cliente que o produto acabou por causa de
   * um 500 momentâneo — e ele iria comprar em outro lugar.
   */
  it('erro de rede vira lookup_failed, nunca out_of_stock', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'boom' })],
      lookupFrom({ boom: new Error('Failed to fetch') })
    )

    expect(res.lines[0].outcome).toBe('lookup_failed')
    expect(res.failed).toHaveLength(1)
    expect(res.outOfStock).toHaveLength(0)
    expect(res.discontinued).toHaveLength(0)
    expect(res.lines[0].newPrice).toBeUndefined()
  })

  it('falha de um item não impede a reposição dos outros', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'a' }), item({ productId: 'boom' }), item({ productId: 'b' })],
      lookupFrom({
        a: product({ id: 'a', stock: 5 }),
        boom: new ApiError({ error: 'indisponível', code: 'db_error' }, 500),
        b: product({ id: 'b', stock: 5 }),
      })
    )

    expect(res.addedLines).toBe(2)
    expect(res.failed).toHaveLength(1)
    expect(res.partial).toBe(true)
  })

  it('consulta os itens em paralelo (pedido grande não vira barra de progresso)', async () => {
    let concurrent = 0
    let peak = 0
    const lookup: ProductLookup = async (id) => {
      concurrent += 1
      peak = Math.max(peak, concurrent)
      await new Promise((r) => setTimeout(r, 10))
      concurrent -= 1
      return product({ id })
    }

    await resolveBuyAgain(
      Array.from({ length: 5 }, (_, i) => item({ productId: `p${i}` })),
      lookup
    )

    expect(peak).toBeGreaterThan(1)
  })
})

describe('resolveBuyAgain — mudança de preço', () => {
  it('sinaliza item reposto cujo preço mudou desde a compra', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'p', unitPrice: 38.5 })],
      lookupFrom({ p: product({ id: 'p', price: 42.9, stock: 10 }) })
    )

    expect(res.priceChanged).toHaveLength(1)
    expect(res.lines[0].oldPrice).toBe(38.5)
    expect(res.lines[0].newPrice).toBe(42.9)
  })

  it('não acusa mudança quando o preço é o mesmo com ruído de float', async () => {
    // 38.5 e 38.50 são o mesmo dinheiro; comparação direta de float pode diferir.
    const res = await resolveBuyAgain(
      [item({ productId: 'p', unitPrice: 38.5 })],
      lookupFrom({ p: product({ id: 'p', price: 0.385 * 100, stock: 10 }) })
    )

    expect(res.priceChanged).toHaveLength(0)
  })

  it('não sinaliza preço de item que não entrou no carrinho', async () => {
    const res = await resolveBuyAgain(
      [item({ productId: 'zero', unitPrice: 10 })],
      lookupFrom({ zero: product({ id: 'zero', price: 999, stock: 0 }) })
    )

    expect(res.priceChanged).toHaveLength(0)
  })
})

describe('isNotFoundError', () => {
  it('reconhece o envelope do backend pelo code, não pelo texto', () => {
    expect(isNotFoundError(new ApiError({ error: 'qualquer texto', code: 'not_found' }, 404))).toBe(true)
    // Mesmo com mensagem reescrita, o code manda.
    expect(isNotFoundError(new ApiError({ error: 'sumiu o produto' }, 404))).toBe(true)
  })

  it('reconhece a sentinela lançada por catalogGet', () => {
    expect(isNotFoundError(new Error('not_found'))).toBe(true)
  })

  it('não confunde outros erros com 404', () => {
    expect(isNotFoundError(new ApiError({ error: 'x', code: 'db_error' }, 500))).toBe(false)
    expect(isNotFoundError(new Error('Failed to fetch'))).toBe(false)
    expect(isNotFoundError(new Error('produto not_found no servidor'))).toBe(false)
    expect(isNotFoundError(undefined)).toBe(false)
  })
})

describe('resolveBuyAgain — pedido vazio', () => {
  it('não quebra e não reporta nada como parcial', async () => {
    const lookup = vi.fn()
    const res = await resolveBuyAgain([], lookup)

    expect(res.totalLines).toBe(0)
    expect(res.partial).toBe(false)
    expect(res.nothingAdded).toBe(true)
    expect(lookup).not.toHaveBeenCalled()
  })
})
