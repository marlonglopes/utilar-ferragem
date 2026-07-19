import { useAuthStore } from '@/store/authStore'
import { AdminApiError } from '@/lib/adminApi'
import {
  filterProducts,
  paginate,
  sortProducts,
  totalPages,
} from '@/lib/adminProductFormat'
import {
  mockCreateProduct,
  mockDeleteImage,
  mockGetProduct,
  mockListImages,
  mockListProducts,
  mockPatchProduct,
  mockReorderImages,
  mockUploadImages,
} from '@/lib/adminProductsMock'
import type {
  AdminProductDetail,
  AdminProductPage,
  AdminProductQuery,
  AdminProductRow,
  BulkStatusResult,
  ImageRejection,
  ImageUploadResponse,
  ProductImageRecord,
  ProductInput,
  ProductStatus,
} from '@/lib/adminProductTypes'

/**
 * Camada de rede da gestão de produtos (catalog-service).
 *
 * Separada de `lib/api.ts` — aquele arquivo é de outro dono e serve o
 * e-commerce. Aqui vale a política do painel, que é mais restritiva:
 *
 * 1. **`cache: 'no-store'` em tudo.** Estas respostas carregam `cost`. Cache de
 *    disco numa máquina de loja é o caminho mais curto para o custo do
 *    fornecedor vazar para o próximo operador que sentar ali.
 * 2. **Nada é persistido.** Nenhuma resposta daqui vai para `localStorage`,
 *    `sessionStorage`, IndexedDB ou telemetria. O que sobrevive ao reload é só
 *    o filtro (busca, categoria, status, página) na query string — e **custo
 *    nunca entra na URL**, nem como filtro nem como ordenação (`sort=margin`
 *    ordena por margem sem carregar valor nenhum).
 * 3. **O token vem do store a cada chamada**, nunca capturado em closure: o
 *    refresh-on-401 do app pode ter trocado o access token no meio de um upload
 *    longo de imagem.
 * 4. **Multipart sem `Content-Type` manual.** O boundary é gerado pelo
 *    `FormData` e só o navegador sabe qual é; header fixo produz um 400 que
 *    parece problema de arquivo e não é.
 *
 * ⚠️ O guard de rota não é fronteira de segurança. Toda rota abaixo é
 * `RequireAdmin` no catalog-service — é o servidor que recusa `cost` para quem
 * não é admin, não esta camada.
 */

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''

/** Sem catalog-service configurado, a tela roda em demonstração. */
export const isProductAdminEnabled = CATALOG_URL !== ''

const BASE = '/api/v1/admin/products'

function authHeaders(): Record<string, string> {
  const token = useAuthStore.getState().user?.token ?? null
  return token ? { Authorization: `Bearer ${token}` } : {}
}

/**
 * Erro do painel que também carrega `details`.
 *
 * `AdminApiError` guarda mensagem, status e `code`, mas descarta `details` — e
 * `details` é justamente onde vive a informação que esta tela mais precisa: a
 * lista de recusas por arquivo quando o upload devolve 400 porque **nenhuma**
 * imagem passou. Sem ela a tela só poderia dizer "5 imagens falharam", que é
 * exatamente o comportamento inútil que este trabalho existe para eliminar.
 *
 * Subclasse local em vez de mudar `adminApi.ts`: aquele arquivo é consumido por
 * cinco telas de contabilidade e auditoria que não têm nada a ver com upload.
 * `instanceof AdminApiError` continua verdadeiro, então quem trata erro do
 * painel de forma genérica não muda em nada.
 */
export class ProductAdminError extends AdminApiError {
  readonly details?: unknown

  constructor(message: string, status: number, code?: string, details?: unknown) {
    super(message, status, code)
    this.name = 'ProductAdminError'
    this.details = details
  }
}

/**
 * Converte a resposta de erro, **preservando `code` e `details`** do envelope
 * `{error, code, requestId, details}`.
 *
 * Preservar o código é o ponto: quem chama distingue `not_found` de
 * `validation_error` por código, nunca por substring da mensagem — foi assim
 * que o PDV do balcão se enroscou com `insufficient_stock`.
 */
async function failure(res: Response): Promise<never> {
  const body = (await res.json().catch(() => ({}))) as {
    error?: string
    code?: string
    details?: unknown
  }
  if (res.status === 401) {
    throw new ProductAdminError(
      'Sua sessão expirou. Entre de novo para continuar.',
      401,
      'unauthorized',
      body.details,
    )
  }
  if (res.status === 403) {
    throw new ProductAdminError(
      'Sua conta não tem permissão de administrador.',
      403,
      'forbidden',
      body.details,
    )
  }
  if (res.status === 404) {
    throw new ProductAdminError(
      body.error ?? 'Produto não encontrado.',
      404,
      body.code ?? 'not_found',
      body.details,
    )
  }
  if (res.status === 413) {
    throw new ProductAdminError(
      'O servidor recusou o envio por tamanho. O teto do corpo é 64 MB — envie menos imagens por vez.',
      413,
      'too_large',
      body.details,
    )
  }
  throw new ProductAdminError(body.error ?? `HTTP ${res.status}`, res.status, body.code, body.details)
}

async function send<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`${CATALOG_URL}${path}`, {
    cache: 'no-store',
    ...init,
    headers: { ...authHeaders(), ...(init.headers ?? {}) },
  })
  if (!res.ok) await failure(res)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function json(body: unknown): RequestInit {
  return { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

function qs(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

// ---------------------------------------------------------------------------
// Lista
// ---------------------------------------------------------------------------

/**
 * Lista paginada do painel.
 *
 * ⚠️ **`GET /api/v1/admin/products` ainda não existe no catalog-service.** Hoje
 * o servidor só tem `GET /admin/products/by-id/:id` (um produto por vez) e a
 * listagem pública, que — corretamente — **não devolve `cost`** e só mostra
 * publicados. Uma lista de produtos com custo, margem e rascunhos precisa da
 * rota admin. O contrato esperado está em `AdminProductPage`.
 *
 * Enquanto ela não existir, o modo demonstração cobre a tela inteira; contra um
 * backend real a chamada devolve 404 e a tela mostra o estado de erro com o
 * código, em vez de fingir uma lista vazia — "nenhum produto" seria uma
 * mentira perigosa numa tela cujo botão publica em lote.
 *
 * Busca, filtro, ordenação e paginação são enviados ao servidor E reaplicados
 * localmente. Não é redundância inútil: no modo demonstração é a única
 * implementação, e contra o servidor é o que garante que a página exibida
 * corresponde ao filtro atual mesmo se o backend ignorar um parâmetro que ainda
 * não implementou.
 */
export async function fetchAdminProducts(q: AdminProductQuery): Promise<AdminProductPage> {
  const local = (all: AdminProductRow[]): AdminProductPage => {
    const filtered = filterProducts(all, q.q ?? '', q.category ?? '', q.status ?? '')
    const sorted = sortProducts(filtered, q.sort, q.dir)
    return {
      data: paginate(sorted, q.page, q.pageSize),
      meta: {
        page: q.page,
        pageSize: q.pageSize,
        total: sorted.length,
        totalPages: totalPages(sorted.length, q.pageSize),
      },
    }
  }

  if (!isProductAdminEnabled) return local(mockListProducts())

  const res = await send<{ data?: AdminProductRow[]; meta?: AdminProductPage['meta'] }>(
    `${BASE}${qs({
      q: q.q,
      category: q.category,
      status: q.status,
      sort: q.sort,
      dir: q.dir,
      page: q.page,
      pageSize: q.pageSize,
    })}`,
    { method: 'GET' },
  )
  const rows = res.data ?? []
  // O servidor pagina; se ele já devolveu `meta`, respeitamos a contagem dele —
  // reordenar a página recebida é local e barato, refiltrar não pode mudar o
  // total (senão a paginação mente sobre quantas páginas existem).
  if (res.meta) {
    return { data: sortProducts(rows, q.sort, q.dir), meta: res.meta }
  }
  return local(rows)
}

// ---------------------------------------------------------------------------
// Detalhe / escrita
// ---------------------------------------------------------------------------

export async function fetchAdminProduct(id: string): Promise<AdminProductDetail> {
  if (!isProductAdminEnabled) return mockGetProduct(id)
  return send<AdminProductDetail>(`${BASE}/by-id/${encodeURIComponent(id)}`, { method: 'GET' })
}

/**
 * PATCH parcial: **campo ausente = não mexe**.
 *
 * Por isso o formulário só envia o que mudou (`diffInput`), e não o objeto
 * inteiro: mandar tudo transformaria qualquer edição de preço numa reescrita de
 * todos os campos fiscais com o que estava na tela — inclusive o que outra
 * pessoa alterou enquanto o formulário estava aberto.
 */
export async function patchAdminProduct(
  id: string,
  input: ProductInput,
): Promise<AdminProductDetail> {
  if (!isProductAdminEnabled) return mockPatchProduct(id, input)
  return send<AdminProductDetail>(`${BASE}/by-id/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    ...json(input),
  })
}

/**
 * Cria o produto. **Sempre como rascunho**, e o `status` é forçado aqui em vez
 * de ser um campo do formulário: produto novo não tem foto, nem ficha, nem
 * conferência — publicar direto colocaria um item cru na vitrine. É a mesma
 * decisão da importação de planilha, que também entra tudo em rascunho.
 */
export async function createAdminProduct(input: ProductInput): Promise<AdminProductDetail> {
  const body: ProductInput = { ...input, status: 'draft' }
  if (!isProductAdminEnabled) return mockCreateProduct(body)
  return send<AdminProductDetail>(BASE, { method: 'POST', ...json(body) })
}

/**
 * Publica/despublica vários produtos — o passo que falta depois de importar a
 * planilha, já que a importação entra tudo como rascunho de propósito.
 *
 * **Não existe rota de lote no servidor**, então isto é um `PATCH` por produto,
 * em janelas de concorrência limitada. Duas consequências que a tela precisa
 * assumir, e assume:
 *
 * - **não é atômico** — pode publicar 8 de 10. Por isso o retorno é
 *   `{ok, failed}` por item, e não um booleano: dizer "falhou" depois de
 *   publicar 8 faria o dono repetir a ação e não entender o resultado;
 * - **a janela é pequena (6)** para não abrir 200 conexões e derrubar o próprio
 *   catálogo com o botão que deveria organizá-lo.
 *
 * Uma rota `POST /admin/products/bulk-status` no catalog-service tornaria isto
 * atômico e uma chamada só. Enquanto não existe, este caminho funciona.
 */
const BULK_CONCURRENCY = 6

export async function bulkSetStatus(
  items: Array<{ id: string; name: string }>,
  status: ProductStatus,
): Promise<BulkStatusResult> {
  const result: BulkStatusResult = { ok: [], failed: [] }

  for (let i = 0; i < items.length; i += BULK_CONCURRENCY) {
    const window = items.slice(i, i + BULK_CONCURRENCY)
    const settled = await Promise.allSettled(
      window.map((it) => patchAdminProduct(it.id, { status })),
    )
    settled.forEach((outcome, idx) => {
      const item = window[idx]
      if (outcome.status === 'fulfilled') {
        result.ok.push(item.id)
        return
      }
      const err = outcome.reason
      result.failed.push({
        id: item.id,
        name: item.name,
        message: err instanceof Error ? err.message : 'Falha desconhecida',
        code: err instanceof AdminApiError ? err.code : undefined,
      })
    })
  }

  return result
}

// ---------------------------------------------------------------------------
// Imagens
// ---------------------------------------------------------------------------

export async function fetchProductImages(productId: string): Promise<ProductImageRecord[]> {
  if (!isProductAdminEnabled) return mockListImages(productId)
  const res = await send<{ data?: ProductImageRecord[] }>(
    `${BASE}/by-id/${encodeURIComponent(productId)}/images`,
    { method: 'GET' },
  )
  return res.data ?? []
}

/**
 * Extrai as recusas de um 400 "todos os arquivos foram recusados".
 *
 * O upload devolve **201 se ao menos um entrou** (com `rejected[]` no corpo de
 * sucesso) e **400 se todos foram recusados** (com as recusas em `details`).
 * São dois formatos para o mesmo fato, e a tela precisa mostrar o mesmo motivo
 * por arquivo nos dois casos — daí a normalização aqui, e não no componente.
 */
function parseUploadFailure(err: unknown): ImageRejection[] | null {
  if (!(err instanceof ProductAdminError)) return null
  const { details } = err
  if (Array.isArray(details)) {
    return details.filter(
      (d): d is ImageRejection =>
        typeof d === 'object' && d !== null && 'filename' in (d as object),
    )
  }
  return null
}

/**
 * Envia várias imagens de uma vez (campo `files`, repetível).
 *
 * O `AbortSignal` é obrigatório na prática: imagem grande demora, o lote tem
 * 90 s de teto no servidor, e sem cancelamento sair da tela deixaria o upload
 * rodando para entregar a resposta a um componente desmontado.
 *
 * **Nunca lança quando o servidor recusou arquivos.** Recusa não é falha de
 * rede: um 400 com todas as imagens recusadas volta como
 * `{uploaded: [], rejected: [...]}`, para o chamador ter um só caminho de
 * renderização — a lista de motivos por arquivo. Erro de verdade (403, 500,
 * rede caída) continua sendo lançado.
 */
export async function uploadProductImages(
  productId: string,
  files: File[],
  signal?: AbortSignal,
  alt?: string,
): Promise<ImageUploadResponse> {
  if (!isProductAdminEnabled) return mockUploadImages(productId, files)

  const fd = new FormData()
  for (const file of files) fd.append('files', file)
  if (alt) fd.append('alt', alt)

  try {
    return await send<ImageUploadResponse>(
      `${BASE}/by-id/${encodeURIComponent(productId)}/images/upload`,
      { method: 'POST', body: fd, signal },
    )
  } catch (err) {
    const rejected = parseUploadFailure(err)
    if (rejected && rejected.length > 0) return { uploaded: [], rejected }
    throw err
  }
}

/**
 * Reordena. O corpo é a lista COMPLETA na ordem desejada e o primeiro vira a
 * capa — é idempotente por contrato, então repetir a chamada depois de um erro
 * de rede é seguro.
 */
export async function reorderProductImages(
  productId: string,
  order: string[],
): Promise<ProductImageRecord[]> {
  if (!isProductAdminEnabled) return mockReorderImages(productId, order)
  await send<unknown>(`${BASE}/by-id/${encodeURIComponent(productId)}/images/order`, {
    method: 'PUT',
    ...json({ order }),
  })
  return fetchProductImages(productId)
}

/** Promove a capa sem mandar a lista inteira. */
export async function setProductImageCover(
  productId: string,
  imageId: string,
): Promise<ProductImageRecord[]> {
  if (!isProductAdminEnabled) {
    const current = mockListImages(productId)
    const order = [imageId, ...current.filter((i) => i.id !== imageId).map((i) => i.id)]
    return mockReorderImages(productId, order)
  }
  await send<unknown>(
    `${BASE}/by-id/${encodeURIComponent(productId)}/images/${encodeURIComponent(imageId)}/cover`,
    { method: 'PUT' },
  )
  return fetchProductImages(productId)
}

export async function deleteProductImage(
  productId: string,
  imageId: string,
): Promise<ProductImageRecord[]> {
  if (!isProductAdminEnabled) {
    mockDeleteImage(productId, imageId)
    return mockListImages(productId)
  }
  await send<unknown>(
    `${BASE}/by-id/${encodeURIComponent(productId)}/images/${encodeURIComponent(imageId)}`,
    { method: 'DELETE' },
  )
  return fetchProductImages(productId)
}
