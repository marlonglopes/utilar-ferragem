const BASE_URL = import.meta.env.VITE_API_URL ?? ''
const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
const AUTH_URL = import.meta.env.VITE_AUTH_URL ?? ''

export const isApiEnabled = BASE_URL !== ''
export const isCatalogEnabled = CATALOG_URL !== ''
export const isOrderEnabled = ORDER_URL !== ''
export const isAuthEnabled = AUTH_URL !== ''

/** Envelope de erro do backend: `{error, code, requestId, details?}`. */
interface ApiErrorEnvelope {
  error: string
  code?: string
  requestId?: string
  details?: unknown
  messages?: string[]
}

/**
 * Erro de API que preserva `code` e `details` do envelope do backend.
 *
 * PORQUÊ: antes daqui, `handleResponse` fazia `throw new Error(body.error)` e
 * jogava fora `code` e `details`. Quem precisava distinguir um erro do outro
 * tinha que casar TEXTO da mensagem — foi o que aconteceu no PDV do balcão com
 * `insufficient_stock`, que chega com o item e o saldo em `details` e só era
 * reconhecido por substring. Funciona até alguém reescrever a mensagem.
 *
 * Estende Error de propósito: todo `catch (err) { err.message }` e
 * `err instanceof Error` que já existe continua funcionando igual.
 */
export class ApiError extends Error {
  readonly code?: string
  readonly status: number
  readonly requestId?: string
  readonly details?: unknown

  constructor(envelope: ApiErrorEnvelope, status: number) {
    super(envelope.messages?.join(', ') ?? envelope.error ?? `HTTP ${status}`)
    this.name = 'ApiError'
    this.code = envelope.code
    this.status = status
    this.requestId = envelope.requestId
    this.details = envelope.details
  }

  /** `true` se o backend classificou o erro com este código. */
  is(code: string): boolean {
    return this.code === code
  }
}

/** Type guard — evita `instanceof` espalhado com import de tipo. */
export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}

/**
 * Lê `details` com um formato esperado, sem `any`.
 * Devolve `undefined` se o erro não for ApiError ou o código não bater — assim
 * quem consome não precisa encadear checagem.
 */
export function apiErrorDetails<T>(err: unknown, code: string): T | undefined {
  if (!isApiError(err) || !err.is(code)) return undefined
  return err.details as T | undefined
}

// Hooks injetados pelo App em runtime — evita ciclo de dependência com o store
// e mantém api.ts isolado de React. Sem hook setado, 401 só "throw" como antes.
type AuthHooks = {
  getToken: () => string | null
  getRefreshToken: () => string | null
  setAccessToken: (token: string) => void
  clearSession: () => void
}
let authHooks: AuthHooks | null = null
export function configureAuthHooks(hooks: AuthHooks) {
  authHooks = hooks
}

// Estado pra evitar refresh paralelo: se múltiplas requests dispararem 401
// simultâneas, só um refresh acontece e os outros aguardam o mesmo Promise.
let inflightRefresh: Promise<string | null> | null = null

/**
 * Renova o access token usando o refresh token, uma vez por vez.
 *
 * Exportada porque o cliente de admin (`adminApi.ts`) precisa da MESMA
 * renovação: o access token dura 15 minutos, e sem isso o painel expirava
 * sozinho no meio do uso — "Sua sessão expirou" ao clicar num produto, mesmo
 * com a aba aberta e o login válido.
 *
 * `inflightRefresh` garante uma renovação só mesmo com várias chamadas
 * paralelas (o painel dispara 3-4 de uma vez): sem isso, a primeira resposta
 * invalidaria o refresh token que as outras ainda estão usando.
 */
export async function refreshAccessToken(): Promise<string | null> {
  return tryRefreshToken()
}

async function tryRefreshToken(): Promise<string | null> {
  if (!authHooks) return null
  if (inflightRefresh) return inflightRefresh
  const refreshToken = authHooks.getRefreshToken()
  if (!refreshToken) return null

  inflightRefresh = (async () => {
    try {
      const res = await fetch(`${AUTH_URL}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken }),
      })
      if (!res.ok) {
        authHooks?.clearSession()
        return null
      }
      const data = (await res.json()) as { accessToken?: string }
      if (!data.accessToken) {
        authHooks?.clearSession()
        return null
      }
      authHooks?.setAccessToken(data.accessToken)
      return data.accessToken
    } catch {
      authHooks?.clearSession()
      return null
    } finally {
      inflightRefresh = null
    }
  })()
  return inflightRefresh
}

// ---------------------------------------------------------------------------
// Idempotency-Key
//
// order-service e payment-service montam o middleware `pkg/idempotency` nos
// POSTs de criação, mas o SPA nunca mandava o header — ou seja, a proteção
// inteira era código morto e um duplo clique no checkout criava pedido e
// cobrança duplicados. O middleware é opt-in pelo cliente: sem header, ele
// passa direto (pkg/idempotency/store.go).
//
// ESTRATÉGIA DA CHAVE — derivada do conteúdo, com memo de curta duração.
//
// Gerar `crypto.randomUUID()` a cada chamada NÃO resolveria nada: duas chamadas
// do mesmo duplo clique teriam chaves diferentes e o servidor trataria as duas
// como operações distintas. A chave precisa ser estável no retry da MESMA
// operação e diferente entre operações distintas.
//
// Receber a chave de fora seria mais explícito, mas os dois call sites
// (CheckoutPage, usePayment) pertencem a outros donos e a mudança forçaria
// edição neles. Então derivamos: (rota + corpo canônico) identifica a operação,
// e um memo module-level guarda o UUID sorteado pra essa identidade.
//
// O memo tem TTL curto (IDEMPOTENCY_MEMO_TTL_MS) e NÃO é renovado a cada hit.
// Motivo: o servidor guarda a chave por 24h, e uma segunda compra legítima do
// mesmo carrinho tem exatamente o mesmo corpo. Sem expirar, ela seria engolida
// como replay. Dois minutos cobrem duplo clique, retry de rede e reenvio
// impaciente do usuário — e deixam passar a recompra deliberada mais tarde.
//
// O valor do header é um UUID (não o hash), como o middleware espera: string
// opaca de 8..128 chars. O hash fica só no lado do cliente, como chave do memo.
const IDEMPOTENT_CREATE_PATHS = new Set(['/api/v1/orders', '/api/v1/payments'])

const IDEMPOTENCY_MEMO_TTL_MS = 2 * 60 * 1000

const idempotencyMemo = new Map<string, { key: string; expiresAt: number }>()

function randomKey(): string {
  // crypto.randomUUID exige contexto seguro (https/localhost) e não existe em
  // jsdom antigo — fallback mantém o header válido em vez de sumir com ele.
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `idem-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`
}

// stableStringify serializa com as chaves ordenadas: dois objetos equivalentes
// montados em ordens diferentes precisam gerar a MESMA identidade.
function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'null'
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, v]) => v !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([k, v]) => `${JSON.stringify(k)}:${stableStringify(v)}`)
  return `{${entries.join(',')}}`
}

function idempotencyKeyFor(path: string, body: unknown): string {
  const identity = `${path}|${stableStringify(body)}`
  const now = Date.now()
  const hit = idempotencyMemo.get(identity)
  if (hit && hit.expiresAt > now) return hit.key

  // Varredura barata de expirados — o mapa nunca passa de alguns itens por sessão.
  for (const [k, v] of idempotencyMemo) {
    if (v.expiresAt <= now) idempotencyMemo.delete(k)
  }

  const key = randomKey()
  idempotencyMemo.set(identity, { key, expiresAt: now + IDEMPOTENCY_MEMO_TTL_MS })
  return key
}

// applyIdempotency injeta o header nos POSTs de criação. Chave explícita do
// caller sempre ganha; senão, deriva do conteúdo.
function applyIdempotency(
  headers: Record<string, string>,
  path: string,
  body: unknown,
  explicitKey?: string,
): void {
  if (explicitKey) {
    headers['Idempotency-Key'] = explicitKey
    return
  }
  if (IDEMPOTENT_CREATE_PATHS.has(path)) {
    headers['Idempotency-Key'] = idempotencyKeyFor(path, body)
  }
}

// Exportado só pra teste: garante isolamento entre casos.
export function __resetIdempotencyMemo(): void {
  idempotencyMemo.clear()
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body: ApiErrorEnvelope = await res
      .json()
      .catch(() => ({ error: `HTTP ${res.status}` }))
    throw new ApiError(body, res.status)
  }
  return res.json() as Promise<T>
}

// Wrapper que tenta refresh se a resposta for 401 e há refreshToken disponível.
// Retorna a Response final (já com retry feito ou a 401 original se refresh falhou).
async function fetchWithAutoRefresh(
  doFetch: (token: string | null) => Promise<Response>,
  initialToken?: string,
): Promise<Response> {
  const tokenAttempt1 = initialToken ?? null
  let res = await doFetch(tokenAttempt1)
  if (res.status !== 401 || !authHooks) return res
  // 401 — tenta um refresh + retry uma única vez. Se falhar, clearSession já foi
  // chamado por tryRefreshToken e a Response 401 cai pro caller normalmente.
  const newToken = await tryRefreshToken()
  if (!newToken) return res
  res = await doFetch(newToken)
  return res
}

// `idempotencyKey` é opcional e retrocompatível: sem ele, POSTs de criação
// (ver IDEMPOTENT_CREATE_PATHS) derivam a chave do conteúdo automaticamente.
export async function apiPost<T>(
  path: string,
  body: unknown,
  token?: string,
  idempotencyKey?: string,
): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
    applyIdempotency(headers, path, body, idempotencyKey)
    return fetch(`${BASE_URL}${path}`, { method: 'POST', headers, body: JSON.stringify(body) })
  }, token)
  return handleResponse<T>(res)
}

export async function apiGet<T>(path: string, token?: string): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = {}
    if (tok) headers['Authorization'] = `Bearer ${tok}`
    return fetch(`${BASE_URL}${path}`, { headers })
  }, token)
  return handleResponse<T>(res)
}

export async function apiPatch<T>(path: string, body: unknown, token?: string): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
    return fetch(`${BASE_URL}${path}`, { method: 'PATCH', headers, body: JSON.stringify(body) })
  }, token)
  return handleResponse<T>(res)
}

// Catalog API — leitura pública, sem auth. Base URL independente (VITE_CATALOG_URL).
export async function catalogGet<T>(path: string): Promise<T> {
  const res = await fetch(`${CATALOG_URL}${path}`)
  if (res.status === 404) throw new Error('not_found')
  return handleResponse<T>(res)
}

// Order API — requer X-User-Id (substitui JWT até auth-service ficar pronto).
function orderHeaders(userId: string, contentType = false): Record<string, string> {
  const h: Record<string, string> = { 'X-User-Id': userId }
  if (contentType) h['Content-Type'] = 'application/json'
  return h
}

export async function orderGet<T>(path: string, userId: string): Promise<T> {
  const res = await fetch(`${ORDER_URL}${path}`, { headers: orderHeaders(userId) })
  if (res.status === 404) throw new Error('not_found')
  return handleResponse<T>(res)
}

export async function orderPost<T>(
  path: string,
  userId: string,
  body: unknown,
  idempotencyKey?: string,
): Promise<T> {
  const headers = orderHeaders(userId, true)
  applyIdempotency(headers, path, body, idempotencyKey)
  const res = await fetch(`${ORDER_URL}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

export async function orderPatch<T>(path: string, userId: string, body: unknown = {}): Promise<T> {
  const res = await fetch(`${ORDER_URL}${path}`, {
    method: 'PATCH',
    headers: orderHeaders(userId, true),
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

// Auth API — público (register/login/refresh) e protegido (me/addresses/logout)
// via Authorization: Bearer <jwt>.
function authHeaders(token?: string, contentType = false): Record<string, string> {
  const h: Record<string, string> = {}
  if (contentType) h['Content-Type'] = 'application/json'
  if (token) h['Authorization'] = `Bearer ${token}`
  return h
}

export async function authPost<T>(path: string, body: unknown, token?: string): Promise<T> {
  const res = await fetch(`${AUTH_URL}${path}`, {
    method: 'POST',
    headers: authHeaders(token, true),
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

export async function authGet<T>(path: string, token: string): Promise<T> {
  const res = await fetch(`${AUTH_URL}${path}`, { headers: authHeaders(token) })
  if (res.status === 404) throw new Error('not_found')
  return handleResponse<T>(res)
}

// Quando auth-service está ligado, o frontend passa o JWT como Bearer para order-service.
// Essa helper troca X-User-Id por Authorization quando aplicável.
// Usa fetchWithAutoRefresh: 401 dispara refresh+retry automático.
export async function orderGetWithJWT<T>(path: string, token: string): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    return fetch(`${ORDER_URL}${path}`, {
      headers: tok ? { Authorization: `Bearer ${tok}` } : {},
    })
  }, token)
  if (res.status === 404) throw new Error('not_found')
  return handleResponse<T>(res)
}

export async function orderPostWithJWT<T>(
  path: string,
  token: string,
  body: unknown,
  idempotencyKey?: string,
): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
    applyIdempotency(headers, path, body, idempotencyKey)
    return fetch(`${ORDER_URL}${path}`, { method: 'POST', headers, body: JSON.stringify(body) })
  }, token)
  return handleResponse<T>(res)
}

export async function orderPatchWithJWT<T>(path: string, token: string, body: unknown = {}): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
    return fetch(`${ORDER_URL}${path}`, { method: 'PATCH', headers, body: JSON.stringify(body) })
  }, token)
  return handleResponse<T>(res)
}
