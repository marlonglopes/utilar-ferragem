/**
 * Contratos da gestão de produtos no painel — espelho fiel do catalog-service.
 *
 * Fonte da verdade:
 * - `services/catalog-service/internal/model/product.go` (`AdminProduct`)
 * - `services/catalog-service/internal/handler/admin_product.go` (`productInput`)
 * - `services/catalog-service/internal/handler/product_image_upload.go`
 * - `docs/imagens-produto.md` (contrato de imagem, completo)
 *
 * Nada aqui é inventado: cada campo existe no JSON que o backend devolve ou
 * aceita. A única exceção é `AdminProductPage`, a rota de LISTA — que ainda não
 * existe no servidor. Está marcada como tal, com o formato exato esperado.
 */

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

/** `validStatus()` no backend: draft|published|archived. Nada além disso. */
export const PRODUCT_STATUSES = ['draft', 'published', 'archived'] as const
export type ProductStatus = (typeof PRODUCT_STATUSES)[number]

export function isProductStatus(v: string): v is ProductStatus {
  return (PRODUCT_STATUSES as readonly string[]).includes(v)
}

// ---------------------------------------------------------------------------
// Imagem
// ---------------------------------------------------------------------------

/**
 * As três variantes normalizadas (300 / 800 / 1600 px, 1:1, letterbox branco).
 *
 * **Ausente em imagem externa.** O catálogo tem fotos CC0 do Wikimedia
 * apontando para URL de terceiro: elas não passaram pelo pipeline, não têm
 * variante, e não podem quebrar a tela. Ver `docs/imagens-produto.md`
 * § "Convivência com imagem externa".
 */
export interface ImageVariants {
  thumb: string
  medium: string
  large: string
}

export interface ProductImageRecord {
  id: string
  url: string
  alt: string
  sortOrder: number
  /** AUSENTE (undefined/null) em imagem externa por URL. */
  variants?: ImageVariants | null
  width?: number
  height?: number
  /** Bytes do arquivo ORIGINAL enviado — só existe em upload. */
  originalBytes?: number
  /** Bytes depois da normalização. */
  bytes?: number
  sourceFormat?: string
  /** `true` = a mesma foto (mesmo hash) já estava no produto; nada novo entrou. */
  deduplicated?: boolean
}

/**
 * Códigos estáveis de recusa por arquivo. **Case pelo código, nunca pela
 * mensagem** — a mensagem do servidor é livre e muda sem aviso.
 */
export const IMAGE_REJECT_CODES = [
  'not_an_image',
  'file_too_large',
  'image_too_large',
  'image_too_small',
  'corrupt_image',
  'processing_timeout',
  'storage_error',
] as const

export type ImageRejectCode = (typeof IMAGE_REJECT_CODES)[number]

export interface ImageRejection {
  filename: string
  /** Mensagem do servidor. Exibida como detalhe, nunca usada para decidir. */
  reason?: string
  /** Pode vir vazio/desconhecido: a tela cai num rótulo genérico, não quebra. */
  code?: ImageRejectCode | string
}

/**
 * Resposta do upload multipart.
 *
 * `201` se **ao menos um** arquivo entrou — `rejected[]` traz o resto. Se
 * **todos** foram recusados é `400`, e as recusas vêm em `details` do envelope
 * de erro (ver `parseUploadFailure` em `adminProductsApi.ts`).
 */
export interface ImageUploadResponse {
  uploaded: ProductImageRecord[]
  rejected?: ImageRejection[] | null
}

// ---------------------------------------------------------------------------
// Produto
// ---------------------------------------------------------------------------

/**
 * Linha da lista do painel.
 *
 * ⚠️ Carrega `cost` e `marginPct`. Isto **só** trafega em rota sob
 * `RequireAdmin` e **nunca** pode ser persistido no cliente — ver a política em
 * `adminProductsApi.ts`.
 */
export interface AdminProductRow {
  id: string
  slug: string
  sku?: string | null
  name: string
  category: string
  brand?: string | null
  price: number
  /** Custo em reais. `null` = não cadastrado (não é o mesmo que zero). */
  cost?: number | null
  /** Margem em PERCENTUAL (0..100), como o servidor calcula. Pode faltar. */
  marginPct?: number | null
  stock: number
  unitOfMeasure: string
  status: ProductStatus
  /** Capa já resolvida para `thumb` pelo servidor, quando houver. */
  imageUrl?: string | null
  updatedAt?: string
}

/** Detalhe completo — o payload de `GET /api/v1/admin/products/by-id/:id`. */
export interface AdminProductDetail extends AdminProductRow {
  description?: string | null
  originalPrice?: number | null
  qtyStep?: number
  barcode?: string | null
  weightKg?: number | null
  lengthCm?: number | null
  widthCm?: number | null
  heightCm?: number | null
  supplierId?: string | null
  supplierSku?: string | null
  ncm?: string | null
  cfop?: string | null
  cest?: string | null
  /** Origem fiscal 0..8 da tabela do CST. */
  origem?: number | null
  specs?: Record<string, string> | null
  sellerId?: string
  seller?: string
  images?: ProductImageRecord[]
}

/**
 * Corpo de `POST /admin/products` e `PATCH /admin/products/by-id/:id`.
 *
 * Todo campo é opcional porque o PATCH é parcial: **ausente = não mexe**. Por
 * isso `null` e `undefined` NÃO são intercambiáveis aqui — mandar `undefined`
 * preserva o valor no banco, e é o que a tela faz para campo não tocado.
 */
export interface ProductInput {
  sku?: string
  slug?: string
  name?: string
  category?: string
  sellerId?: string
  price?: number
  originalPrice?: number | null
  brand?: string
  stock?: number
  description?: string
  status?: ProductStatus
  cost?: number | null
  unitOfMeasure?: string
  qtyStep?: number
  barcode?: string
  weightKg?: number | null
  lengthCm?: number | null
  widthCm?: number | null
  heightCm?: number | null
  supplierId?: string
  supplierSku?: string
  ncm?: string
  cfop?: string
  cest?: string
  origem?: number | null
  specs?: Record<string, string>
}

// ---------------------------------------------------------------------------
// Consulta da lista
// ---------------------------------------------------------------------------

export const PRODUCT_SORTS = [
  'name',
  'price',
  'cost',
  'margin',
  'stock',
  'updated',
] as const

export type ProductSortKey = (typeof PRODUCT_SORTS)[number]

export interface AdminProductQuery {
  q?: string
  category?: string
  status?: ProductStatus | ''
  sort: ProductSortKey
  dir: 'asc' | 'desc'
  page: number
  pageSize: number
}

/**
 * ⚠️ **Esta rota ainda não existe no catalog-service.** Hoje só há
 * `GET /admin/products/by-id/:id` (um produto). A lista com `cost` precisa de
 * `GET /api/v1/admin/products` devolvendo exatamente este envelope — o mesmo
 * formato de `meta` que a listagem pública já usa, para não inventar um
 * terceiro dialeto de paginação no mesmo serviço.
 */
export interface AdminProductPage {
  data: AdminProductRow[]
  meta: {
    page: number
    pageSize: number
    total: number
    totalPages: number
  }
}

/** Resultado da ação em lote — por item, porque falha parcial é o caso comum. */
export interface BulkStatusResult {
  ok: string[]
  failed: Array<{ id: string; name: string; message: string; code?: string }>
}
