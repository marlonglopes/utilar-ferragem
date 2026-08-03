import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

/**
 * Testa o CAMINHO REAL do uploader (isBulkImagesEnabled = true), que o teste em
 * mock não exercita: XHR, cabeçalho Authorization, campo "files", progresso,
 * refresh-on-401 e retry, erro→throw, e cancelamento via AbortSignal.
 * Um lote de centenas de fotos passa dos 15 min do access token, então o 401 no
 * meio do lote É o caminho normal — tem de estar coberto.
 */

interface ProgressEv {
  lengthComputable: boolean
  loaded: number
  total: number
}

class FakeXHR {
  static last: FakeXHR
  static onSend: (xhr: FakeXHR) => void = () => {}
  method = ''
  url = ''
  headers: Record<string, string> = {}
  timeout = 0
  status = 0
  responseText = ''
  sent: unknown = null
  aborted = false
  upload: { onprogress: ((e: ProgressEv) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  ontimeout: (() => void) | null = null
  onabort: (() => void) | null = null
  constructor() {
    FakeXHR.last = this
  }
  open(m: string, u: string) {
    this.method = m
    this.url = u
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v
  }
  send(body: unknown) {
    this.sent = body
    FakeXHR.onSend(this)
  }
  abort() {
    this.aborted = true
    this.onabort?.()
  }
}

const refreshAccessToken = vi.fn()

async function loadModule() {
  vi.resetModules()
  vi.stubEnv('VITE_CATALOG_URL', 'http://catalog.test')
  vi.doMock('@/lib/api', () => ({ refreshAccessToken }))
  vi.doMock('@/store/authStore', () => ({
    useAuthStore: { getState: () => ({ user: { token: 'jwt-1' } }) },
  }))
  vi.doMock('@/lib/adminApi', () => ({ adminGet: vi.fn() }))
  return import('@/lib/adminBulkImagesApi')
}

beforeEach(() => {
  refreshAccessToken.mockReset().mockResolvedValue('jwt-new')
  FakeXHR.onSend = () => {}
  ;(globalThis as unknown as { XMLHttpRequest: unknown }).XMLHttpRequest = FakeXHR
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.doUnmock('@/lib/api')
  vi.doUnmock('@/store/authStore')
  vi.doUnmock('@/lib/adminApi')
})

describe('uploadProductImage — caminho real (XHR)', () => {
  it('sucesso 200: URL, Authorization, campo "files" e progresso 100', async () => {
    const { uploadProductImage } = await loadModule()
    FakeXHR.onSend = (xhr) => {
      xhr.upload.onprogress?.({ lengthComputable: true, loaded: 50, total: 100 })
      xhr.status = 200
      xhr.onload?.()
    }
    const prog: number[] = []
    const file = new File(['x'], '6320.jpg', { type: 'image/jpeg' })
    await uploadProductImage('prod-9', file, (p) => prog.push(p))

    expect(FakeXHR.last.method).toBe('POST')
    expect(FakeXHR.last.url).toBe(
      'http://catalog.test/api/v1/admin/products/by-id/prod-9/images/upload'
    )
    expect(FakeXHR.last.headers['Authorization']).toBe('Bearer jwt-1')
    expect(FakeXHR.last.sent).toBeInstanceOf(FormData)
    expect((FakeXHR.last.sent as FormData).get('files')).toBeInstanceOf(File)
    expect(prog).toContain(50)
    expect(prog.at(-1)).toBe(100)
  })

  it('401 no meio do lote: renova o token e retenta com o novo Bearer', async () => {
    const { uploadProductImage } = await loadModule()
    const seenTokens: string[] = []
    let call = 0
    FakeXHR.onSend = (xhr) => {
      seenTokens.push(xhr.headers['Authorization'])
      call++
      xhr.status = call === 1 ? 401 : 200
      xhr.onload?.()
    }
    await uploadProductImage('p', new File(['x'], '6320.jpg'), () => {})

    expect(refreshAccessToken).toHaveBeenCalledTimes(1)
    expect(seenTokens).toEqual(['Bearer jwt-1', 'Bearer jwt-new'])
  })

  it('erro 500 com {error}: lança a mensagem do backend', async () => {
    const { uploadProductImage } = await loadModule()
    FakeXHR.onSend = (xhr) => {
      xhr.status = 500
      xhr.responseText = JSON.stringify({ error: 'arquivo corrompido' })
      xhr.onload?.()
    }
    await expect(
      uploadProductImage('p', new File(['x'], '6320.jpg'), () => {})
    ).rejects.toThrow('arquivo corrompido')
  })

  it('erro sem corpo JSON: lança "HTTP <status>"', async () => {
    const { uploadProductImage } = await loadModule()
    FakeXHR.onSend = (xhr) => {
      xhr.status = 502
      xhr.responseText = '<html>bad gateway</html>'
      xhr.onload?.()
    }
    await expect(
      uploadProductImage('p', new File(['x'], '6320.jpg'), () => {})
    ).rejects.toThrow('HTTP 502')
  })

  it('cancelar em voo (AbortSignal) rejeita com AbortError', async () => {
    const { uploadProductImage } = await loadModule()
    FakeXHR.onSend = () => {} // deixa pendente
    const ctrl = new AbortController()
    const p = uploadProductImage('p', new File(['x'], '6320.jpg'), () => {}, ctrl.signal)
    ctrl.abort()
    await expect(p).rejects.toMatchObject({ name: 'AbortError' })
  })

  it('signal já abortado antes do envio não sobe nada', async () => {
    const { uploadProductImage } = await loadModule()
    let sent = false
    FakeXHR.onSend = () => {
      sent = true
    }
    const ctrl = new AbortController()
    ctrl.abort()
    const p = uploadProductImage('p', new File(['x'], '6320.jpg'), () => {}, ctrl.signal)
    await expect(p).rejects.toMatchObject({ name: 'AbortError' })
    expect(sent).toBe(false)
  })
})

describe('resolveBySku — caminho real (HTTP)', () => {
  it('chama by-sku (dedup na query) e devolve data', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_CATALOG_URL', 'http://catalog.test')
    const adminGet = vi
      .fn()
      .mockResolvedValue({ data: [{ sku: '6320', id: 'p1', name: 'X', hasImage: false }] })
    vi.doMock('@/lib/adminApi', () => ({ adminGet }))
    vi.doMock('@/lib/api', () => ({ refreshAccessToken: vi.fn() }))
    vi.doMock('@/store/authStore', () => ({ useAuthStore: { getState: () => ({ user: null }) } }))
    const { resolveBySku } = await import('@/lib/adminBulkImagesApi')

    const r = await resolveBySku(['6320', '7492'])
    expect(adminGet).toHaveBeenCalledTimes(1)
    const path = adminGet.mock.calls[0][1] as string
    expect(path).toContain('/api/v1/admin/products/by-sku?skus=')
    expect(r).toHaveLength(1)
    expect(r[0].sku).toBe('6320')
  })

  it('resposta sem data → []', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_CATALOG_URL', 'http://catalog.test')
    vi.doMock('@/lib/adminApi', () => ({ adminGet: vi.fn().mockResolvedValue({}) }))
    vi.doMock('@/lib/api', () => ({ refreshAccessToken: vi.fn() }))
    vi.doMock('@/store/authStore', () => ({ useAuthStore: { getState: () => ({ user: null }) } }))
    const { resolveBySku } = await import('@/lib/adminBulkImagesApi')
    expect(await resolveBySku(['6320'])).toEqual([])
  })
})
