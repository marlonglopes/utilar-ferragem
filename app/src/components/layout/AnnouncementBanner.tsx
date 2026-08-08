import { Info, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { useStoreSettings } from '@/hooks/useStoreSettings'
import type { AnnouncementLevel } from '@/lib/storeSettingsApi'
import { cn } from '@/lib/cn'

// Estilo por tom. Cores próprias (não a laranja da marca) porque o aviso é
// SINAL de estado (promoção/alerta), não identidade — e precisa contrastar com o
// resto do chrome pra ser lido.
const LEVELS: Record<AnnouncementLevel, { cls: string; Icon: typeof Info }> = {
  info: { cls: 'bg-brand-blue text-white', Icon: Info },
  warning: { cls: 'bg-amber-500 text-amber-950', Icon: AlertTriangle },
  success: { cls: 'bg-green-600 text-white', Icon: CheckCircle2 },
}

/**
 * Faixa de aviso da loja, no topo da vitrine. O dono liga/edita em
 * /admin/loja (config da loja). Some por completo quando desligada — nada de
 * espaço reservado nem barra em branco.
 */
export function AnnouncementBanner() {
  const { data } = useStoreSettings()
  const a = data?.announcement
  if (!a?.enabled || a.message.trim() === '') return null

  const { cls, Icon } = LEVELS[a.level] ?? LEVELS.info
  return (
    <div className={cn('px-4 py-2 text-sm font-medium', cls)} role="status">
      <div className="container flex items-center justify-center gap-2 text-center">
        <Icon className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
        <span>{a.message}</span>
      </div>
    </div>
  )
}
