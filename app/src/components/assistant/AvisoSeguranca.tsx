import { AlertTriangle, ShieldAlert } from 'lucide-react'
import { inferirGravidade, type Gravidade } from './helpers'

const ESTILOS: Record<Gravidade, { box: string; icon: string; label: string }> = {
  alta: {
    box: 'border-red-400 bg-red-50 text-red-900',
    icon: 'text-red-600',
    label: 'Risco de segurança',
  },
  media: {
    box: 'border-amber-400 bg-amber-50 text-amber-900',
    icon: 'text-amber-600',
    label: 'Atenção',
  },
}

/**
 * Aviso de segurança — anexado pelo SERVIDOR, fora do alcance do modelo.
 *
 * Precisa ser impossível confundir com texto normal da conversa: borda grossa,
 * fundo próprio, ícone e `role="alert"` para que leitor de tela o anuncie. Se
 * este componente falhar em se destacar, o cliente executa uma obra a partir de
 * um número que a Alice deliberadamente não deu.
 */
export function AvisoSeguranca({ aviso, gravidade }: { aviso: string; gravidade?: Gravidade }) {
  const g = gravidade ?? inferirGravidade(aviso)
  const s = ESTILOS[g]
  const Icon = g === 'alta' ? ShieldAlert : AlertTriangle

  return (
    <div
      role="alert"
      data-gravidade={g}
      className={`flex gap-2 rounded-lg border-l-4 px-3 py-2.5 text-[13px] leading-snug shadow-sm ${s.box}`}
    >
      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${s.icon}`} aria-hidden="true" />
      <div>
        <div className="font-display text-[11px] font-bold uppercase tracking-wide">{s.label}</div>
        <p className="mt-0.5">{aviso}</p>
      </div>
    </div>
  )
}

/** Lista de avisos. Nada renderiza quando não há avisos. */
export function AvisosSeguranca({ avisos }: { avisos?: string[] }) {
  if (!avisos || avisos.length === 0) return null
  return (
    <div className="space-y-2">
      {avisos.map((a, i) => (
        <AvisoSeguranca key={i} aviso={a} />
      ))}
    </div>
  )
}
