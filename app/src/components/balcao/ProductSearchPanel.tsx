import { forwardRef, useState } from 'react'
import { Search, ScanLine, X } from 'lucide-react'
import { Skeleton } from '@/components/ui'
import { formatCurrency } from '@/lib/format'
import { cn } from '@/lib/cn'
import { StockChip } from './StockChip'
import {
  useBalcaoProducts,
  deriveSku,
  deriveUnit,
  type QueryKind,
} from '@/hooks/useBalcaoProducts'
import type { Product } from '@/types/product'

const KIND_HINT: Record<QueryKind, string> = {
  empty: 'Busque por nome, SKU ou código de barras · F2',
  text: 'Buscando por nome',
  sku: 'Buscando por SKU',
  barcode: 'Código de barras lido',
}

interface ProductTileProps {
  product: Product
  onAdd: (product: Product) => void
}

/**
 * Card tocável de produto. O alvo de toque é o card inteiro (min-h 128px), não
 * um botãozinho — o vendedor usa em pé, muitas vezes com a mão suja.
 */
function ProductTile({ product, onAdd }: ProductTileProps) {
  const out = product.stock <= 0
  return (
    <button
      type="button"
      disabled={out}
      onClick={() => onAdd(product)}
      className={cn(
        'flex min-h-[128px] flex-col items-start gap-1 rounded-xl border border-gray-200 bg-white p-3 text-left transition-colors',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange',
        out ? 'opacity-60' : 'hover:border-brand-orange hover:bg-orange-50 active:bg-orange-100'
      )}
    >
      <div className="flex w-full items-start justify-between gap-2">
        <span className="text-2xl leading-none" aria-hidden="true">
          {product.icon}
        </span>
        <span className="font-mono text-[11px] font-semibold text-gray-500">
          {deriveSku(product)}
        </span>
      </div>

      <p className="line-clamp-2 text-sm font-semibold leading-snug text-gray-900">
        {product.name}
      </p>

      <div className="mt-auto flex w-full items-end justify-between gap-2">
        <span className="font-display text-base font-bold text-brand-blue">
          {formatCurrency(product.price)}
          <span className="ml-1 text-xs font-medium text-gray-500">/{deriveUnit(product)}</span>
        </span>
      </div>
      <StockChip stock={product.stock} />
    </button>
  )
}

export interface ProductSearchPanelProps {
  query: string
  onQueryChange: (value: string) => void
  onAdd: (product: Product) => void
}

/**
 * Coluna esquerda do PDV: busca grande + grade de produtos.
 *
 * O input é exposto por ref para o atalho F2 dar foco nele sem que a página
 * precise conhecer o DOM interno.
 */
export const ProductSearchPanel = forwardRef<HTMLInputElement, ProductSearchPanelProps>(
  function ProductSearchPanel({ query, onQueryChange, onAdd }, ref) {
    const { products, isLoading, kind, clientFiltered } = useBalcaoProducts(query)
    const [scanHint, setScanHint] = useState(false)

    return (
      <section className="flex min-h-0 flex-1 flex-col gap-4 p-4" aria-label="Busca de produtos">
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search
              className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-400"
              aria-hidden="true"
            />
            <input
              ref={ref}
              type="search"
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              placeholder="Nome, SKU ou código de barras"
              aria-label="Buscar produto por nome, SKU ou código de barras"
              className="h-14 w-full rounded-xl border border-gray-300 bg-white pl-12 pr-12 text-base text-gray-900 placeholder:text-gray-400 focus:border-transparent focus:outline-none focus:ring-2 focus:ring-brand-orange"
            />
            {query && (
              <button
                type="button"
                onClick={() => onQueryChange('')}
                aria-label="Limpar busca"
                className="absolute right-2 top-1/2 flex h-12 w-12 -translate-y-1/2 items-center justify-center rounded-lg text-gray-400 hover:bg-gray-100"
              >
                <X className="h-5 w-5" aria-hidden="true" />
              </button>
            )}
          </div>

          {/*
            TODO(hardware): "Escanear" ainda não abre a câmera nem fala com
            leitor USB. Leitores de código de barras USB agem como teclado e já
            funcionam hoje: eles digitam no input focado e dão Enter. Este botão
            só foca o campo e explica isso. Câmera (BarcodeDetector / getUserMedia)
            fica para quando houver tablet homologado.
          */}
          <button
            type="button"
            onClick={() => {
              setScanHint(true)
              if (typeof ref === 'object' && ref?.current) ref.current.focus()
            }}
            className="flex h-14 min-w-[120px] items-center justify-center gap-2 rounded-xl bg-brand-blue px-4 font-semibold text-white transition-colors hover:bg-brand-blue-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-blue"
          >
            <ScanLine className="h-5 w-5" aria-hidden="true" />
            Escanear
          </button>
        </div>

        <p className="text-xs font-medium text-gray-500" role="status">
          {scanHint && kind === 'empty'
            ? 'Aponte o leitor para o código — ele digita direto no campo.'
            : KIND_HINT[kind]}
          {clientFiltered && ' · filtrado no dispositivo'}
        </p>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {isLoading ? (
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-32 rounded-xl" />
              ))}
            </div>
          ) : products.length === 0 ? (
            <div className="flex h-48 flex-col items-center justify-center rounded-xl border border-dashed border-gray-300 text-center">
              <p className="font-semibold text-gray-700">Nenhum produto encontrado</p>
              <p className="mt-1 text-sm text-gray-500">
                Confira o SKU ou tente parte do nome.
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4">
              {products.map((p) => (
                <ProductTile key={p.id} product={p} onAdd={onAdd} />
              ))}
            </div>
          )}
        </div>
      </section>
    )
  }
)
