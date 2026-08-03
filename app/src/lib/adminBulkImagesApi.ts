// Cliente do UPLOADER DE IMAGENS EM LOTE. Casa SKUs a produtos e sobe cada foto
// pelo mesmo endpoint de upload que já existe (normalização no backend). Upload
// via XHR para ter PROGRESSO (fetch não expõe progresso de upload fácil), com
// refresh-on-401 (um lote de centenas de fotos passa dos 15 min do access token).
import { adminGet } from '@/lib/adminApi'
import { refreshAccessToken } from '@/lib/api'
import { useAuthStore } from '@/store/authStore'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''
export const isBulkImagesEnabled = CATALOG_URL !== ''

export interface SkuMatch {
  sku: string
  id: string
  name: string
  hasImage: boolean
}

/**
 * Pool de concorrência: roda `worker` sobre `items` com no máximo `concurrency`
 * em voo. É o que sobe as fotos em paralelo sem estourar o navegador/servidor
 * com centenas de uploads simultâneos. Todos os itens são processados; a espera
 * total é a do runner mais lento, não a soma.
 */
export async function runPool<T>(
  items: T[],
  worker: (t: T) => Promise<void>,
  concurrency: number
): Promise<void> {
  let idx = 0
  const runners = Array.from({ length: Math.min(Math.max(1, concurrency), items.length) }, async () => {
    while (idx < items.length) {
      const i = idx++
      await worker(items[i])
    }
  })
  await Promise.all(runners)
}

/** Base do nome do arquivo: tira caminho e extensão. */
function fileBase(name: string): string {
  return name
    .replace(/^.*[/\\]/, '')
    .replace(/\.[^.]+$/, '')
    .trim()
}

/**
 * SKUs candidatos de um nome de arquivo, do mais específico ao menos.
 *
 * `6320-2.jpg` → ["6320-2", "6320"]: tenta o nome cheio primeiro; se não casar,
 * tenta sem o sufixo `-N`/`_N` (segunda foto do mesmo produto). Assim NÃO
 * quebramos um SKU que legitimamente termina em `-dígitos` (ex.: UTL-FER-0007,
 * que casa pelo nome cheio) E ainda permitimos várias fotos por produto num SKU
 * numérico. Um nome que não casa em nenhum candidato vira "sem produto" — nunca
 * um upload no produto errado.
 */
export function skuCandidates(name: string): string[] {
  const base = fileBase(name)
  const stripped = base.replace(/[-_\s]\d+$/, '').trim()
  return base === stripped || stripped === '' ? [base] : [base, stripped]
}

/** SKU "principal" para exibição quando o arquivo não casa (o mais curto). */
export function skuFromFilename(name: string): string {
  const c = skuCandidates(name)
  return c[c.length - 1]
}

export async function resolveBySku(skus: string[]): Promise<SkuMatch[]> {
  if (skus.length === 0) return []
  if (!isBulkImagesEnabled) return mockResolve(skus)
  const q = encodeURIComponent(skus.join(','))
  const res = await adminGet<{ data: SkuMatch[] }>(
    CATALOG_URL,
    `/api/v1/admin/products/by-sku?skus=${q}`
  )
  return res.data ?? []
}

interface UploadResult {
  status: number
  errorMessage: string
}

function doUpload(
  productId: string,
  file: File,
  token: string | null,
  onProgress: (pct: number) => void,
  signal?: AbortSignal
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${CATALOG_URL}/api/v1/admin/products/by-id/${productId}/images/upload`)
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    xhr.timeout = 60_000
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => resolve({ status: xhr.status, errorMessage: parseError(xhr.responseText) })
    xhr.onerror = () => reject(new Error('falha de conexão'))
    xhr.ontimeout = () => reject(new Error('tempo esgotado'))
    xhr.onabort = () => reject(new DOMException('cancelado', 'AbortError'))
    if (signal) {
      if (signal.aborted) {
        xhr.abort()
        return
      }
      signal.addEventListener('abort', () => xhr.abort(), { once: true })
    }
    const fd = new FormData()
    fd.append('files', file, file.name) // o backend aceita o campo "files"
    xhr.send(fd)
  })
}

export async function uploadProductImage(
  productId: string,
  file: File,
  onProgress: (pct: number) => void,
  signal?: AbortSignal
): Promise<void> {
  if (!isBulkImagesEnabled) {
    onProgress(100)
    return
  }
  const token = useAuthStore.getState().user?.token ?? null
  let res = await doUpload(productId, file, token, onProgress, signal)

  // 401: o access token expirou no meio do lote — renova e tenta UMA vez.
  if (res.status === 401) {
    const novo = await refreshAccessToken()
    onProgress(0)
    res = await doUpload(productId, file, novo, onProgress, signal)
  }

  if (res.status < 200 || res.status >= 300) {
    throw new Error(res.errorMessage || `HTTP ${res.status}`)
  }
  onProgress(100)
}

function parseError(body: string): string {
  try {
    const j = JSON.parse(body)
    return typeof j?.error === 'string' ? j.error : ''
  } catch {
    return ''
  }
}

// Mock (modo demo): casa qualquer SKU numérico como se existisse, para a tela
// ser navegável sem backend.
function mockResolve(skus: string[]): SkuMatch[] {
  return skus
    .filter((s) => /^[0-9]{2,}$/.test(s))
    .map((s, i) => ({ sku: s, id: `mock-${s}`, name: `Produto ${s}`, hasImage: i % 4 === 0 }))
}
