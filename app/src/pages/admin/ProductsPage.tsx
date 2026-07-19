import { useCallback, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, ImageOff, Plus, Search } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import {
  EmptyState,
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import {
  MarginCell,
  Reais,
  StatusPill,
} from '@/components/admin/products/productPrimitives'
import { cn } from '@/lib/cn'
import { formatCount } from '@/lib/adminFormat'
import { STATUS_LABEL } from '@/lib/adminProductFormat'
import { TOP_LEVEL_CATEGORIES } from '@/lib/taxonomy'
import { isProductAdminEnabled } from '@/lib/adminProductsApi'
import { useAdminProductList, useBulkStatus } from '@/hooks/useAdminProducts'
import {
  PRODUCT_STATUSES,
  isProductStatus,
  type AdminProductQuery,
  type AdminProductRow,
  type ProductSortKey,
  type ProductStatus,
} from '@/lib/adminProductTypes'

/**
 * Lista de produtos do painel.
 *
 * Até aqui o dono não tinha como editar um produto pela tela: ou reimportava a
 * planilha inteira, ou chamava a API na mão. Esta é a porta de entrada da
 * edição — e, principalmente, é onde o **passo que falta depois de importar**
 * acontece: a importação entra tudo como rascunho de propósito, e alguém
 * precisa publicar.
 *
 * ⚠️ Esta tela exibe CUSTO e MARGEM. O painel é o lugar certo para isso, mas o
 * dado não pode escapar dele: o filtro que vai para a URL é busca, categoria,
 * status, ordenação e página — **nunca valor de custo**. Ordenar por margem
 * (`ordem=margem`) não carrega número nenhum na query string.
 */

const PAGE_SIZE = 25

interface Column {
  key: ProductSortKey | null
  label: string
  numeric?: boolean
  /** Colunas que somem no celular — a tabela rola, mas o essencial vem primeiro. */
  hideBelow?: string
}

const COLUMNS: Column[] = [
  { key: null, label: 'Foto' },
  { key: null, label: 'SKU', hideBelow: 'hidden sm:table-cell' },
  { key: 'name', label: 'Produto' },
  { key: null, label: 'Categoria', hideBelow: 'hidden lg:table-cell' },
  { key: 'price', label: 'Preço', numeric: true },
  { key: 'cost', label: 'Custo', numeric: true },
  { key: 'margin', label: 'Margem', numeric: true },
  { key: 'stock', label: 'Estoque', numeric: true, hideBelow: 'hidden sm:table-cell' },
  { key: null, label: 'Situação' },
]

export default function AdminProductsPage() {
  const [params, setParams] = useSearchParams()
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const q = params.get('busca') ?? ''
  const category = params.get('categoria') ?? ''
  const statusParam = params.get('situacao') ?? ''
  const status: ProductStatus | '' = isProductStatus(statusParam) ? statusParam : ''
  const sort = (params.get('ordem') as ProductSortKey | null) ?? 'name'
  const dir = params.get('dir') === 'desc' ? 'desc' : 'asc'
  const page = Math.max(1, Number(params.get('pagina') ?? '1') || 1)

  const setParam = useCallback(
    (key: string, value: string, resetPage = true) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (value) sp.set(key, value)
          else sp.delete(key)
          if (resetPage) sp.delete('pagina')
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const query: AdminProductQuery = useMemo(
    () => ({ q, category, status, sort, dir, page, pageSize: PAGE_SIZE }),
    [q, category, status, sort, dir, page],
  )

  const { data, isLoading, isError, error, refetch } = useAdminProductList(query)
  const bulk = useBulkStatus()

  // Memoizado, e não `data?.data ?? []`: aquele `??` cria um array novo a cada
  // render, o que trocaria a identidade das dependências de `toggleAll` e
  // `selectedRows` em todo ciclo — e a seleção em lote é justamente o estado
  // que não pode ser recriado embaixo do dono no meio da escolha.
  const rows = useMemo(() => data?.data ?? [], [data])
  const meta = data?.meta

  const toggleSort = useCallback(
    (key: ProductSortKey) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (sp.get('ordem') === key) {
            sp.set('dir', sp.get('dir') === 'desc' ? 'asc' : 'desc')
          } else {
            sp.set('ordem', key)
            sp.set('dir', 'asc')
          }
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const toggleOne = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const allOnPageSelected = rows.length > 0 && rows.every((r) => selected.has(r.id))

  const toggleAll = useCallback(() => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (rows.every((r) => next.has(r.id))) rows.forEach((r) => next.delete(r.id))
      else rows.forEach((r) => next.add(r.id))
      return next
    })
  }, [rows])

  const selectedRows: AdminProductRow[] = useMemo(
    () => rows.filter((r) => selected.has(r.id)),
    [rows, selected],
  )

  const runBulk = useCallback(
    (next: ProductStatus) => {
      if (selectedRows.length === 0) return
      bulk.mutate(
        { items: selectedRows.map((r) => ({ id: r.id, name: r.name })), status: next },
        { onSuccess: () => setSelected(new Set()) },
      )
    },
    [bulk, selectedRows],
  )

  const totalPagesCount = meta?.totalPages ?? 1

  return (
    <AdminShell
      title="Produtos"
      description="Editar, publicar e cadastrar. É aqui que o rascunho da planilha vira produto na loja."
      toolbar={
        <Link
          to="/admin/produtos/novo"
          className="inline-flex items-center gap-1.5 rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-orange/90"
        >
          <Plus className="h-3.5 w-3.5" aria-hidden="true" />
          Novo produto
        </Link>
      }
    >
      <div className="space-y-4">
        {!isProductAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado (
            <code className="font-mono">VITE_CATALOG_URL</code> vazio): os produtos, custos e
            margens abaixo são <strong>inventados</strong> e nada é gravado. Serve para conhecer a
            tela, não para operar a loja.
          </p>
        )}

        {/* --------------------------------------------------- Filtros */}
        <Section title="Filtros">
          <div className="grid gap-3 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
            <div className="lg:col-span-2">
              <label htmlFor="pf-q" className="block text-xs font-semibold text-gray-700">
                Buscar
              </label>
              <div className="relative mt-1">
                <Search
                  className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  aria-hidden="true"
                />
                <input
                  id="pf-q"
                  type="search"
                  value={q}
                  onChange={(e) => setParam('busca', e.target.value)}
                  placeholder="Nome, SKU ou marca"
                  className="w-full rounded-md border border-gray-300 bg-white py-1.5 pl-8 pr-2.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
                />
              </div>
            </div>

            <div>
              <label htmlFor="pf-cat" className="block text-xs font-semibold text-gray-700">
                Categoria
              </label>
              <select
                id="pf-cat"
                value={category}
                onChange={(e) => setParam('categoria', e.target.value)}
                className="mt-1 w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
              >
                <option value="">Todas</option>
                {TOP_LEVEL_CATEGORIES.map((c) => (
                  <option key={c.slug} value={c.slug}>
                    {c.slug}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label htmlFor="pf-status" className="block text-xs font-semibold text-gray-700">
                Situação
              </label>
              <select
                id="pf-status"
                value={status}
                onChange={(e) => setParam('situacao', e.target.value)}
                className="mt-1 w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
              >
                <option value="">Todas</option>
                {PRODUCT_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {STATUS_LABEL[s]}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </Section>

        {/* ---------------------------------------------- Ação em lote */}
        {selected.size > 0 && (
          <div
            className="flex flex-wrap items-center gap-2 rounded-md border border-brand-blue/30 bg-brand-blue-light/40 p-3"
            role="region"
            aria-label="Ações em lote"
          >
            <p className="text-xs font-semibold text-brand-blue">
              {formatCount(selected.size)} selecionado{selected.size === 1 ? '' : 's'}
            </p>
            <div className="flex-1" />
            <button
              type="button"
              onClick={() => runBulk('published')}
              disabled={bulk.isPending}
              className="rounded-md bg-brand-blue px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:bg-gray-300"
            >
              Publicar
            </button>
            <button
              type="button"
              onClick={() => runBulk('draft')}
              disabled={bulk.isPending}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Voltar a rascunho
            </button>
            <button
              type="button"
              onClick={() => setSelected(new Set())}
              className="rounded-md px-2 py-1.5 text-xs font-semibold text-gray-600 hover:bg-white"
            >
              Limpar seleção
            </button>
          </div>
        )}

        {/* Resultado do lote: por item, porque falha parcial é o caso comum. */}
        {bulk.data && (
          <div
            role="status"
            className={cn(
              'rounded-md border p-3 text-xs leading-relaxed',
              bulk.data.failed.length > 0
                ? 'border-amber-200 border-l-4 border-l-amber-500 bg-amber-50/70 text-amber-900'
                : 'border-green-200 border-l-4 border-l-green-600 bg-green-50 text-green-900',
            )}
          >
            <p className="font-semibold">
              {bulk.data.ok.length} produto{bulk.data.ok.length === 1 ? '' : 's'} atualizado
              {bulk.data.ok.length === 1 ? '' : 's'}
              {bulk.data.failed.length > 0 && `; ${bulk.data.failed.length} falhou`}
            </p>
            {bulk.data.failed.length > 0 && (
              <ul className="mt-1.5 space-y-1">
                {bulk.data.failed.map((f) => (
                  <li key={f.id}>
                    <strong>{f.name}</strong> — {f.message}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}

        {/* ---------------------------------------------------- Tabela */}
        <Section
          title="Catálogo"
          description={
            meta
              ? `${formatCount(meta.total)} produto${meta.total === 1 ? '' : 's'} no filtro atual`
              : undefined
          }
        >
          {isLoading ? (
            <LoadingRows rows={8} />
          ) : isError ? (
            <ErrorState
              message={(error as Error).message}
              onRetry={() => {
                void refetch()
              }}
            />
          ) : rows.length === 0 ? (
            <EmptyState
              title="Nenhum produto neste filtro"
              description="Ajuste a busca, a categoria ou a situação. Produtos recém-importados ficam em Rascunho."
            />
          ) : (
            // A rolagem horizontal fica NO contêiner, nunca no body: se a página
            // inteira rolasse, o cabeçalho sairia da tela no celular.
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th className="w-8">
                      <input
                        type="checkbox"
                        checked={allOnPageSelected}
                        onChange={toggleAll}
                        aria-label="Selecionar todos desta página"
                        className="h-4 w-4 rounded border-gray-300 text-brand-blue focus:ring-brand-blue"
                      />
                    </Th>
                    {COLUMNS.map((col) => (
                      <Th key={col.label} numeric={col.numeric} className={col.hideBelow}>
                        {col.key ? (
                          <button
                            type="button"
                            onClick={() => toggleSort(col.key as ProductSortKey)}
                            className="inline-flex items-center gap-1 hover:text-gray-900"
                          >
                            {col.label}
                            {sort === col.key &&
                              (dir === 'asc' ? (
                                <ArrowUp className="h-3 w-3" aria-hidden="true" />
                              ) : (
                                <ArrowDown className="h-3 w-3" aria-hidden="true" />
                              ))}
                          </button>
                        ) : (
                          col.label
                        )}
                      </Th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id} className="hover:bg-gray-50">
                      <Td>
                        <input
                          type="checkbox"
                          checked={selected.has(r.id)}
                          onChange={() => toggleOne(r.id)}
                          aria-label={`Selecionar ${r.name}`}
                          className="h-4 w-4 rounded border-gray-300 text-brand-blue focus:ring-brand-blue"
                        />
                      </Td>
                      <Td>
                        {r.imageUrl ? (
                          <img
                            src={r.imageUrl}
                            alt=""
                            loading="lazy"
                            className="h-9 w-9 rounded border border-gray-200 object-contain"
                          />
                        ) : (
                          <span
                            className="flex h-9 w-9 items-center justify-center rounded border border-dashed border-gray-300 text-gray-300"
                            title="Sem foto"
                          >
                            <ImageOff className="h-4 w-4" aria-hidden="true" />
                          </span>
                        )}
                      </Td>
                      <Td className="hidden font-mono text-xs text-gray-500 sm:table-cell">
                        {r.sku ?? '—'}
                      </Td>
                      <Td>
                        <Link
                          to={`/admin/produtos/${r.id}`}
                          className="font-medium text-brand-blue hover:underline"
                        >
                          {r.name}
                        </Link>
                        {r.brand && <p className="text-xs text-gray-500">{r.brand}</p>}
                      </Td>
                      <Td className="hidden text-xs text-gray-600 lg:table-cell">{r.category}</Td>
                      <Td numeric>
                        <Reais value={r.price} />
                      </Td>
                      <Td numeric>
                        <Reais value={r.cost} />
                      </Td>
                      <Td numeric>
                        <MarginCell price={r.price} cost={r.cost} />
                      </Td>
                      <Td numeric className="hidden sm:table-cell">
                        {r.stock.toLocaleString('pt-BR')} {r.unitOfMeasure}
                      </Td>
                      <Td>
                        <StatusPill status={r.status} />
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </ScrollArea>
          )}

          {/* ------------------------------------------------ Paginação */}
          {meta && totalPagesCount > 1 && (
            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 px-3 py-2 sm:px-4">
              <p className="text-xs text-gray-500">
                Página {meta.page} de {totalPagesCount}
              </p>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  disabled={page <= 1}
                  onClick={() => setParam('pagina', String(page - 1), false)}
                  className="rounded border border-gray-300 px-2.5 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-40"
                >
                  Anterior
                </button>
                <button
                  type="button"
                  disabled={page >= totalPagesCount}
                  onClick={() => setParam('pagina', String(page + 1), false)}
                  className="rounded border border-gray-300 px-2.5 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-50 disabled:opacity-40"
                >
                  Próxima
                </button>
              </div>
            </div>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}
