import { describe, it, expect } from 'vitest'
import { ApiError, isApiError, apiErrorDetails } from '@/lib/api'
import { describeOrderError } from '@/hooks/useBalcaoCheckout'

/**
 * Regressão: `handleResponse` fazia `throw new Error(body.error)` e descartava
 * `code` e `details` do envelope do backend. Quem precisava distinguir um erro
 * do outro casava TEXTO da mensagem.
 *
 * O caso concreto era o PDV do balcão: `insufficient_stock` chega com o produto
 * e o saldo em `details`, e era reconhecido por `/estoque insuficiente/i`. Bastava
 * alguém reescrever a mensagem no backend para o vendedor passar a ver o erro
 * cru, no meio de uma venda, com o cliente esperando no caixa.
 */
describe('ApiError', () => {
  const envelope = {
    error: 'estoque insuficiente para o produto p-1: pedido 5, disponível 3',
    code: 'insufficient_stock',
    requestId: '01KXWG4DNZ',
    details: { productId: 'p-1', requested: 5, available: 3 },
  }

  it('preserva code, status, requestId e details', () => {
    const err = new ApiError(envelope, 409)
    expect(err.code).toBe('insufficient_stock')
    expect(err.status).toBe(409)
    expect(err.requestId).toBe('01KXWG4DNZ')
    expect(err.details).toEqual({ productId: 'p-1', requested: 5, available: 3 })
  })

  it('continua sendo um Error — não quebra catch existente', () => {
    const err = new ApiError(envelope, 409)
    // Todo `catch (e) { e.message }` e `e instanceof Error` do app segue igual.
    expect(err).toBeInstanceOf(Error)
    expect(err.message).toContain('estoque insuficiente')
  })

  it('is() classifica pelo código', () => {
    const err = new ApiError(envelope, 409)
    expect(err.is('insufficient_stock')).toBe(true)
    expect(err.is('not_found')).toBe(false)
  })

  it('cai na mensagem quando o backend não classifica', () => {
    const err = new ApiError({ error: 'algo quebrou' }, 500)
    expect(err.code).toBeUndefined()
    expect(err.message).toBe('algo quebrou')
  })

  it('apiErrorDetails só devolve quando o código bate', () => {
    const err = new ApiError(envelope, 409)
    expect(apiErrorDetails(err, 'insufficient_stock')).toEqual(envelope.details)
    expect(apiErrorDetails(err, 'not_found')).toBeUndefined()
    expect(apiErrorDetails(new Error('comum'), 'insufficient_stock')).toBeUndefined()
    expect(apiErrorDetails(null, 'insufficient_stock')).toBeUndefined()
  })

  it('isApiError distingue de Error comum', () => {
    expect(isApiError(new ApiError(envelope, 409))).toBe(true)
    expect(isApiError(new Error('comum'))).toBe(false)
    expect(isApiError('texto')).toBe(false)
  })
})

describe('describeOrderError — classifica por code, não por texto', () => {
  it('usa o saldo real de details em vez de repetir a mensagem crua', () => {
    const err = new ApiError(
      {
        error: 'mensagem que o backend pode reescrever a qualquer momento',
        code: 'insufficient_stock',
        details: { productId: 'p-1', requested: 5, available: 3 },
      },
      409,
    )
    const msg = describeOrderError(err)
    // "tem 3, você pediu 5" é acionável no balcão; "não deu" não é.
    expect(msg).toContain('3')
    expect(msg).toContain('5')
    expect(msg).toMatch(/ajuste a quantidade/i)
  })

  it('reconhece estoque mesmo com a mensagem em outro idioma', () => {
    // O ponto da mudança: a classificação não depende mais do texto.
    const err = new ApiError(
      { error: 'not enough stock', code: 'insufficient_stock', details: { available: 0 } },
      409,
    )
    expect(describeOrderError(err)).toMatch(/estoque insuficiente/i)
  })

  it('traduz sessão expirada e falta de permissão', () => {
    expect(describeOrderError(new ApiError({ error: 'x', code: 'unauthorized' }, 401)))
      .toMatch(/sessão expirada/i)
    expect(describeOrderError(new ApiError({ error: 'x', code: 'forbidden' }, 403)))
      .toMatch(/permissão/i)
  })

  it('mantém o texto quando é validação — o campo diz mais que o código', () => {
    const err = new ApiError(
      { error: 'Telefone do cliente é obrigatório', code: 'validation_error' },
      400,
    )
    expect(describeOrderError(err)).toMatch(/telefone/i)
  })

  it('ainda trata erro de rede, que não tem envelope', () => {
    expect(describeOrderError(new TypeError('Failed to fetch')))
      .toMatch(/sem conexão/i)
  })

  it('nunca devolve string vazia', () => {
    expect(describeOrderError(null)).toBeTruthy()
    expect(describeOrderError(new Error(''))).toBeTruthy()
  })
})
