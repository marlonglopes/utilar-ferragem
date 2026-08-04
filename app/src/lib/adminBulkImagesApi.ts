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

/**
 * Candidatos de SKU a partir do CAMINHO RELATIVO do arquivo (com pastas).
 *
 * O plano da loja é "1 pasta por SKU" (fotografa tudo, cada pasta é um código).
 * Então a PASTA imediata é o candidato primário; o nome do arquivo entra como
 * fallback (mantém o modelo antigo "6320.jpg" solto funcionando). Ex.:
 *   "6320/IMG_0001.jpg"            → ["6320", "IMG_0001", "IMG"]  (casa por pasta)
 *   "lote/6320/foto.jpg"           → ["6320", "foto"]            (pasta imediata)
 *   "6320.jpg"                     → ["6320"]                    (solto, como antes)
 */
export function skuCandidatesForFile(path: string): string[] {
  const parts = path.split('/').filter(Boolean)
  const fileName = parts[parts.length - 1] ?? path
  const folder = parts.length >= 2 ? parts[parts.length - 2].trim() : ''
  const fromName = skuCandidates(fileName)
  const out = folder ? [folder, ...fromName] : fromName
  return Array.from(new Set(out.filter(Boolean)))
}

/**
 * Ordena as fotos para upload de forma que a CAPA seja determinística.
 *
 * O backend dá `sort_order 0` (a capa) para a PRIMEIRA foto que chega de cada
 * produto. Se as fotos do mesmo produto subissem em paralelo, a capa viraria
 * corrida (qual chega primeiro). Então agrupamos por produto e, dentro do grupo,
 * ordenamos por nome de arquivo em ordem NATURAL (1, 2, 10 — não 1, 10, 2). O
 * chamador sobe cada grupo EM SÉRIE (a 1ª foto vira capa) e grupos diferentes em
 * paralelo. Assim "1.jpg" é sempre a capa, previsível para o lojista.
 */
export function planUploadOrder<T extends { productId: string; fileName: string }>(
  items: T[]
): T[][] {
  const byProduct = new Map<string, T[]>()
  for (const it of items) {
    const arr = byProduct.get(it.productId) ?? []
    arr.push(it)
    byProduct.set(it.productId, arr)
  }
  return Array.from(byProduct.values()).map((g) =>
    [...g].sort((a, b) => a.fileName.localeCompare(b.fileName, undefined, { numeric: true }))
  )
}

/** Arquivo coletado com seu caminho relativo (preserva a pasta = SKU). */
export interface CollectedFile {
  file: File
  /** Caminho relativo, ex.: "6320/IMG_0001.jpg". Sem pasta = só o nome. */
  path: string
}

// Tipos mínimos da File System API de drag-and-drop (não vêm no lib.dom padrão).
interface FSEntry {
  isFile: boolean
  isDirectory: boolean
  name: string
  file?: (onOk: (f: File) => void, onErr?: (e: unknown) => void) => void
  createReader?: () => FSDirectoryReader
}
interface FSDirectoryReader {
  readEntries: (onOk: (entries: FSEntry[]) => void, onErr?: (e: unknown) => void) => void
}

/**
 * Extrai TODOS os arquivos de um drop, DESCENDO em pastas (recursivo).
 *
 * `dataTransfer.files` NÃO expande diretórios — por isso arrastar pastas não
 * trazia nada. Aqui usamos `webkitGetAsEntry()` + travessia recursiva, que é o
 * único jeito de ler o conteúdo de pastas soltas na área. Sem suporte a entries
 * (navegador antigo), cai no comportamento antigo (`files`, sem pastas).
 */
export async function collectFilesFromDataTransfer(dt: DataTransfer): Promise<CollectedFile[]> {
  const items = dt.items
  const first = items && items.length > 0 ? items[0] : null
  const canTraverse =
    !!first && typeof (first as { webkitGetAsEntry?: unknown }).webkitGetAsEntry === 'function'

  if (!canTraverse) {
    return Array.from(dt.files).map((file) => ({ file, path: file.name }))
  }

  const roots: FSEntry[] = []
  for (let i = 0; i < items.length; i++) {
    const getEntry = (items[i] as { webkitGetAsEntry?: () => FSEntry | null }).webkitGetAsEntry
    const entry = getEntry ? getEntry.call(items[i]) : null
    if (entry) roots.push(entry)
  }
  const out: CollectedFile[] = []
  await Promise.all(roots.map((e) => walkEntry(e, '', out)))
  return out
}

function walkEntry(entry: FSEntry, prefix: string, out: CollectedFile[]): Promise<void> {
  if (entry.isFile && entry.file) {
    return new Promise((resolve) => {
      entry.file!(
        (f) => {
          out.push({ file: f, path: prefix + entry.name })
          resolve()
        },
        () => resolve()
      )
    })
  }
  if (entry.isDirectory && entry.createReader) {
    const reader = entry.createReader()
    return readAllEntries(reader).then((children) =>
      Promise.all(children.map((c) => walkEntry(c, prefix + entry.name + '/', out))).then(
        () => undefined
      )
    )
  }
  return Promise.resolve()
}

// readEntries devolve no MÁXIMO ~100 por chamada; é preciso chamar em loop até
// vir um lote vazio. Esquecer disso perde arquivos em pastas grandes.
function readAllEntries(reader: FSDirectoryReader): Promise<FSEntry[]> {
  return new Promise((resolve) => {
    const all: FSEntry[] = []
    const readBatch = () => {
      reader.readEntries(
        (batch) => {
          if (!batch.length) {
            resolve(all)
            return
          }
          all.push(...batch)
          readBatch()
        },
        () => resolve(all)
      )
    }
    readBatch()
  })
}

// Tamanho do lote da consulta by-sku. 200 SKUs EAN-13 ≈ 2,8 KB de query —
// folgadamente abaixo do teto de ~4-8 KB dos proxies. Mandar tudo numa query só
// estourava 414 (URI Too Long) já em ~250-520 SKUs — ver o teste de carga.
const BY_SKU_BATCH = 200

export async function resolveBySku(skus: string[]): Promise<SkuMatch[]> {
  if (skus.length === 0) return []
  if (!isBulkImagesEnabled) return mockResolve(skus)
  const batches: string[][] = []
  for (let i = 0; i < skus.length; i += BY_SKU_BATCH) {
    batches.push(skus.slice(i, i + BY_SKU_BATCH))
  }
  const results = await Promise.all(batches.map(fetchBySkuBatch))
  return results.flat()
}

async function fetchBySkuBatch(skus: string[]): Promise<SkuMatch[]> {
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
