const BASE_URL = import.meta.env.VITE_API_URL ?? ''
const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''

export const isApiEnabled = BASE_URL !== ''
export const isCatalogEnabled = CATALOG_URL !== ''

interface ApiError {
  error: string
  messages?: string[]
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body: ApiError = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
    throw new Error(body.messages?.join(', ') ?? body.error)
  }
  return res.json() as Promise<T>
}

export async function apiPost<T>(path: string, body: unknown, token?: string): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

export async function apiGet<T>(path: string, token?: string): Promise<T> {
  const headers: Record<string, string> = {}
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(`${BASE_URL}${path}`, { headers })
  return handleResponse<T>(res)
}

export async function apiPatch<T>(path: string, body: unknown, token?: string): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'PATCH',
    headers,
    body: JSON.stringify(body),
  })
  return handleResponse<T>(res)
}

// Catalog API — leitura pública, sem auth. Base URL independente (VITE_CATALOG_URL).
export async function catalogGet<T>(path: string): Promise<T> {
  const res = await fetch(`${CATALOG_URL}${path}`)
  if (res.status === 404) throw new Error('not_found')
  return handleResponse<T>(res)
}
