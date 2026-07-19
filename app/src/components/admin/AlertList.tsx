import { Link } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, XOctagon } from 'lucide-react'
import { cn } from '@/lib/cn'
import { SEVERITY_BAR, SEVERITY_TEXT } from '@/components/admin/tokens'
import { EmptyState } from '@/components/admin/primitives'
import { formatRelativeTime } from '@/lib/format'
import { sortAlerts } from '@/lib/adminFormat'
import type { Alert, Severity } from '@/lib/adminTypes'

const ICONS: Record<Severity, typeof AlertTriangle> = {
  ok: CheckCircle2,
  warn: AlertTriangle,
  critical: XOctagon,
}

/**
 * Lista de alertas ativos.
 *
 * Três canais redundantes de severidade — faixa lateral (forma), ícone
 * (forma) e cor. Cor sozinha não é aceitável num painel cuja função é fazer o
 * problema saltar aos olhos de quem passa perto da tela.
 *
 * Ordenados por severidade e não por hora: o crítico de ontem importa mais que
 * o aviso de agora.
 */
export function AlertList({ alerts, compact = false }: { alerts: Alert[]; compact?: boolean }) {
  const sorted = sortAlerts(alerts)

  if (sorted.length === 0) {
    return <EmptyState title="Nenhum alerta ativo" description="Tudo dentro dos limites." />
  }

  return (
    <ul className="divide-y divide-gray-100">
      {sorted.map((a) => {
        const Icon = ICONS[a.severity]
        const body = (
          <div className={cn('flex gap-2.5 py-2.5 pl-3 pr-3 sm:pl-4', SEVERITY_BAR[a.severity])}>
            <Icon
              className={cn('mt-0.5 h-4 w-4 shrink-0', SEVERITY_TEXT[a.severity])}
              aria-hidden="true"
            />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-gray-900">{a.title}</p>
              {!compact && <p className="mt-0.5 text-xs leading-relaxed text-gray-600">{a.detail}</p>}
              <p className="mt-1 flex flex-wrap items-center gap-x-2 text-[11px] text-gray-500">
                <span className="font-mono">{a.source}</span>
                <span aria-hidden="true">·</span>
                <time dateTime={a.firedAt}>{formatRelativeTime(a.firedAt)}</time>
              </p>
            </div>
          </div>
        )
        return (
          <li key={a.id}>
            {a.href ? (
              <Link to={a.href} className="block hover:bg-gray-50 focus:bg-gray-50 focus:outline-none">
                {body}
              </Link>
            ) : (
              body
            )}
          </li>
        )
      })}
    </ul>
  )
}
