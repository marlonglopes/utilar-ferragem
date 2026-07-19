import { MOCK_PRODUCTS } from '@/lib/mockProducts'
import type {
  AdminProductDetail,
  AdminProductRow,
  ImageUploadResponse,
  ProductImageRecord,
  ProductInput,
  ProductStatus,
} from '@/lib/adminProductTypes'

/**
 * Catálogo de demonstração da gestão de produtos.
 *
 * Existe para a tela ser operável sem catalog-service — o mesmo critério de
 * mock do resto do app (`VITE_CATALOG_URL` vazio). O objetivo NÃO é parecer
 * bonito: é reproduzir os casos que a tela precisa tratar e que um catálogo
 * saudável esconderia.
 *
 * Casos plantados de propósito:
 * - produto **em rascunho** (o estado em que a importação de planilha deixa
 *   tudo) — é o que dá o que fazer ao botão de publicar em lote;
 * - produto **sem custo** cadastrado → margem desconhecida, não zero;
 * - produto **com preço abaixo do custo** → o alerta que a tela existe para dar;
 * - produto com **imagem externa** (sem `variants`), que é o legado Wikimedia e
 *   não pode quebrar a galeria;
 * - produto **sem imagem nenhuma**.
 *
 * ⚠️ Este módulo é o ÚNICO lugar do frontend com custo escrito em código, e é
 * dado inventado. Nenhum custo real da Utilar entra aqui.
 */

/** Deriva um custo plausível do preço, sem depender de aleatoriedade. */
function derivedCost(price: number, marginTarget: number): number {
  return Math.round(price * (1 - marginTarget) * 100) / 100
}

function externalImage(id: string, url: string, sortOrder: number): ProductImageRecord {
  // Sem `variants` — é o formato da foto CC0 do Wikimedia que aponta para fora.
  return { id, url, alt: 'foto de referência (Wikimedia, CC0)', sortOrder }
}

function ownImage(id: string, sortOrder: number, originalBytes: number, bytes: number): ProductImageRecord {
  const base = `/media/produtos/demo/${id}`
  return {
    id,
    url: `${base}-large.jpg`,
    alt: 'foto do produto',
    sortOrder,
    variants: { thumb: `${base}-thumb.jpg`, medium: `${base}-medium.jpg`, large: `${base}-large.jpg` },
    width: 1600,
    height: 1600,
    originalBytes,
    bytes,
    sourceFormat: 'jpeg',
    deduplicated: false,
  }
}

/**
 * Estado do catálogo de demonstração, em MEMÓRIA de módulo.
 *
 * Módulo e não `localStorage`, e isto é deliberado: as linhas carregam `cost`.
 * Persistir o mock deixaria custo em disco na máquina da loja — exatamente o
 * que a política do painel proíbe. Recarregar a página zera a demonstração, e
 * esse é o comportamento correto.
 */
let rows: AdminProductRow[] | null = null
let images: Record<string, ProductImageRecord[]> | null = null

function seed(): AdminProductRow[] {
  return MOCK_PRODUCTS.slice(0, 40).map((p, i) => {
    // Distribuição estável (sem RNG): o teste precisa do mesmo catálogo sempre.
    const semCusto = i % 9 === 4
    const prejuizo = i % 13 === 3
    const status: ProductStatus = i % 7 === 1 ? 'draft' : i % 11 === 5 ? 'archived' : 'published'
    const cost = semCusto
      ? null
      : prejuizo
        ? derivedCost(p.price, -0.08) // custo ACIMA do preço
        : derivedCost(p.price, 0.1 + ((i * 7) % 25) / 100)
    return {
      id: p.id,
      slug: p.slug,
      sku: `UTL-${String(1000 + i)}`,
      name: p.name,
      category: p.category,
      brand: p.brand ?? p.seller,
      price: p.price,
      cost,
      stock: p.stock,
      unitOfMeasure: 'un',
      status,
      imageUrl: null,
      updatedAt: new Date(Date.UTC(2026, 6, 1 + (i % 18), 9, 0, 0)).toISOString(),
    }
  })
}

function seedImages(): Record<string, ProductImageRecord[]> {
  return {
    // Imagem própria, com variantes e antes/depois.
    '1': [
      ownImage('img-1a', 0, 4_821_994, 183_220),
      ownImage('img-1b', 1, 2_004_112, 121_880),
    ],
    // Mistura: própria + externa. É o caso que a galeria precisa distinguir.
    '9': [
      ownImage('img-9a', 0, 3_110_500, 154_002),
      externalImage(
        'img-9b',
        'https://upload.wikimedia.org/wikipedia/commons/thumb/demo/cimento.jpg',
        1,
      ),
    ],
    // Só externa.
    '13': [
      externalImage(
        'img-13a',
        'https://upload.wikimedia.org/wikipedia/commons/thumb/demo/cabo.jpg',
        0,
      ),
    ],
    // '2' fica sem imagem nenhuma, de propósito.
  }
}

function state(): AdminProductRow[] {
  if (!rows) rows = seed()
  return rows
}

function imageState(): Record<string, ProductImageRecord[]> {
  if (!images) images = seedImages()
  return images
}

/** Zera a demonstração — usado pelos testes para não vazar estado entre casos. */
export function resetMockProducts(): void {
  rows = null
  images = null
}

export function mockListProducts(): AdminProductRow[] {
  return state().map((r) => ({
    ...r,
    // A capa da listagem é o `thumb`, como na rota pública (`loadThumbnails`).
    imageUrl: imageState()[r.id]?.[0]?.variants?.thumb ?? imageState()[r.id]?.[0]?.url ?? null,
  }))
}

export function mockGetProduct(id: string): AdminProductDetail {
  const row = state().find((r) => r.id === id)
  if (!row) throw new Error(`produto ${id} não existe na demonstração`)
  const source = MOCK_PRODUCTS.find((p) => p.id === id)
  return {
    ...row,
    description: source?.description ?? '',
    originalPrice: source?.originalPrice ?? null,
    qtyStep: 1,
    barcode: null,
    weightKg: null,
    lengthCm: null,
    widthCm: null,
    heightCm: null,
    supplierId: null,
    supplierSku: null,
    ncm: '8467.21.00',
    cfop: '5102',
    cest: null,
    origem: 0,
    specs: source?.specs ?? {},
    seller: source?.seller,
    sellerId: source?.sellerId,
    images: imageState()[id] ?? [],
  }
}

export function mockPatchProduct(id: string, input: ProductInput): AdminProductDetail {
  const all = state()
  const idx = all.findIndex((r) => r.id === id)
  if (idx < 0) throw new Error(`produto ${id} não existe na demonstração`)
  const next = { ...all[idx] }
  // `undefined` = campo não tocado, exatamente como no PATCH do servidor.
  if (input.name !== undefined) next.name = input.name
  if (input.price !== undefined) next.price = input.price
  if (input.cost !== undefined) next.cost = input.cost
  if (input.stock !== undefined) next.stock = input.stock
  if (input.status !== undefined) next.status = input.status
  if (input.category !== undefined) next.category = input.category
  if (input.brand !== undefined) next.brand = input.brand
  if (input.sku !== undefined) next.sku = input.sku
  if (input.unitOfMeasure !== undefined) next.unitOfMeasure = input.unitOfMeasure
  next.updatedAt = new Date().toISOString()
  all[idx] = next
  return mockGetProduct(id)
}

export function mockCreateProduct(input: ProductInput): AdminProductDetail {
  const id = `demo-${Date.now().toString(36)}`
  const all = state()
  all.unshift({
    id,
    slug: (input.slug ?? input.name ?? 'produto').toLowerCase().replace(/\s+/g, '-'),
    sku: input.sku ?? null,
    name: input.name ?? 'Sem nome',
    category: input.category ?? 'ferramentas',
    brand: input.brand ?? null,
    price: input.price ?? 0,
    cost: input.cost ?? null,
    stock: input.stock ?? 0,
    unitOfMeasure: input.unitOfMeasure ?? 'un',
    // Entra como rascunho SEMPRE, igual à importação: produto novo não vai
    // direto para a vitrine sem alguém publicar de propósito.
    status: 'draft',
    imageUrl: null,
    updatedAt: new Date().toISOString(),
  })
  return mockGetProduct(id)
}

export function mockListImages(productId: string): ProductImageRecord[] {
  return imageState()[productId] ?? []
}

export function mockReorderImages(productId: string, order: string[]): ProductImageRecord[] {
  const current = imageState()[productId] ?? []
  const byId = new Map(current.map((i) => [i.id, i]))
  const next = order
    .map((id) => byId.get(id))
    .filter((i): i is ProductImageRecord => Boolean(i))
    .map((img, idx) => ({ ...img, sortOrder: idx }))
  imageState()[productId] = next
  return next
}

export function mockDeleteImage(productId: string, imageId: string): void {
  const current = imageState()[productId] ?? []
  imageState()[productId] = current
    .filter((i) => i.id !== imageId)
    .map((img, idx) => ({ ...img, sortOrder: idx }))
}

/**
 * Upload de demonstração.
 *
 * Aceita o arquivo e inventa as variantes — o navegador não vai redimensionar
 * nada aqui. A recusa é simulada pela EXTENSÃO/tipo declarado, e é preciso
 * dizer o porquê: o servidor de verdade decide pelos **bytes** e ignora nome e
 * `Content-Type`. Esta aproximação existe só para exercitar o caminho de erro
 * da tela sem backend; ela não é a regra, é uma imitação da regra.
 */
export function mockUploadImages(productId: string, files: File[]): ImageUploadResponse {
  const uploaded: ProductImageRecord[] = []
  const rejected: ImageUploadResponse['rejected'] = []
  const current = imageState()[productId] ?? []
  let order = current.length

  for (const file of files) {
    if (file.size > 12 * 1024 * 1024) {
      rejected.push({ filename: file.name, code: 'file_too_large', reason: 'arquivo acima de 12 MB' })
      continue
    }
    if (!file.type.startsWith('image/')) {
      rejected.push({ filename: file.name, code: 'not_an_image', reason: 'tipo declarado não é imagem' })
      continue
    }
    const id = `img-demo-${order}-${Math.abs(hash(file.name)).toString(36)}`
    const original = file.size || 3_000_000
    uploaded.push(ownImage(id, order, original, Math.max(40_000, Math.round(original * 0.06))))
    order += 1
  }

  imageState()[productId] = [...current, ...uploaded]
  return { uploaded, rejected }
}

function hash(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i += 1) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}
