import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AlertTriangle, History, Minus, Plus, Search } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import {
  Chip,
  EmptyState,
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { cn } from '@/lib/cn'
import { useAdjustStock, useAdminStock, useStockMovements } from '@/hooks/useAdminStock'
import { isStockAdminEnabled, STOCK_REASONS, type StockItem } from '@/lib/adminStockApi'

const inputCls =
  'w-full rounded-md border border-gray-300 bg-white px-2.5 py-1.5 text-sm text-gray-900 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

/** Quantidade com até 3 casas, sem zeros à toa (12, 2.5, 0.125). */
function fmtQty(n: number): string {
  return n.toLocaleString('pt-BR', { maximumFractionDigits: 3 })
}

function fmtWhen(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/**
 * Estoque — a tela do almoxarife.
 *
 * Ajuste RELATIVO com motivo (não sobrescrita), alerta de baixo (os baixos no
 * topo) e histórico por produto. Nunca mostra custo — o almoxarife não vê
 * custo, e a API nem devolve.
 */
export default function StockPage() {
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const low = params.get('baixo') === '1'

  const setParam = (key: string, value: string) => {
    setParams(
      (prev) => {
        const sp = new URLSearchParams(prev)
        if (value) sp.set(key, value)
        else sp.delete(key)
        return sp
      },
      { replace: true }
    )
  }

  const { data, isLoading, isError, error, refetch } = useAdminStock({ q, low, perPage: 50 })
  const rows = data?.data ?? []
  const [openId, setOpenId] = useState<string | null>(null)

  return (
    <AdminShell
      title="Estoque"
      description="Conferir, dar entrada/baixa com motivo e acompanhar o histórico — a trilha do almoxarifado."
    >
      <div className="space-y-4">
        {!isStockAdminEnabled && (
          <p className="rounded-md border border-gray-200 border-l-4 border-l-amber-500 bg-amber-50/60 p-3 text-xs leading-relaxed text-gray-700">
            <strong>Modo demonstração.</strong> O catálogo não está configurado (
            <code className="font-mono">VITE_CATALOG_URL</code> vazio): os números abaixo são
            inventados.
          </p>
        )}

        <Section title="Filtros">
          <div className="flex flex-wrap items-end gap-3 p-3 sm:p-4">
            <div className="min-w-[16rem] flex-1">
              <label htmlFor="st-q" className="block text-xs font-semibold text-gray-700">
                Buscar
              </label>
              <div className="relative mt-1">
                <Search
                  className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  aria-hidden="true"
                />
                <input
                  id="st-q"
                  type="search"
                  value={q}
                  onChange={(e) => setParam('q', e.target.value)}
                  placeholder="Nome ou SKU"
                  className={cn(inputCls, 'pl-8')}
                />
              </div>
            </div>
            <label className="flex items-center gap-2 pb-1.5 text-sm text-gray-700">
              <input
                type="checkbox"
                checked={low}
                onChange={(e) => setParam('baixo', e.target.checked ? '1' : '')}
                className="h-4 w-4 rounded border-gray-300 text-brand-orange focus:ring-brand-orange"
              />
              Só estoque baixo
            </label>
          </div>
        </Section>

        <Section
          title="Produtos"
          description={data?.meta ? `${data.meta.total} item(ns)` : undefined}
        >
          {isError ? (
            <div className="p-4">
              <ErrorState
                message={error instanceof Error ? error.message : 'Falha ao carregar'}
                onRetry={() => void refetch()}
              />
            </div>
          ) : isLoading ? (
            <LoadingRows rows={8} />
          ) : rows.length === 0 ? (
            <div className="p-4">
              <EmptyState title="Nenhum produto" description="Ajuste a busca ou o filtro." />
            </div>
          ) : (
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>SKU</Th>
                    <Th>Produto</Th>
                    <Th numeric>Estoque</Th>
                    <Th numeric>Limite</Th>
                    <Th className="text-right">Ações</Th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((item) => (
                    <StockRow
                      key={item.id}
                      item={item}
                      open={openId === item.id}
                      onToggle={() => setOpenId((cur) => (cur === item.id ? null : item.id))}
                    />
                  ))}
                </tbody>
              </Table>
            </ScrollArea>
          )}
        </Section>
      </div>
    </AdminShell>
  )
}

function StockRow({
  item,
  open,
  onToggle,
}: {
  item: StockItem
  open: boolean
  onToggle: () => void
}) {
  return (
    <>
      <tr className={cn('hover:bg-gray-50', item.lowStock && 'bg-red-50/40')}>
        <Td className="font-mono text-xs text-gray-500">{item.sku ?? '—'}</Td>
        <Td className="text-gray-800">{item.name}</Td>
        <Td numeric>
          <span className={cn('tabular-nums font-semibold', item.lowStock && 'text-red-700')}>
            {fmtQty(item.stock)}
          </span>
          {item.lowStock && (
            <Chip className="ml-2 bg-red-50 text-red-700 ring-red-600/20">
              <AlertTriangle className="mr-0.5 inline h-3 w-3" aria-hidden="true" />
              baixo
            </Chip>
          )}
        </Td>
        <Td numeric className="tabular-nums text-gray-500">
          {fmtQty(item.lowStockThreshold)}
        </Td>
        <Td className="text-right">
          <button
            type="button"
            onClick={onToggle}
            className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-semibold text-gray-700 hover:bg-gray-50"
          >
            {open ? 'Fechar' : 'Gerenciar'}
          </button>
        </Td>
      </tr>
      {open && (
        <tr className="bg-gray-50/70">
          <td colSpan={5} className="px-3 py-3 sm:px-4">
            <div className="grid gap-4 lg:grid-cols-2">
              <AdjustForm item={item} />
              <MovementHistory id={item.id} />
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

function AdjustForm({ item }: { item: StockItem }) {
  const adjust = useAdjustStock()
  const [dir, setDir] = useState<1 | -1>(1)
  const [qty, setQty] = useState('')
  const [reason, setReason] = useState<string>(STOCK_REASONS[0])

  const amount = Number(qty.replace(',', '.'))
  const valid = qty.trim() !== '' && Number.isFinite(amount) && amount > 0 && reason.trim() !== ''
  const delta = dir * amount
  const wouldBeNegative = valid && item.stock + delta < 0

  const submit = () => {
    if (!valid || wouldBeNegative) return
    adjust.mutate(
      { id: item.id, input: { delta, reason: reason.trim() } },
      {
        onSuccess: () => {
          setQty('')
        },
      }
    )
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-3">
      <h4 className="text-xs font-bold uppercase tracking-wide text-gray-500">Ajustar estoque</h4>
      <div className="mt-2 flex items-stretch gap-1.5">
        <div className="inline-flex overflow-hidden rounded-md border border-gray-300">
          <button
            type="button"
            onClick={() => setDir(1)}
            aria-pressed={dir === 1}
            className={cn(
              'flex items-center gap-1 px-2.5 text-sm font-semibold',
              dir === 1 ? 'bg-emerald-600 text-white' : 'text-gray-600 hover:bg-gray-50'
            )}
          >
            <Plus className="h-3.5 w-3.5" aria-hidden="true" /> Entrada
          </button>
          <button
            type="button"
            onClick={() => setDir(-1)}
            aria-pressed={dir === -1}
            className={cn(
              'flex items-center gap-1 px-2.5 text-sm font-semibold',
              dir === -1 ? 'bg-red-600 text-white' : 'text-gray-600 hover:bg-gray-50'
            )}
          >
            <Minus className="h-3.5 w-3.5" aria-hidden="true" /> Baixa
          </button>
        </div>
        <input
          type="text"
          inputMode="decimal"
          value={qty}
          onChange={(e) => setQty(e.target.value)}
          placeholder="Qtd."
          aria-label="Quantidade"
          className={cn(inputCls, 'w-24')}
        />
      </div>
      <label className="mt-2 block text-xs font-semibold text-gray-700">Motivo</label>
      <input
        list="stock-reasons"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        className={cn(inputCls, 'mt-1')}
        placeholder="Por que o estoque mudou?"
      />
      <datalist id="stock-reasons">
        {STOCK_REASONS.map((r) => (
          <option key={r} value={r} />
        ))}
      </datalist>

      {valid && (
        <p className="mt-2 text-xs text-gray-600">
          Novo estoque:{' '}
          <span className={cn('font-semibold', wouldBeNegative ? 'text-red-700' : 'text-gray-900')}>
            {wouldBeNegative ? 'inválido (ficaria negativo)' : fmtQty(item.stock + delta)}
          </span>
        </p>
      )}
      {adjust.isError && (
        <p className="mt-2 text-xs text-red-700">
          {adjust.error instanceof Error ? adjust.error.message : 'Falha ao ajustar'}
        </p>
      )}
      <button
        type="button"
        onClick={submit}
        disabled={!valid || wouldBeNegative || adjust.isPending}
        className="mt-2 inline-flex items-center gap-1.5 rounded-md bg-brand-orange px-3 py-1.5 text-xs font-semibold text-gray-900 hover:bg-brand-orange-dark disabled:opacity-50"
      >
        {adjust.isPending ? 'Aplicando…' : 'Aplicar ajuste'}
      </button>
    </div>
  )
}

function MovementHistory({ id }: { id: string }) {
  const { data: moves = [], isLoading } = useStockMovements(id)
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-3">
      <h4 className="flex items-center gap-1.5 text-xs font-bold uppercase tracking-wide text-gray-500">
        <History className="h-3.5 w-3.5" aria-hidden="true" /> Histórico
      </h4>
      {isLoading ? (
        <p className="mt-2 text-xs text-gray-400">Carregando…</p>
      ) : moves.length === 0 ? (
        <p className="mt-2 text-xs text-gray-400">Nenhum movimento registrado ainda.</p>
      ) : (
        <ul className="mt-2 space-y-1.5">
          {moves.map((m) => (
            <li key={m.id} className="flex items-baseline justify-between gap-2 text-xs">
              <span className="flex items-baseline gap-1.5">
                <span
                  className={cn(
                    'font-semibold tabular-nums',
                    m.delta >= 0 ? 'text-emerald-700' : 'text-red-700'
                  )}
                >
                  {m.delta >= 0 ? '+' : ''}
                  {fmtQty(m.delta)}
                </span>
                <span className="text-gray-700">{m.reason}</span>
              </span>
              <span className="whitespace-nowrap text-gray-400">
                → {fmtQty(m.resultingStock)} · {fmtWhen(m.createdAt)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
