import { Check } from 'lucide-react'
import { cn } from '@/lib/cn'
import { IMPORT_STEPS, type ImportStep } from '@/hooks/useImportWizard'

/**
 * Indicador dos quatro passos.
 *
 * Existe para tornar visível que o pipeline é em DOIS TEMPOS — conferir e só
 * então aprovar. Um formulário de upload único ensinaria que importar é um
 * clique, que é precisamente a expectativa errada: entre o dry-run e o commit
 * há uma decisão humana, e a barra é o primeiro lugar onde isso aparece.
 *
 * Passo já percorrido é clicável (o operador volta para corrigir o mapeamento);
 * passo à frente, não — não se pula a revisão.
 */
export function ImportStepper({
  current,
  reachable,
  onNavigate,
}: {
  current: ImportStep
  reachable: ImportStep[]
  onNavigate: (step: ImportStep) => void
}) {
  const currentIndex = IMPORT_STEPS.findIndex((s) => s.id === current)

  return (
    <nav aria-label="Etapas da importação">
      <ol className="flex flex-wrap items-center gap-x-1 gap-y-2 text-xs sm:gap-x-2">
        {IMPORT_STEPS.map((s, i) => {
          const done = i < currentIndex
          const active = s.id === current
          const canGo = reachable.includes(s.id) && !active
          return (
            <li key={s.id} className="flex items-center gap-1 sm:gap-2">
              <button
                type="button"
                onClick={() => canGo && onNavigate(s.id)}
                disabled={!canGo}
                aria-current={active ? 'step' : undefined}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-2 py-1 font-semibold transition-colors',
                  active && 'bg-brand-blue-light text-brand-blue',
                  !active && done && 'text-gray-700 hover:bg-gray-100',
                  !active && !done && 'text-gray-400',
                  !canGo && 'cursor-default',
                )}
              >
                <span
                  className={cn(
                    'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold',
                    active && 'bg-brand-blue text-white',
                    !active && done && 'bg-emerald-600 text-white',
                    !active && !done && 'bg-gray-200 text-gray-500',
                  )}
                >
                  {done ? <Check className="h-3 w-3" aria-hidden="true" /> : i + 1}
                </span>
                <span className={cn(i !== currentIndex && 'hidden sm:inline')}>{s.label}</span>
              </button>
              {i < IMPORT_STEPS.length - 1 && (
                <span className="h-px w-3 bg-gray-300 sm:w-6" aria-hidden="true" />
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

export default ImportStepper
