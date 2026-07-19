import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { AdminShell } from '@/components/admin/AdminShell'
import { AlertList } from '@/components/admin/AlertList'
import { PeriodPicker } from '@/components/admin/PeriodPicker'
import { BarSeries, StackedBar } from '@/components/admin/charts'
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
import { CHART } from '@/components/admin/tokens'
import { useAdminPeriod } from '@/hooks/useAdminPeriod'
import { useAdminOverview } from '@/hooks/useAdminQueries'
import {
  conversionRate,
  formatCents,
  formatCentsCompact,
  formatCount,
  formatPercent,
  pctChange,
} from '@/lib/adminFormat'
import type { OrderStatus, Severity } from '@/lib/adminTypes'

/**
 * Visão geral — "os números que importam num dia".
 *
 * Ordem deliberada: resumo antes do detalhe. Alerta crítico primeiro (é o que
 * exige ação agora), depois os KPIs de venda, depois a série, e só então as
 * tabelas de composição. Quem abre no celular às 8h da manhã vê o essencial
 * sem rolar.
 */

const STATUS_LABEL: Record<OrderStatus, string> = {
  pending: 'Aguardando pagamento',
  paid: 'Pago',
  picking: 'Em separação',
  shipped: 'Enviado',
  delivered: 'Entregue',
  canceled: 'Cancelado',
  refunded: 'Estornado',
}

/**
 * Cores da composição por status: uma rampa de UM tom (azul da marca,
 * claro→escuro) porque status de pedido é ordinal — o pedido caminha de
 * "aguardando" até "entregue". Cancelado e estornado saem da rampa e usam
 * cinza/vermelho, porque são saídas do fluxo, não estágios dele.
 */
const STATUS_COLOR: Record<OrderStatus, string> = {
  pending: '#9EC5F4',
  paid: '#6DA7EC',
  picking: '#3987E5',
  shipped: '#256ABF',
  delivered: '#1B3E8A',
  canceled: '#9CA3AF',
  refunded: '#D03B3B',
}

/** Conversão de pagamento: abaixo de 80% é problema de checkout, não de mercado. */
function conversionSeverity(rate: number | null): Severity | undefined {
  if (rate === null) return undefined
  if (rate < 0.8) return 'critical'
  if (rate < 0.88) return 'warn'
  return 'ok'
}

export default function AdminOverviewPage() {
  const periodCtl = useAdminPeriod('month')
  const { period } = periodCtl
  const { data, isLoading, isError, error, refetch } = useAdminOverview(period)

  const conv = useMemo(() => (data ? conversionRate(data.funnel) : null), [data])

  const barPoints = useMemo(
    () =>
      (data?.series ?? []).map((p) => ({
        label: p.date.slice(5).split('-').reverse().join('/'),
        value: p.valueCents,
        tooltip: `${p.date.slice(5).split('-').reverse().join('/')}: ${formatCents(p.valueCents)} · ${p.orders} pedidos`,
      })),
    [data],
  )

  const criticalAlerts = (data?.alerts ?? []).filter((a) => a.severity === 'critical')

  return (
    <AdminShell
      title="Visão geral"
      description="Como está a venda agora e o que precisa de atenção."
      toolbar={<PeriodPicker {...periodCtl} />}
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
          {/* Alertas críticos no topo — só os críticos. A lista completa vive em
              Observabilidade; aqui é interrupção, e interrupção demais vira ruído. */}
          {criticalAlerts.length > 0 && (
            <Section
              title={`${criticalAlerts.length} alerta${criticalAlerts.length > 1 ? 's' : ''} crítico${criticalAlerts.length > 1 ? 's' : ''}`}
              description="Exige ação agora."
              actions={
                <Link
                  to="/admin/observabilidade"
                  className="text-xs font-semibold text-brand-blue hover:underline"
                >
                  Ver todos
                </Link>
              }
            >
              <AlertList alerts={criticalAlerts} compact />
            </Section>
          )}

          {/* KPIs */}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
            <StatTile
              label="Vendas hoje"
              value={formatCentsCompact(data.kpis.todayCents)}
              delta={pctChange(data.kpis.todayCents, data.kpis.todayPrevCents)}
              deltaLabel="vs. ontem"
              series={data.series.slice(-14).map((p) => p.valueCents)}
            />
            <StatTile
              label="Últimos 7 dias"
              value={formatCentsCompact(data.kpis.weekCents)}
              delta={pctChange(data.kpis.weekCents, data.kpis.weekPrevCents)}
              deltaLabel="vs. 7 dias antes"
              series={data.series.slice(-7).map((p) => p.valueCents)}
            />
            <StatTile
              label="Período"
              value={formatCentsCompact(data.kpis.monthCents)}
              delta={pctChange(data.kpis.monthCents, data.kpis.monthPrevCents)}
              deltaLabel="vs. período anterior"
              series={data.series.map((p) => p.valueCents)}
            />
            <StatTile
              label="Ticket médio"
              value={formatCents(data.kpis.avgTicketCents)}
              delta={pctChange(data.kpis.avgTicketCents, data.kpis.avgTicketPrevCents)}
            />
            <StatTile
              label="Pedidos"
              value={formatCount(data.kpis.orderCount)}
              delta={pctChange(data.kpis.orderCount, data.kpis.orderCountPrev)}
            />
            <StatTile
              label="Conversão de pagamento"
              value={conv === null ? '—' : formatPercent(conv)}
              hint={`${formatCount(data.funnel.confirmed)} de ${formatCount(data.funnel.created)} confirmados`}
              severity={conversionSeverity(conv)}
            />
          </div>

          {/* Série + composição */}
          <div className="grid gap-4 xl:grid-cols-3">
            <Section
              title="Vendas por dia"
              description={`${period.from.split('-').reverse().join('/')} a ${period.to.split('-').reverse().join('/')}`}
              className="xl:col-span-2"
            >
              <div className="p-3 sm:p-4">
                <BarSeries
                  points={barPoints}
                  height={160}
                  formatMax={formatCentsCompact}
                  label="Vendas líquidas por dia no período"
                />
              </div>
            </Section>

            <Section title="Pedidos por status" description="Composição do período.">
              <div className="p-3 sm:p-4">
                <StackedBar
                  segments={data.byStatus.map((b) => ({
                    key: b.status,
                    label: STATUS_LABEL[b.status],
                    value: b.count,
                    color: STATUS_COLOR[b.status],
                  }))}
                />
                <ul className="mt-3 space-y-1.5">
                  {data.byStatus.map((b) => (
                    <li key={b.status} className="flex items-center gap-2 text-xs">
                      <span
                        className="h-2.5 w-2.5 shrink-0 rounded-sm"
                        style={{ backgroundColor: STATUS_COLOR[b.status] }}
                        aria-hidden="true"
                      />
                      <span className="flex-1 truncate text-gray-700">{STATUS_LABEL[b.status]}</span>
                      <span className="tabular-nums text-gray-500">{formatCount(b.count)}</span>
                      <Money cents={b.valueCents} className="w-24 text-right" />
                    </li>
                  ))}
                </ul>
              </div>
            </Section>
          </div>

          {/* Funil por método */}
          <Section
            title="Conversão por método de pagamento"
            description="Criado → confirmado. Boleto sempre converte menos; crédito caindo é sinal de antifraude apertado."
          >
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Método</Th>
                    <Th numeric>Criados</Th>
                    <Th numeric>Confirmados</Th>
                    <Th numeric>Conversão</Th>
                    <Th className="w-40">&nbsp;</Th>
                  </tr>
                </thead>
                <tbody>
                  {data.funnel.byMethod.map((m) => {
                    const rate = conversionRate(m)
                    return (
                      <tr key={m.method} className="hover:bg-gray-50">
                        <Td className="font-medium text-gray-900">{m.method}</Td>
                        <Td numeric>{formatCount(m.created)}</Td>
                        <Td numeric>{formatCount(m.confirmed)}</Td>
                        <Td numeric className="font-semibold">
                          {rate === null ? '—' : formatPercent(rate)}
                        </Td>
                        <Td>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-gray-100">
                            <div
                              className="h-full rounded-full"
                              style={{
                                width: `${(rate ?? 0) * 100}%`,
                                backgroundColor: CHART.primary,
                              }}
                            />
                          </div>
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </Table>
            </ScrollArea>
          </Section>

          {/* Pedidos travados */}
          <Section
            title="Pedidos travados"
            description="Pagos e sem separação. Se esta lista cresce, veja a fila do outbox em Observabilidade."
            actions={
              <Link
                to="/admin/observabilidade"
                className="text-xs font-semibold text-brand-blue hover:underline"
              >
                Ver outbox
              </Link>
            }
          >
            {data.stuckOrders.length === 0 ? (
              <EmptyState title="Nenhum pedido travado" description="Todo pagamento confirmado virou separação." />
            ) : (
              <ScrollArea>
                <Table>
                  <thead>
                    <tr>
                      <Th>Pedido</Th>
                      <Th>Cliente</Th>
                      <Th numeric>Valor</Th>
                      <Th numeric>Parado há</Th>
                      <Th>Situação</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.stuckOrders.map((o) => {
                      const sev: Severity =
                        o.stuckForHours >= 24 ? 'critical' : o.stuckForHours >= 8 ? 'warn' : 'ok'
                      return (
                        <tr key={o.orderId} className="hover:bg-gray-50">
                          <Td className="whitespace-nowrap font-mono text-xs text-gray-900">
                            {o.orderNumber}
                          </Td>
                          <Td className="max-w-[16rem] truncate text-gray-700">{o.customerName}</Td>
                          <Td numeric>
                            <Money cents={o.totalCents} emphasis />
                          </Td>
                          <Td numeric className="whitespace-nowrap">{o.stuckForHours}h</Td>
                          <Td>
                            <SeverityPill severity={sev}>
                              {sev === 'critical' ? 'Mais de 1 dia' : sev === 'warn' ? 'Atrasado' : 'Recente'}
                            </SeverityPill>
                          </Td>
                        </tr>
                      )
                    })}
                  </tbody>
                </Table>
              </ScrollArea>
            )}
          </Section>
        </div>
      )}
    </AdminShell>
  )
}
