import { useMemo } from 'react'
import { useProducts } from '@/hooks/useProducts'
import { ASSUMED_COST_RATIO, round2, type NewBalcaoItem } from '@/store/balcaoStore'
import type { Product } from '@/types/product'

/**
 * Busca de produtos para o PDV de balcão.
 *
 * Reusa `useProducts` (que já cobre modo mock e catalog-service real). A camada
 * extra aqui existe porque o balcão precisa de três coisas que o catálogo do
 * e-commerce não expõe: SKU, código de barras e custo.
 */

// ---------------------------------------------------------------------------
// Derivações — tudo isto vira campo real assim que o backend existir
// ---------------------------------------------------------------------------

/**
 * TODO(backend): `Product` não tem `sku`. O catalog-service precisa expor o SKU
 * real do produto (coluna já existe no seed? não — precisa de migration).
 * Enquanto isso o SKU é derivado do id de forma determinística, o que é
 * suficiente para demonstrar a busca mas NÃO casa com a etiqueta física.
 */
export function deriveSku(product: Product): string {
  const prefix = (product.category ?? 'ger').slice(0, 3).toUpperCase()
  const num = String(product.id).replace(/\D/g, '').padStart(5, '0').slice(-5)
  return `${prefix}-${num}`
}

/**
 * TODO(backend): idem para EAN/GTIN. Sem o código de barras real vindo do
 * catalog-service, o leitor de código de barras físico não vai casar com nada
 * em produção — este valor é só para o protótipo/demo.
 */
export function deriveBarcode(product: Product): string {
  const digits = String(product.id).replace(/\D/g, '').padStart(6, '0').slice(-6)
  return `789${digits}0000`.slice(0, 13)
}

/**
 * TODO(backend): `Product` não tem `cost`. A barra de margem é o coração do
 * bloco de negociação e hoje ela opera sobre um custo ESTIMADO
 * (preço × {@link ASSUMED_COST_RATIO}). Até o catalog-service expor custo real
 * (idealmente custo médio por seller), os números de margem são ilustrativos.
 */
export function deriveUnitCost(product: Product): number {
  return round2(product.price * ASSUMED_COST_RATIO)
}

const UNIT_BY_CATEGORY: Record<string, string> = {
  construcao: 'sc',
  eletrica: 'rl',
  hidraulica: 'pç',
  pintura: 'lt',
  fixacao: 'cx',
}

/** TODO(backend): unidade de venda também não existe em `Product`. */
export function deriveUnit(product: Product): string {
  return UNIT_BY_CATEGORY[product.category] ?? 'un'
}

/** Converte um `Product` do catálogo em item pronto para a comanda. */
export function toBalcaoItem(product: Product, quantity = 1): NewBalcaoItem {
  return {
    productId: product.id,
    sku: deriveSku(product),
    barcode: deriveBarcode(product),
    name: product.name,
    icon: product.icon,
    unit: deriveUnit(product),
    unitPrice: product.price,
    unitCost: deriveUnitCost(product),
    quantity,
    stock: product.stock,
  }
}

// ---------------------------------------------------------------------------
// Classificação da query
// ---------------------------------------------------------------------------

export type QueryKind = 'empty' | 'barcode' | 'sku' | 'text'

/** Só dígitos e comprido = leitura de scanner. Com hífen/alfanumérico = SKU. */
export function classifyQuery(raw: string): QueryKind {
  const q = raw.trim()
  if (!q) return 'empty'
  if (/^\d{8,14}$/.test(q)) return 'barcode'
  if (/^[a-z]{2,4}-?\d{2,6}$/i.test(q)) return 'sku'
  return 'text'
}

function normalize(value: string): string {
  return value
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
}

/** Filtro client-side por SKU / código de barras / nome. */
export function matchesQuery(product: Product, raw: string): boolean {
  const q = normalize(raw.trim())
  if (!q) return true
  const compact = q.replace(/-/g, '')
  return (
    normalize(product.name).includes(q) ||
    normalize(deriveSku(product)).replace(/-/g, '').includes(compact) ||
    deriveBarcode(product).includes(compact)
  )
}

export interface UseBalcaoProductsResult {
  products: Product[]
  isLoading: boolean
  isError: boolean
  /** Como a query foi interpretada — a UI mostra isso como dica. */
  kind: QueryKind
  /** Houve busca por SKU/barcode resolvida no cliente. */
  clientFiltered: boolean
}

/**
 * TODO(backend): o catalog-service só aceita `q` como busca textual sobre nome/
 * descrição/seller — não indexa SKU nem código de barras. Enquanto isso, quando
 * a query parece SKU ou EAN, puxamos uma página larga e filtramos no cliente.
 * Isso NÃO escala além de algumas centenas de SKUs: o balcão precisa de
 * `GET /api/v1/products?sku=` e `?barcode=` (lookup exato, indexado) no
 * catalog-service.
 */
export function useBalcaoProducts(rawQuery: string, limit = 24): UseBalcaoProductsResult {
  const query = rawQuery.trim()
  const kind = classifyQuery(query)
  const clientFiltered = kind === 'sku' || kind === 'barcode'

  const { data, isLoading, isError } = useProducts(
    clientFiltered
      ? { per_page: 100 } // sem `q`: o backend não sabe filtrar por SKU/EAN
      : { q: query || undefined, per_page: limit }
  )

  const products = useMemo(() => {
    const all = data?.data ?? []
    if (!clientFiltered) return all.slice(0, limit)
    return all.filter((p) => matchesQuery(p, query)).slice(0, limit)
  }, [data, clientFiltered, query, limit])

  return { products, isLoading, isError, kind, clientFiltered }
}
