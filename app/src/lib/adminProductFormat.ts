import type { Severity } from '@/lib/adminTypes'
import type {
  AdminProductRow,
  ImageRejectCode,
  ProductImageRecord,
  ProductStatus,
} from '@/lib/adminProductTypes'

/**
 * Regras e formatação da gestão de produtos. Tudo aqui é função pura — é o que
 * torna a parte que importa (margem, preço abaixo do custo) testável sem
 * montar tela.
 *
 * ⚠️ Nenhuma função deste arquivo escreve em `localStorage`, telemetria ou URL.
 * Custo e margem passam por aqui; eles só podem existir em memória e em pixel.
 */

// ---------------------------------------------------------------------------
// Margem
// ---------------------------------------------------------------------------

/**
 * Tolerância de meio centavo nas comparações de dinheiro.
 *
 * Espelha `holdIfPriceBelowCost` no `ingest/pipeline.go`, que usa
 * `cost - price <= 0.005` para NÃO reter. Preço e custo chegam como float de
 * JSON: sem tolerância, `10.00` contra `10.000000000000002` acusaria prejuízo
 * num produto vendido exatamente no custo.
 */
export const MONEY_EPSILON = 0.005

/**
 * Margem sobre o PREÇO (`(preço − custo) / preço`), em fração.
 *
 * É a mesma conta do `marginPct()` do backend (`handler/store_cost.go`), só que
 * em fração em vez de percentual, para casar com o resto do painel — todo
 * `formatPercent` da casa recebe 0..1.
 *
 * Devolve `null` quando não há resposta honesta:
 * - **sem custo cadastrado** → margem desconhecida, e `null` não é zero. Zero
 *   apareceria como "0,0%" e o dono leria "esse produto não dá lucro", que é
 *   uma afirmação que ninguém fez.
 * - **preço ≤ 0** → divisão sem significado.
 *
 * Margem negativa é resultado VÁLIDO e é devolvida como tal: é exatamente o
 * caso que a tela precisa gritar.
 */
export function marginFraction(price: number, cost: number | null | undefined): number | null {
  if (cost === null || cost === undefined) return null
  if (!Number.isFinite(price) || !Number.isFinite(cost)) return null
  if (price <= 0) return null
  return (price - cost) / price
}

/** O mesmo em percentual (0..100), que é a unidade que o servidor devolve. */
export function marginPercent(price: number, cost: number | null | undefined): number | null {
  const f = marginFraction(price, cost)
  return f === null ? null : f * 100
}

/** Lucro por unidade, em reais. `null` sem custo — mesma razão da margem. */
export function unitProfit(price: number, cost: number | null | undefined): number | null {
  if (cost === null || cost === undefined) return null
  if (!Number.isFinite(price) || !Number.isFinite(cost)) return null
  return price - cost
}

/**
 * Preço de venda abaixo do custo.
 *
 * **Espelha a importação de propósito.** `holdIfPriceBelowCost` retém a linha
 * da planilha nesse caso; se a edição manual fosse mais permissiva, o caminho
 * para cadastrar produto no prejuízo seria simplesmente "não use a planilha" —
 * e a defesa inteira viraria decoração.
 *
 * Vender exatamente no custo (margem zero) **não** é abaixo do custo: é uma
 * decisão comercial legítima (item de isca, liquidação) e alertar ali gastaria
 * o alarme à toa.
 */
export function isPriceBelowCost(price: number, cost: number | null | undefined): boolean {
  if (cost === null || cost === undefined) return false
  if (!Number.isFinite(price) || !Number.isFinite(cost)) return false
  return cost - price > MONEY_EPSILON
}

/**
 * Severidade da margem para o medidor ao vivo.
 *
 * Os cortes são os de ferragem já usados na tela de vendedores
 * (`marginSeverity` em `adminFormat.ts`): abaixo de 12% a venda não paga a
 * operação, acima de 22% está saudável. Repetidos aqui — e não importados —
 * porque este caso tem um degrau a mais: margem NEGATIVA, que na tela de
 * vendedores não existe (ninguém agrega prejuízo) e aqui é o alvo principal.
 */
export function productMarginSeverity(fraction: number | null): Severity | null {
  if (fraction === null) return null
  if (fraction < 0) return 'critical'
  if (fraction < 0.12) return 'critical'
  if (fraction < 0.22) return 'warn'
  return 'ok'
}

/** Tudo que a tela de edição precisa saber sobre preço × custo, de uma vez. */
export interface MarginReading {
  /** Fração 0..1, ou `null` sem custo cadastrado. Pode ser negativa. */
  fraction: number | null
  /** Lucro unitário em reais, ou `null`. */
  profit: number | null
  severity: Severity | null
  belowCost: boolean
  /** `true` quando não há custo — a tela mostra "—" e um convite a preencher. */
  unknown: boolean
}

export function readMargin(price: number, cost: number | null | undefined): MarginReading {
  const fraction = marginFraction(price, cost)
  return {
    fraction,
    profit: unitProfit(price, cost),
    severity: productMarginSeverity(fraction),
    belowCost: isPriceBelowCost(price, cost),
    unknown: cost === null || cost === undefined,
  }
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

export const STATUS_LABEL: Record<ProductStatus, string> = {
  draft: 'Rascunho',
  published: 'Publicado',
  archived: 'Arquivado',
}

export const STATUS_HINT: Record<ProductStatus, string> = {
  draft: 'Não aparece na loja. É assim que a importação de planilha deixa tudo.',
  published: 'Visível na vitrine e vendável.',
  archived: 'Fora da loja e fora da lista padrão, sem perder histórico.',
}

/** Classe do chip por status. Rascunho é âmbar: é um estado pendente, não neutro. */
export const STATUS_PILL: Record<ProductStatus, string> = {
  draft: 'bg-amber-50 text-amber-900 ring-amber-600/25',
  published: 'bg-green-50 text-green-900 ring-green-600/25',
  archived: 'bg-gray-100 text-gray-600 ring-gray-500/25',
}

// ---------------------------------------------------------------------------
// Recusa de imagem
// ---------------------------------------------------------------------------

/**
 * Rótulo por código de recusa.
 *
 * Cada texto diz **o que fazer**, não só o que houve: "3 de 5 imagens falharam"
 * sem dizer quais nem por quê é exatamente o que esta tabela existe para
 * impedir. Os limites citados são os de `docs/imagens-produto.md` § Limites.
 */
const REJECT_LABEL: Record<ImageRejectCode, string> = {
  not_an_image:
    'O arquivo não é uma imagem. O tipo é conferido pelo conteúdo — renomear a extensão não passa.',
  file_too_large: 'Arquivo acima de 12 MB. Reduza a resolução ou salve em JPEG.',
  image_too_large:
    'Imagem grande demais em pixels (teto de 50 MP e 12.000 px por lado). Redimensione antes de enviar.',
  image_too_small: 'Imagem pequena demais — o maior lado precisa ter ao menos 200 px.',
  corrupt_image: 'O arquivo está corrompido e não pôde ser aberto. Exporte a foto de novo.',
  processing_timeout:
    'O processamento estourou o tempo limite. Tente enviar esta imagem sozinha, ou reduza a resolução.',
  storage_error:
    'Falha ao gravar no armazenamento. Não é problema do seu arquivo — tente de novo em instantes.',
}

/**
 * Traduz o código de recusa. Código desconhecido (backend novo, cliente velho)
 * cai num texto genérico em vez de sumir: um arquivo recusado em silêncio é
 * pior que um recusado com motivo vago.
 */
export function rejectLabel(code: string | undefined, reason?: string): string {
  if (code && code in REJECT_LABEL) return REJECT_LABEL[code as ImageRejectCode]
  if (reason && reason.trim() !== '') return reason
  return 'Recusada pelo servidor por um motivo não identificado.'
}

/** Recusa por limite/config do servidor, e não por defeito do arquivo. */
export function isServerSideRejection(code: string | undefined): boolean {
  return code === 'storage_error' || code === 'processing_timeout'
}

// ---------------------------------------------------------------------------
// Bytes
// ---------------------------------------------------------------------------

/** 4821994 → "4,6 MB". Base 1024, que é como o sistema operacional mostra. */
export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toLocaleString('pt-BR', { maximumFractionDigits: 0 })} KB`
  const mb = kb / 1024
  return `${mb.toLocaleString('pt-BR', { maximumFractionDigits: 1 })} MB`
}

/**
 * Quanto a normalização economizou, em fração (0,96 = 96% menor).
 *
 * `null` quando não dá para afirmar nada: sem o tamanho original, ou quando o
 * arquivo cresceu (acontece com PNG minúsculo virando JPEG 1:1) — nesse caso a
 * tela mostra os dois números e cala sobre "economia".
 */
export function byteSavings(
  originalBytes: number | null | undefined,
  bytes: number | null | undefined,
): number | null {
  if (!originalBytes || !bytes) return null
  if (!Number.isFinite(originalBytes) || !Number.isFinite(bytes)) return null
  if (originalBytes <= 0 || bytes >= originalBytes) return null
  return (originalBytes - bytes) / originalBytes
}

// ---------------------------------------------------------------------------
// Imagens: capa, ordem, variantes
// ---------------------------------------------------------------------------

/**
 * Imagem própria (passou pelo pipeline) × imagem externa por URL.
 *
 * A distinção é `variants`, exatamente como manda `docs/imagens-produto.md`.
 * A tela precisa dela para não prometer prévia de tamanho nem "antes/depois" de
 * uma foto que mora no servidor de outra pessoa.
 */
export function hasVariants(img: ProductImageRecord): boolean {
  return Boolean(img.variants && img.variants.thumb)
}

/**
 * Melhor URL para um contexto, com queda segura.
 *
 * Imagem externa não tem variante: devolve `url` como está. É o caminho das
 * fotos CC0 do Wikimedia, e ele **não pode quebrar** — por isso não há acesso
 * direto a `img.variants.thumb` em nenhum componente.
 */
export function imageSrc(img: ProductImageRecord, size: 'thumb' | 'medium' | 'large'): string {
  if (img.variants && img.variants[size]) return img.variants[size]
  return img.url
}

/** Ordena pelo `sortOrder` do servidor; a primeira é a capa, por definição. */
export function sortImages(images: ProductImageRecord[]): ProductImageRecord[] {
  return [...images].sort((a, b) => a.sortOrder - b.sortOrder)
}

/**
 * Move uma imagem de posição, devolvendo a lista nova com `sortOrder`
 * renumerado de 0 em diante.
 *
 * Renumerar aqui (e não deixar buracos) é o que faz `PUT .../images/order` ser
 * idempotente: o corpo é sempre a lista completa na ordem desejada, e o
 * primeiro elemento é sempre a capa. Índice fora da faixa devolve a lista
 * intacta em vez de embaralhar — um arrastar cancelado não pode reordenar nada.
 */
export function moveImage(
  images: ProductImageRecord[],
  from: number,
  to: number,
): ProductImageRecord[] {
  if (from === to) return images
  if (from < 0 || from >= images.length) return images
  if (to < 0 || to >= images.length) return images
  const next = [...images]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next.map((img, i) => ({ ...img, sortOrder: i }))
}

/**
 * Promove uma imagem a capa: ela vai para a posição 0 e o resto desce.
 *
 * Preserva a ordem relativa das demais — promover a 4ª foto não pode
 * reembaralhar as outras três, que o dono já ordenou.
 */
export function promoteCover(
  images: ProductImageRecord[],
  imageId: string,
): ProductImageRecord[] {
  const idx = images.findIndex((i) => i.id === imageId)
  if (idx <= 0) return images
  return moveImage(images, idx, 0)
}

/** Só os ids, na ordem — é o corpo de `PUT .../images/order`. */
export function imageOrderPayload(images: ProductImageRecord[]): string[] {
  return images.map((i) => i.id)
}

// ---------------------------------------------------------------------------
// Lista: busca, filtro, ordenação
// ---------------------------------------------------------------------------

/** Normaliza para busca: minúsculo e sem acento, para "furadeira" achar "Furadeira". */
export function normalizeSearch(s: string): string {
  return s
    .toLocaleLowerCase('pt-BR')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .trim()
}

/**
 * Filtro local da lista — usado no modo demonstração e como rede de segurança
 * quando o servidor devolve mais do que o filtro pedia.
 *
 * Busca casa em nome, SKU e marca: são os três campos por onde o dono procura
 * um produto. Custo NÃO é campo de busca, de propósito — digitar um custo na
 * caixa de busca colocaria o número no histórico do formulário.
 */
export function filterProducts(
  rows: AdminProductRow[],
  q: string,
  category: string,
  status: string,
): AdminProductRow[] {
  const needle = normalizeSearch(q)
  return rows.filter((r) => {
    if (category && r.category !== category) return false
    if (status && r.status !== status) return false
    if (!needle) return true
    const haystack = normalizeSearch(`${r.name} ${r.sku ?? ''} ${r.brand ?? ''}`)
    return haystack.includes(needle)
  })
}

/**
 * Ordena a lista. Linhas SEM custo vão sempre para o fim na ordenação por
 * custo/margem, independente da direção: "desconhecido" não é nem o maior nem o
 * menor, e deixá-lo competir faria produto sem custo aparecer como o de pior
 * margem da loja.
 */
export function sortProducts(
  rows: AdminProductRow[],
  key: string,
  dir: 'asc' | 'desc',
): AdminProductRow[] {
  const mult = dir === 'desc' ? -1 : 1
  const value = (r: AdminProductRow): number | null => {
    switch (key) {
      case 'price':
        return r.price
      case 'cost':
        return r.cost ?? null
      case 'margin':
        return marginFraction(r.price, r.cost)
      case 'stock':
        return r.stock
      default:
        return null
    }
  }

  return [...rows].sort((a, b) => {
    if (key === 'name') return a.name.localeCompare(b.name, 'pt-BR') * mult
    if (key === 'updated') return (a.updatedAt ?? '').localeCompare(b.updatedAt ?? '') * mult
    const va = value(a)
    const vb = value(b)
    if (va === null && vb === null) return a.name.localeCompare(b.name, 'pt-BR')
    if (va === null) return 1
    if (vb === null) return -1
    if (va === vb) return a.name.localeCompare(b.name, 'pt-BR')
    return (va - vb) * mult
  })
}

/** Recorta a página. `page` é 1-based, como na URL. */
export function paginate<T>(rows: T[], page: number, pageSize: number): T[] {
  const start = (Math.max(1, page) - 1) * pageSize
  return rows.slice(start, start + pageSize)
}

export function totalPages(total: number, pageSize: number): number {
  return Math.max(1, Math.ceil(total / Math.max(1, pageSize)))
}
