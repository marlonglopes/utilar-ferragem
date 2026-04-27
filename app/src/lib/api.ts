const BASE_URL = import.meta.env.VITE_API_URL ?? ''
const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
const ORDER_URL = import.meta.env.VITE_ORDER_URL ?? ''
const AUTH_URL = import.meta.env.VITE_AUTH_URL ?? ''

export const isApiEnabled = BASE_URL !== ''
export const isCatalogEnabled = CATALOG_URL !== ''
export const isOrderEnabled = ORDER_URL !== ''
export const isAuthEnabled = AUTH_URL !== ''

interface ApiError {
  error: string
  messages?: string[]
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

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body: ApiError = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
    throw new Error(body.messages?.join(', ') ?? body.error)
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

export async function apiPost<T>(path: string, body: unknown, token?: string): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
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

export async function orderPost<T>(path: string, userId: string, body: unknown): Promise<T> {
  const res = await fetch(`${ORDER_URL}${path}`, {
    method: 'POST',
    headers: orderHeaders(userId, true),
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

export async function orderPostWithJWT<T>(path: string, token: string, body: unknown): Promise<T> {
  const res = await fetchWithAutoRefresh((tok) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (tok) headers['Authorization'] = `Bearer ${tok}`
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
