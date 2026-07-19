import { useCallback, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AlertTriangle, Download } from 'lucide-react'
import { AdminShell } from '@/components/admin/AdminShell'
import { PeriodPicker } from '@/components/admin/PeriodPicker'
import { Sparkline } from '@/components/admin/charts'
import {
  Chip,
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
import { CHART } from '@/components/admin/tokens'
import { useAdminPeriod } from '@/hooks/useAdminPeriod'
import {
  useAdminAccounting,
  useAdminLedger,
  useAdminReconciliation,
} from '@/hooks/useAdminQueries'
import { fetchAccountingExport } from '@/lib/adminApi'
import {
  accountingImbalanceCents,
  formatCents,
  formatCentsCompact,
  formatCount,
  formatPercent,
  ledgerKindLabel,
  methodLabel,
} from '@/lib/adminFormat'
import { downloadBlob, downloadCsv, ledgerFilename, ledgerToCsv } from '@/lib/adminExport'
import { formatDateTime } from '@/lib/format'
import type { LedgerEntry, ReconciliationItem } from '@/lib/adminTypes'

/**
 * Auditoria contábil — o núcleo do que o dono pediu.
 *
 * Hierarquia: o resultado (bruto → taxas → líquido) vem primeiro em números
 * grandes, a quebra por método logo abaixo, as divergências de reconciliação em
 * destaque (porque é o único bloco que pede ação), e o livro-razão navegável
 * por último, que é onde se confere linha a linha.
 */

const RECON_TYPE_LABEL: Record<ReconciliationItem['type'], string> = {
  missing_in_ledger: 'Falta no nosso livro',
  missing_in_psp: 'Falta no extrato do PSP',
  amount_mismatch: 'Valor divergente',
  fee_mismatch: 'Taxa divergente',
}

const KIND_OPTIONS: Array<{ value: string; label: string }> = [
  { value: '', label: 'Todas as naturezas' },
  { value: 'sale', label: 'Venda' },
  { value: 'psp_fee', label: 'Taxa PSP' },
  { value: 'refund', label: 'Estorno' },
  { value: 'chargeback', label: 'Chargeback' },
  { value: 'adjustment', label: 'Ajuste' },
  { value: 'payout', label: 'Repasse' },
]

const METHOD_OPTIONS: Array<{ value: string; label: string }> = [
  { value: '', label: 'Todos os métodos' },
  { value: 'pix', label: 'Pix' },
  // `card` é o valor gravado no razão; crédito/débito vêm do funil e do balcão.
  { value: 'card', label: 'Cartão' },
  { value: 'credit_card', label: 'Crédito' },
  { value: 'debit_card', label: 'Débito' },
  { value: 'boleto', label: 'Boleto' },
  { value: 'cash', label: 'Dinheiro' },
]

const PAGE_SIZE = 25

const selectCls =
  'rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-700 focus:border-brand-blue focus:outline-none focus:ring-1 focus:ring-brand-blue'

export default function AdminAccountingPage() {
  const periodCtl = useAdminPeriod('month')
  const { period } = periodCtl
  const [params, setParams] = useSearchParams()
  const [exporting, setExporting] = useState(false)

  const kind = params.get('natureza') ?? ''
  const method = params.get('metodo') ?? ''
  const page = Math.max(1, Number(params.get('pagina') ?? '1') || 1)

  const setParam = useCallback(
    (key: string, value: string) => {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          if (value) sp.set(key, value)
          else sp.delete(key)
          if (key !== 'pagina') sp.delete('pagina')
          return sp
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const summary = useAdminAccounting(period)
  const recon = useAdminReconciliation(period)
  const ledger = useAdminLedger({
    ...period,
    kind: kind || undefined,
    method: method || undefined,
    page,
    pageSize: PAGE_SIZE,
  })

  /**
   * Se `net ≠ bruto − taxas − estornos − chargebacks`, o resumo do backend está
   * errado. Um painel contábil que exibe número que não fecha é pior que
   * nenhum, então isso vira um aviso vermelho no topo em vez de sumir.
   */
  const imbalance = summary.data ? accountingImbalanceCents(summary.data) : 0

  const [exportError, setExportError] = useState<string | null>(null)

  /**
   * Com backend, o arquivo vem do servidor em streaming (pode ter centenas de
   * milhares de linhas e não deve ser materializado duas vezes na aba). Sem
   * backend, o CSV é montado no cliente a partir do livro carregado — com o
   * MESMO cabeçalho que o servidor emite, para o contador não ver diferença.
   */
  const handleExport = useCallback(async () => {
    setExporting(true)
    setExportError(null)
    try {
      const remote = await fetchAccountingExport(period, 'csv')
      if (remote) {
        downloadBlob(remote.filename, remote.blob)
        return
      }
      const { fetchLedger } = await import('@/lib/adminApi')
      const all = await fetchLedger({ ...period, page: 1, pageSize: 100_000 })
      downloadCsv(ledgerFilename(period.from, period.to), ledgerToCsv(all.items))
    } catch (e) {
      setExportError(e instanceof Error ? e.message : 'Falha ao gerar o arquivo')
    } finally {
      setExporting(false)
    }
  }, [period])

  const totalPages = ledger.data ? Math.max(1, Math.ceil(ledger.data.total / PAGE_SIZE)) : 1

  const netSeries = useMemo(
    () => (summary.data?.series ?? []).map((p) => p.valueCents),
    [summary.data],
  )

  return (
    <AdminShell
      title="Auditoria contábil"
      description="Receita, taxas, estornos e o livro de lançamentos — com rastro até o pedido de origem."
      toolbar={
        <div className="flex flex-wrap items-center gap-2">
          <PeriodPicker {...periodCtl} />
          <button
            type="button"
            onClick={() => void handleExport()}
            disabled={exporting}
            className="inline-flex items-center gap-1.5 rounded-md bg-brand-blue px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-blue-dark disabled:opacity-60"
          >
            <Download className="h-3.5 w-3.5" aria-hidden="true" />
            {exporting ? 'Gerando…' : 'Exportar p/ contador'}
          </button>
          {exportError && (
            <span role="alert" className="text-xs font-medium text-red-700">
              {exportError}
            </span>
          )}
        </div>
      }
    >
      {summary.isError && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <ErrorState
            message={(summary.error as Error)?.message ?? 'Erro desconhecido'}
            onRetry={() => void summary.refetch()}
          />
        </div>
      )}

      {summary.isLoading && !summary.data && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <LoadingRows rows={6} />
        </div>
      )}

      {summary.data && (
        <div className="space-y-4">
          {imbalance !== 0 && (
            <div className="flex items-start gap-2 rounded-lg border border-l-4 border-gray-200 border-l-red-600 bg-red-50 p-3">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
              <div>
                <p className="text-sm font-semibold text-red-800">O resumo não fecha</p>
                <p className="mt-0.5 text-xs text-red-700">
                  Líquido informado difere de bruto − taxas − estornos − chargebacks em{' '}
                  <strong className="tabular-nums">{formatCents(imbalance)}</strong>. Não use estes
                  números para fechamento até o payment-service corrigir.
                </p>
              </div>
            </div>
          )}

          {/* Resultado */}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
            <StatTile label="Receita bruta" value={formatCentsCompact(summary.data.grossCents)} series={netSeries} />
            <StatTile
              label="Taxas do PSP"
              value={formatCentsCompact(summary.data.pspFeeCents)}
              hint={`${formatPercent(summary.data.grossCents > 0 ? summary.data.pspFeeCents / summary.data.grossCents : 0, 2)} do bruto`}
            />
            <StatTile
              label="Estornos"
              value={formatCentsCompact(summary.data.refundCents)}
              hint={`${formatPercent(summary.data.grossCents > 0 ? summary.data.refundCents / summary.data.grossCents : 0, 2)} do bruto`}
            />
            <StatTile
              label="Chargebacks"
              value={formatCentsCompact(summary.data.chargebackCents)}
              severity={
                summary.data.grossCents > 0 && summary.data.chargebackCents / summary.data.grossCents > 0.01
                  ? 'warn'
                  : undefined
              }
              hint="Limite do adquirente: 1% do bruto"
            />
            <StatTile
              label="Receita líquida"
              value={formatCentsCompact(summary.data.netCents)}
              className="col-span-2 lg:col-span-1 ring-1 ring-brand-blue/20"
              seriesColor={CHART.primary}
              series={netSeries}
            />
          </div>

          {/* Por método */}
          <Section
            title="Por método de pagamento"
            description="Onde a taxa come a margem. Pix é o mais barato; crédito parcelado é o mais caro."
          >
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Método</Th>
                    <Th numeric>Transações</Th>
                    <Th numeric>Bruto</Th>
                    <Th numeric>Taxa PSP</Th>
                    <Th numeric>Taxa efetiva</Th>
                    <Th numeric>Estornos</Th>
                    <Th numeric>Chargebacks</Th>
                    <Th numeric>Líquido</Th>
                  </tr>
                </thead>
                <tbody>
                  {summary.data.byMethod.map((m) => (
                    <tr key={m.method} className="hover:bg-gray-50">
                      <Td className="whitespace-nowrap font-medium text-gray-900">
                        {methodLabel(m.method)}
                      </Td>
                      <Td numeric>{formatCount(m.transactions)}</Td>
                      <Td numeric><Money cents={m.grossCents} /></Td>
                      <Td numeric><Money cents={m.pspFeeCents} /></Td>
                      <Td numeric className="text-gray-600">{formatPercent(m.effectiveFeeRate, 2)}</Td>
                      <Td numeric><Money cents={m.refundCents} /></Td>
                      <Td numeric>
                        {/* `null` = a rota não recorta chargeback por método.
                            "—" diz "não sabemos"; um zero afirmaria "não houve". */}
                        {m.chargebackCents === null ? (
                          <span className="text-gray-300" title="Não recortado por método nesta fonte">
                            —
                          </span>
                        ) : (
                          <Money cents={m.chargebackCents} />
                        )}
                      </Td>
                      <Td numeric><Money cents={m.netCents} emphasis /></Td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr className="bg-gray-50 font-semibold">
                    <Td className="text-gray-900">Total</Td>
                    <Td numeric>
                      {formatCount(summary.data.byMethod.reduce((a, m) => a + m.transactions, 0))}
                    </Td>
                    <Td numeric><Money cents={summary.data.grossCents} emphasis /></Td>
                    <Td numeric><Money cents={summary.data.pspFeeCents} emphasis /></Td>
                    <Td numeric className="text-gray-600">
                      {formatPercent(
                        summary.data.grossCents > 0 ? summary.data.pspFeeCents / summary.data.grossCents : 0,
                        2,
                      )}
                    </Td>
                    <Td numeric><Money cents={summary.data.refundCents} emphasis /></Td>
                    <Td numeric><Money cents={summary.data.chargebackCents} emphasis /></Td>
                    <Td numeric><Money cents={summary.data.netCents} emphasis /></Td>
                  </tr>
                </tfoot>
              </Table>
            </ScrollArea>
          </Section>

          {/* Reconciliação */}
          <Section
            title="Divergências de reconciliação com o PSP"
            description={
              recon.data
                ? `${formatCount(recon.data.matchedCount)} transações batidas · última execução ${formatDateTime(recon.data.lastRunAt)}`
                : 'Comparação entre o nosso livro e o extrato do Appmax.'
            }
            actions={
              recon.data && recon.data.divergentCount > 0 ? (
                <span className="inline-flex items-center gap-2 text-xs">
                  <span className="text-gray-500">Diferença acumulada</span>
                  <strong className="tabular-nums text-red-700">
                    {formatCents(recon.data.totalDeltaCents)}
                  </strong>
                </span>
              ) : undefined
            }
          >
            {recon.isLoading && !recon.data && <LoadingRows rows={3} />}
            {recon.data && recon.data.items.length === 0 && (
              <EmptyState title="Sem divergências" description="O livro bate com o extrato do PSP no período." />
            )}
            {recon.data && recon.data.items.length > 0 && (
              <ScrollArea>
                <Table>
                  <thead>
                    <tr>
                      <Th>Data</Th>
                      <Th>Tipo</Th>
                      <Th numeric>Nosso livro</Th>
                      <Th numeric>Extrato PSP</Th>
                      <Th numeric>Diferença</Th>
                      <Th>Origem</Th>
                      <Th>Observação</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {recon.data.items.map((it) => (
                      <tr key={it.id} className="hover:bg-gray-50">
                        <Td className="whitespace-nowrap tabular-nums text-gray-600">
                          {it.date.split('-').reverse().join('/')}
                        </Td>
                        <Td>
                          <SeverityPill severity={it.severity}>{RECON_TYPE_LABEL[it.type]}</SeverityPill>
                        </Td>
                        <Td numeric><Money cents={it.ledgerCents} /></Td>
                        <Td numeric><Money cents={it.pspCents} /></Td>
                        <Td numeric>
                          <span
                            className={
                              it.deltaCents === 0
                                ? 'tabular-nums text-gray-500'
                                : 'font-semibold tabular-nums text-red-700'
                            }
                          >
                            {formatCents(it.deltaCents)}
                          </span>
                        </Td>
                        <Td className="whitespace-nowrap font-mono text-xs text-gray-600">
                          {it.orderNumber ?? it.pspTransactionId ?? '—'}
                        </Td>
                        <Td className="max-w-[24rem] text-xs text-gray-600">{it.note}</Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </ScrollArea>
            )}
          </Section>

          {/* Livro-razão */}
          <Section
            title="Livro de lançamentos"
            description="Partida dobrada. Cada linha aponta para o pedido que a originou."
            actions={
              <div className="flex flex-wrap items-center gap-2">
                <label className="sr-only" htmlFor="ledger-kind">Natureza</label>
                <select
                  id="ledger-kind"
                  className={selectCls}
                  value={kind}
                  onChange={(e) => setParam('natureza', e.target.value)}
                >
                  {KIND_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
                <label className="sr-only" htmlFor="ledger-method">Método</label>
                <select
                  id="ledger-method"
                  className={selectCls}
                  value={method}
                  onChange={(e) => setParam('metodo', e.target.value)}
                >
                  {METHOD_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              </div>
            }
          >
            {ledger.isLoading && !ledger.data && <LoadingRows rows={8} />}
            {ledger.data && ledger.data.items.length === 0 && (
              <EmptyState title="Nenhum lançamento" description="Nenhum lançamento bate com os filtros." />
            )}
            {ledger.data && ledger.data.items.length > 0 && (
              <>
                <ScrollArea>
                  <Table>
                    <thead>
                      <tr>
                        <Th>Data</Th>
                        <Th>Conta</Th>
                        <Th>Natureza</Th>
                        <Th>Método</Th>
                        <Th numeric>Débito</Th>
                        <Th numeric>Crédito</Th>
                        <Th>Pedido</Th>
                        <Th>Histórico</Th>
                      </tr>
                    </thead>
                    <tbody>
                      {ledger.data.items.map((e: LedgerEntry) => (
                        <tr key={e.id} className="hover:bg-gray-50">
                          <Td className="whitespace-nowrap tabular-nums text-xs text-gray-600">
                            {formatDateTime(e.occurredAt)}
                          </Td>
                          <Td className="whitespace-nowrap">
                            <span className="font-mono text-xs text-gray-500">{e.accountCode}</span>{' '}
                            <span className="text-gray-800">{e.account}</span>
                          </Td>
                          <Td><Chip>{ledgerKindLabel(e.kind)}</Chip></Td>
                          <Td className="whitespace-nowrap text-gray-600">{methodLabel(e.method)}</Td>
                          <Td numeric>
                            {e.debitCents > 0 ? <Money cents={e.debitCents} emphasis /> : <span className="text-gray-300">—</span>}
                          </Td>
                          <Td numeric>
                            {e.creditCents > 0 ? <Money cents={e.creditCents} emphasis /> : <span className="text-gray-300">—</span>}
                          </Td>
                          <Td className="whitespace-nowrap">
                            {e.orderNumber ? (
                              <a
                                href={`/pedido/${e.orderId}`}
                                className="font-mono text-xs text-brand-blue hover:underline"
                              >
                                {e.orderNumber}
                              </a>
                            ) : (
                              <span className="text-gray-300">—</span>
                            )}
                          </Td>
                          <Td className="max-w-[22rem] truncate text-xs text-gray-600">{e.memo}</Td>
                        </tr>
                      ))}
                    </tbody>
                    <tfoot>
                      <tr className="bg-gray-50 font-semibold">
                        <Td colSpan={4} className="text-xs text-gray-600">
                          Totais do filtro ({formatCount(ledger.data.total)} lançamentos)
                        </Td>
                        <Td numeric><Money cents={ledger.data.totalDebitCents} emphasis /></Td>
                        <Td numeric><Money cents={ledger.data.totalCreditCents} emphasis /></Td>
                        <Td colSpan={2}>
                          {ledger.data.totalDebitCents === ledger.data.totalCreditCents ? (
                            <SeverityPill severity="ok">Partida dobrada fecha</SeverityPill>
                          ) : (
                            <SeverityPill severity="critical">
                              Diferença de {formatCents(ledger.data.totalDebitCents - ledger.data.totalCreditCents)}
                            </SeverityPill>
                          )}
                        </Td>
                      </tr>
                    </tfoot>
                  </Table>
                </ScrollArea>

                <div className="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 px-3 py-2 text-xs">
                  <span className="text-gray-500">
                    Página {page} de {totalPages} · {formatCount(ledger.data.total)} lançamentos
                  </span>
                  <div className="flex gap-1.5">
                    <button
                      type="button"
                      onClick={() => setParam('pagina', String(page - 1))}
                      disabled={page <= 1}
                      className="rounded border border-gray-300 px-2.5 py-1 font-semibold text-gray-700 disabled:opacity-40 enabled:hover:bg-gray-50"
                    >
                      Anterior
                    </button>
                    <button
                      type="button"
                      onClick={() => setParam('pagina', String(page + 1))}
                      disabled={page >= totalPages}
                      className="rounded border border-gray-300 px-2.5 py-1 font-semibold text-gray-700 disabled:opacity-40 enabled:hover:bg-gray-50"
                    >
                      Próxima
                    </button>
                  </div>
                </div>
              </>
            )}
          </Section>

          <p className="flex items-center gap-2 px-1 text-[11px] text-gray-500">
            <Sparkline values={netSeries.slice(-20)} width={60} height={16} filled={false} showLast={false} />
            Receita líquida diária no período. Valores em centavos no contrato da API; nenhum dado
            desta tela é gravado no navegador.
          </p>
        </div>
      )}
    </AdminShell>
  )
}
