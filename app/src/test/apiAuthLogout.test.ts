import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { authLogout } from '@/lib/api'

/**
 * Contrato HTTP de `POST /api/v1/auth/logout` (auth-service).
 *
 * Lido de `services/auth-service/internal/handler/auth.go` + `cmd/server/main.go`:
 * - rota no grupo `priv` ⇒ exige `Authorization: Bearer <access token>`;
 * - corpo `model.RefreshRequest` ⇒ `{ "refreshToken": "..." }`;
 * - resposta **204 No Content**, sem corpo.
 */

const AUTH_URL = 'http://auth.test'
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.resetModules()
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
  vi.stubEnv('VITE_AUTH_URL', AUTH_URL)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

/** Reimporta o módulo para que ele leia a env stubada em tempo de carga. */
async function freshApi() {
  return import('@/lib/api')
}

function noContent() {
  // 204 de verdade: sem corpo. `res.json()` aqui estoura.
  return new Response(null, { status: 204 })
}

describe('authLogout — contrato com o auth-service', () => {
  it('faz POST para /api/v1/auth/logout', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(noContent())

    await api.authLogout('access-123', 'refresh-456')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe(`${AUTH_URL}/api/v1/auth/logout`)
    expect(init.method).toBe('POST')
  })

  it('manda o access token no header — a rota é autenticada', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(noContent())

    await api.authLogout('access-123', 'refresh-456')

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers['Authorization']).toBe('Bearer access-123')
    expect(init.headers['Content-Type']).toBe('application/json')
  })

  it('manda o refreshToken no corpo — sem ele o servidor não revoga nada', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(noContent())

    await api.authLogout('access-123', 'refresh-456')

    const [, init] = fetchMock.mock.calls[0]
    // O handler faz bind de RefreshRequest; corpo sem esse campo cai no ramo
    // que devolve 204 SEM revogar — falha silenciosa.
    expect(JSON.parse(init.body)).toEqual({ refreshToken: 'refresh-456' })
  })

  /**
   * REGRESSÃO: tratar 204 com um leitor de JSON.
   *
   * O caminho óbvio seria reusar `authPost`, mas ele termina em
   * `handleResponse` → `res.json()`. Num 204 (corpo vazio) isso lança
   * `SyntaxError: Unexpected end of JSON input` — a revogação funcionaria no
   * servidor e explodiria no cliente, no meio do logout.
   */
  it('lida com 204 No Content sem tentar parsear corpo', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(noContent())

    await expect(api.authLogout('access-123', 'refresh-456')).resolves.toBe(true)
  })

  it('nunca lança quando a rede cai', async () => {
    const api = await freshApi()
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    // Se lançasse, derrubaria o logout inteiro e prenderia o usuário logado.
    await expect(api.authLogout('access-123', 'refresh-456')).resolves.toBe(false)
  })

  it('nunca lança quando o servidor responde erro', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(new Response('{"error":"unauthorized"}', { status: 401 }))

    await expect(api.authLogout('access-123', 'refresh-456')).resolves.toBe(false)
  })

  it('não chama a rede sem access token — não há o que revogar', async () => {
    const api = await freshApi()

    await expect(api.authLogout(null, 'refresh-456')).resolves.toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('tolera refresh token ausente', async () => {
    const api = await freshApi()
    fetchMock.mockResolvedValue(noContent())

    await api.authLogout('access-123', null)

    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(init.body)).toEqual({ refreshToken: '' })
  })
})

describe('authLogout — modo mock (sem auth-service)', () => {
  it('não tenta revogar quando não há auth-service configurado', async () => {
    vi.stubEnv('VITE_AUTH_URL', '')
    vi.resetModules()
    const api = await import('@/lib/api')

    await expect(api.authLogout('access-123', 'refresh-456')).resolves.toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('authLogout — módulo real exporta a função', () => {
  it('está disponível para o hook de logout', () => {
    expect(typeof authLogout).toBe('function')
  })
})
