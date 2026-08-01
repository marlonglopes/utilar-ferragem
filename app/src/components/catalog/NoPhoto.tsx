import { ImageOff } from 'lucide-react'
import { cn } from '@/lib/cn'

/**
 * Placeholder "sem foto ainda" — INTENCIONAL, não quebrado.
 *
 * Muitos produtos reais importados do ERP ainda não têm foto de verdade. A loja
 * é física e tem reputação: foto genérica ERRADA (cabo de rede com imagem de
 * fio de roçadeira) é pior que foto nenhuma. Então listamos o produto real (nome,
 * preço, estoque — tudo verdadeiro) e marcamos claramente que a foto está por vir.
 */
export function NoPhoto({ className, compact }: { className?: string; compact?: boolean }) {
  return (
    <div
      className={cn(
        'flex h-full w-full select-none flex-col items-center justify-center gap-1.5 bg-gray-50 text-gray-300',
        className
      )}
    >
      <ImageOff className={compact ? 'h-7 w-7' : 'h-10 w-10'} aria-hidden="true" />
      {/* gray-600 (não 400): "sem foto" ainda é texto que o cliente lê — tem que
          passar AA sobre o cinza claro (gray-400 dava ~2,5:1). O ícone acima é
          decorativo (aria-hidden), então pode ficar claro. */}
      <span className="text-[11px] font-semibold uppercase tracking-wide text-gray-600">
        Sem foto ainda
      </span>
    </div>
  )
}
