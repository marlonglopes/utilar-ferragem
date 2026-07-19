import { AdminShell } from '@/components/admin/AdminShell'
import { AlertList } from '@/components/admin/AlertList'
import { Meter, Sparkline } from '@/components/admin/charts'
import {
  ErrorState,
  LoadingRows,
  ScrollArea,
  Section,
  SeverityPill,
  StatTile,
  Table,
  Td,
  Th,
} from '@/components/admin/primitives'
import { CHART, SEVERITY_HEX, SEVERITY_TEXT } from '@/components/admin/tokens'
import { cn } from '@/lib/cn'
import { useAdminObservability } from '@/hooks/useAdminQueries'
import { formatCount, formatDuration, formatLatency, formatPercent } from '@/lib/adminFormat'
import { formatRelativeTime } from '@/lib/format'
import type { Severity } from '@/lib/adminTypes'

/**
 * Observabilidade — saúde dos serviços, latência, erro, fila do outbox.
 *
 * A fila do outbox ganha destaque próprio, acima da tabela de serviços, porque
 * ela é o elo entre "pagamento confirmado" e "pedido pago": se ela cresce, o
 * dinheiro entrou e o pedido não andou — exatamente a lista de pedidos travados
 * da visão geral. Um número que explica outro número merece estar em evidência.
 *
 * Esta tela atualiza sozinha a cada 30s (ver `useAdminObservability`). Saúde de
 * serviço que só muda com F5 não serve para acompanhar incidente.
 */

const SERVICE_LABEL: Record<string, string> = {
  auth: 'auth-service',
  catalog: 'catalog-service',
  order: 'order-service',
  payment: 'payment-service',
}

/** p95 acima de 300ms é o orçamento estourado; acima de 600ms é incidente. */
function latencySeverity(p95: number): Severity {
  if (p95 >= 600) return 'critical'
  if (p95 >= 300) return 'warn'
  return 'ok'
}

/** 1% de 5xx já é visível para o cliente; 2% é incidente aberto. */
function errorSeverity(rate: number): Severity {
  if (rate >= 0.02) return 'critical'
  if (rate >= 0.01) return 'warn'
  return 'ok'
}

export default function AdminObservabilityPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useAdminObservability()

  return (
    <AdminShell
      title="Observabilidade"
      description={
        data
          ? `Coletado ${formatRelativeTime(data.collectedAt)} · atualiza sozinho a cada 30s`
          : 'Saúde dos serviços, latência e fila de eventos.'
      }
      toolbar={
        <button
          type="button"
          onClick={() => void refetch()}
          className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
        >
          {isFetching ? 'Atualizando…' : 'Atualizar agora'}
        </button>
      }
    >
      {isError && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <ErrorState message={(error as Error)?.message ?? 'Erro desconhecido'} onRetry={() => void refetch()} />
        </div>
      )}

      {isLoading && !data && (
        <div className="rounded-lg border border-gray-200 bg-white">
          <LoadingRows rows={6} />
        </div>
      )}

      {data && (
        <div className="space-y-4">
          {/* Outbox em primeiro plano */}
          <Section
            title="Fila do outbox — payment-service"
            description="Se isto cresce, pagamento confirmado não está virando pedido pago."
            actions={<SeverityPill severity={data.outbox.severity} />}
          >
            <div className="grid grid-cols-2 gap-3 p-3 sm:p-4 lg:grid-cols-4">
              <StatTile
                label="Eventos pendentes"
                value={formatCount(data.outbox.pending)}
                severity={data.outbox.severity}
              />
              <StatTile
                label="Evento mais antigo"
                value={formatDuration(data.outbox.oldestAgeSeconds)}
                severity={data.outbox.severity}
                hint="Idade importa mais que tamanho da fila"
              />
              <StatTile
                label="Falhas"
                value={formatCount(data.outbox.failed)}
                severity={data.outbox.failed > 0 ? 'warn' : 'ok'}
                hint="Excederam o número de tentativas"
              />
              <StatTile
                label="Vazão do relay"
                value={`${formatCount(data.outbox.publishedPerMinute)}/min`}
                hint="Eventos publicados no Redpanda"
              />
            </div>
          </Section>

          {/* Serviços */}
          <Section title="Saúde dos serviços" description="Janela de 5 minutos.">
            <ScrollArea>
              <Table>
                <thead>
                  <tr>
                    <Th>Serviço</Th>
                    <Th>Estado</Th>
                    <Th numeric>p50</Th>
                    <Th numeric>p95</Th>
                    <Th numeric>p99</Th>
                    <Th numeric>Erro 5xx</Th>
                    <Th numeric>req/min</Th>
                    <Th numeric>Uptime</Th>
                    <Th>Versão</Th>
                    <Th className="w-32">p95 recente</Th>
                  </tr>
                </thead>
                <tbody>
                  {data.services.map((s) => {
                    const lat = latencySeverity(s.p95Ms)
                    const err = errorSeverity(s.errorRate)
                    return (
                      <tr key={s.name} className="hover:bg-gray-50">
                        <Td className="whitespace-nowrap font-mono text-xs font-medium text-gray-900">
                          {SERVICE_LABEL[s.name] ?? s.name}
                        </Td>
                        <Td>
                          <SeverityPill severity={s.status}>
                            {s.up ? (s.status === 'ok' ? 'Saudável' : 'Degradado') : 'Fora do ar'}
                          </SeverityPill>
                        </Td>
                        <Td numeric className="text-gray-600">{formatLatency(s.p50Ms)}</Td>
                        <Td numeric className={cn('font-semibold', SEVERITY_TEXT[lat])}>
                          {formatLatency(s.p95Ms)}
                        </Td>
                        <Td numeric className="text-gray-600">{formatLatency(s.p99Ms)}</Td>
                        <Td numeric className={cn('font-semibold', SEVERITY_TEXT[err])}>
                          {formatPercent(s.errorRate, 2)}
                        </Td>
                        <Td numeric className="text-gray-600">{formatCount(s.rpm)}</Td>
                        <Td numeric className="text-gray-600">{formatPercent(s.uptimePct, 2)}</Td>
                        <Td className="whitespace-nowrap font-mono text-[11px] text-gray-500">{s.version}</Td>
                        <Td>
                          <Sparkline
                            values={s.latencySeries}
                            width={112}
                            height={26}
                            color={lat === 'ok' ? CHART.primary : SEVERITY_HEX[lat]}
                            label={`Latência p95 recente do ${s.name}`}
                          />
                          <Meter
                            value={Math.min(1, s.p95Ms / 1000)}
                            severity={lat}
                            className="mt-1"
                            label={`p95 do ${s.name} contra o orçamento de 1s`}
                          />
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </Table>
            </ScrollArea>
          </Section>

          <Section
            title={`Alertas ativos (${data.alerts.length})`}
            description="Ordenados por severidade, não por hora — o crítico de ontem importa mais que o aviso de agora."
          >
            <AlertList alerts={data.alerts} />
          </Section>
        </div>
      )}
    </AdminShell>
  )
}
