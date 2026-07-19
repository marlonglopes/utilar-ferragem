import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { ArrowDown, ArrowUp } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { PeriodPicker } from '@/components/admin/PeriodPicker'
import { Meter, Sparkline } from '@/components/admin/charts'
import {
  EmptyState,
  ErrorState,
  LoadingRows,
  Money,
  ScrollArea,
  Section,
  SeverityPill,
  StatTile,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { CHART, SEVERITY_TEXT } from '@/components/admin/tokens'
import { cn } from '@/lib/cn'
import { useAdminPeriod } from '@/hooks/useAdminPeriod'
import { useAdminSellers } from '@/hooks/useAdminQueries'
import {
  discountSeverity,
  formatCents,
  formatCentsCompact,
  formatCount,
  formatPercent,
  marginSeverity,
  sellerTotals,
  sortSellers,
  type SellerSortKey,
} from '@/lib/adminFormat'

/**
 * Desempenho dos vendedores.
 *
 * O pedido do dono foi explícito: separar "quem vende muito" de "quem vende
 * bem". Por isso desconto médio e margem média não são colunas secundárias —
 * carregam severidade própria (pill + medidor) e são ordenáveis, exatamente
 * como o total vendido. Um vendedor no topo do volume com margem vermelha
 * aparece vermelho no topo.
 */

const COLUMNS: Array<{ key: SellerSortKey; label: string; hint?: string }> = [
  { key: 'totalCents', label: 'Total vendido' },
  { key: 'orderCount', label: 'Pedidos' },
  { key: 'avgTicketCents', label: 'Ticket médio' },
  { key: 'avgDiscountPct', label: 'Desc. médio', hint: 'Desconto médio concedido sobre o bruto' },
  { key: 'avgMarginPct', label: 'Margem média', hint: 'Margem realizada, já agregada no backend' },
  { key: 'managerApprovals', label: 'Aprov. gerente', hint: 'Pedidos que estouraram o teto de desconto do cargo' },
]

export default function AdminSellersPage() {
  const periodCtl = useAdminPeriod('month')
  const { period } = periodCtl
  const [params, setParams] = useSearchParams()

  const storeId = params.get('loja') ?? ''
  const sortKey = (params.get('ordem') as SellerSortKey | null) ?? 'totalCents'
  const sortDir = params.get('dir') === 'asc' ? 'asc' : 'desc'

  const setParam = useCallback(
    (key: string, value: string) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (value) sp.set(key, value)
          else sp.delete(key)
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const toggleSort = useCallback(
    (key: SellerSortKey) => {
      if (key === sortKey) {
        setParam('dir', sortDir === 'desc' ? 'asc' : 'desc')
      } else {
        setParams(
          (prev) => {
            const sp = new URLSearchParams(prev)
            sp.set('ordem', key)
            sp.set('dir', 'desc')
            return sp
          },
          { replace: true },
        )
      }
    },
    [sortKey, sortDir, setParam, setParams],
  )

  const { data, isLoading, isError, error, refetch } = useAdminSellers(period, storeId || undefined)

  const sellers = useMemo(() => {
    if (!data) return []
    const filtered = storeId ? data.sellers.filter((s) => s.storeId === storeId) : data.sellers
    return sortSellers(filtered, sortKey, sortDir)
  }, [data, storeId, sortKey, sortDir])

  const totals = useMemo(() => sellerTotals(sellers), [sellers])

  /** Referência das barras da coluna de volume: o maior do recorte atual. */
  const maxTotal = sellers.reduce((m, s) => Math.max(m, s.totalCents), 0) || 1

  return (
    <AdminShell
      title="Desempenho dos vendedores"
      description="Quem vende muito e quem vende bem — não são sempre os mesmos."
      toolbar={
        <div className="flex flex-wrap items-center gap-2">
          <label className="sr-only" htmlFor="seller-store">Loja</label>
          <select
            id="seller-store"
            value={storeId}
            onChange={(e) => setParam('loja', e.target.value)}
            className="rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-700 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue"
          >
            <option value="">Todas as lojas</option>
            {(data?.stores ?? []).map((s) => (
              <option key={s.id} value={s.id}>{s.name}</option>
            ))}
          </select>
          <PeriodPicker {...periodCtl} />
        </div>
      }
    >
      {isError && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <ErrorState message={(error as Error)?.message ?? 'Erro desconhecido'} onRetry={() => void refetch()} />
        </div>
      )}

      {isLoading && !data && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <LoadingRows rows={8} />
        </div>
      )}

      {data && (
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
            <StatTile label="Total vendido" value={formatCentsCompact(totals.totalCents)} />
            <StatTile label="Pedidos" value={formatCount(totals.orderCount)} />
            <StatTile label="Ticket médio" value={formatCents(totals.avgTicketCents)} />
            <StatTile
              label="Desconto médio"
              value={formatPercent(totals.avgDiscountPct)}
              severity={discountSeverity(totals.avgDiscountPct)}
              hint="Ponderado por valor vendido"
            />
            <StatTile
              label="Margem média"
              value={formatPercent(totals.avgMarginPct)}
              severity={marginSeverity(totals.avgMarginPct)}
              hint="Ponderada por valor vendido"
            />
          </div>

          <Section
            title="Ranking"
            description="Clique no cabeçalho para reordenar. Margem e desconto carregam severidade própria."
          >
            {sellers.length === 0 ? (
              <EmptyState title="Nenhum vendedor no recorte" description="Ajuste o período ou a loja." />
            ) : (
              <ScrollArea>
                <Table>
                  <thead>
                    <tr>
                      <Th className="sticky left-0 z-10 bg-gray-50">#</Th>
                      <Th className="sticky left-8 z-10 bg-gray-50">Vendedor</Th>
                      <Th>Loja</Th>
                      {COLUMNS.map((c) => (
                        <Th
                          key={c.key}
                          numeric
                          className="p-0"
                          // `aria-sort` pertence ao cabeçalho de coluna, não ao
                          // botão dentro dele — o leitor de tela procura no
                          // `columnheader`.
                          aria-sort={
                            sortKey === c.key
                              ? sortDir === 'asc'
                                ? 'ascending'
                                : 'descending'
                              : 'none'
                          }
                        >
                          <button
                            type="button"
                            onClick={() => toggleSort(c.key)}
                            title={c.hint}
                            className={cn(
                              'flex w-full items-center justify-end gap-1 px-3 py-2 text-xs font-semibold uppercase tracking-wide hover:text-gray-900',
                              sortKey === c.key ? 'text-brand-blue' : 'text-gray-500',
                            )}
                          >
                            {c.label}
                            {sortKey === c.key &&
                              (sortDir === 'desc' ? (
                                <ArrowDown className="h-3 w-3" aria-hidden="true" />
                              ) : (
                                <ArrowUp className="h-3 w-3" aria-hidden="true" />
                              ))}
                          </button>
                        </Th>
                      ))}
                      <Th className="w-28">Tendência</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {sellers.map((s, i) => {
                      const mSev = marginSeverity(s.avgMarginPct)
                      const dSev = discountSeverity(s.avgDiscountPct)
                      return (
                        <tr key={s.sellerId} className="hover:bg-gray-50">
                          <Td className="sticky left-0 z-10 bg-white tabular-nums text-xs text-gray-400">
                            {i + 1}
                          </Td>
                          <Td className="sticky left-8 z-10 whitespace-nowrap bg-white font-medium text-gray-900">
                            {s.sellerName}
                          </Td>
                          <Td className="whitespace-nowrap text-xs text-gray-500">{s.storeName}</Td>
                          <Td numeric>
                            <Money cents={s.totalCents} emphasis />
                            {/* Barra proporcional embaixo do número: a comparação
                                entre linhas vira geometria, não aritmética mental. */}
                            <Meter value={s.totalCents / maxTotal} className="mt-1" />
                          </Td>
                          <Td numeric>{formatCount(s.orderCount)}</Td>
                          <Td numeric><Money cents={s.avgTicketCents} /></Td>
                          <Td numeric>
                            <span className={cn('font-semibold tabular-nums', SEVERITY_TEXT[dSev])}>
                              {formatPercent(s.avgDiscountPct)}
                            </span>
                          </Td>
                          <Td numeric>
                            <SeverityPill severity={mSev}>{formatPercent(s.avgMarginPct)}</SeverityPill>
                          </Td>
                          <Td numeric>
                            {s.managerApprovals > 0 ? (
                              <span className="tabular-nums text-gray-700">{s.managerApprovals}</span>
                            ) : (
                              <span className="text-gray-300">—</span>
                            )}
                          </Td>
                          <Td>
                            <Sparkline
                              values={s.series.map((p) => p.valueCents)}
                              width={96}
                              height={26}
                              color={CHART.primary}
                              label={`Vendas diárias de ${s.sellerName}`}
                            />
                          </Td>
                        </tr>
                      )
                    })}
                  </tbody>
                  <tfoot>
                    <tr className="bg-gray-50 font-semibold">
                      <Td className="sticky left-0 z-10 bg-gray-50">&nbsp;</Td>
                      <Td className="sticky left-8 z-10 bg-gray-50 text-gray-900">
                        {sellers.length} vendedores
                      </Td>
                      <Td>&nbsp;</Td>
                      <Td numeric><Money cents={totals.totalCents} emphasis /></Td>
                      <Td numeric>{formatCount(totals.orderCount)}</Td>
                      <Td numeric><Money cents={totals.avgTicketCents} emphasis /></Td>
                      <Td numeric>{formatPercent(totals.avgDiscountPct)}</Td>
                      <Td numeric>{formatPercent(totals.avgMarginPct)}</Td>
                      <Td numeric>{formatCount(totals.managerApprovals)}</Td>
                      <Td>&nbsp;</Td>
                    </tr>
                  </tfoot>
                </Table>
              </ScrollArea>
            )}
          </Section>

          <p className="px-1 text-[11px] leading-relaxed text-gray-500">
            Margem é agregada no backend a partir do custo dos itens — o custo unitário nunca é
            enviado ao navegador. As médias do rodapé são ponderadas por valor vendido, não médias
            simples: um vendedor com 2 pedidos não pode mover a margem da loja como um com 200.
          </p>
        </div>
      )}
    </AdminShell>
  )
}
