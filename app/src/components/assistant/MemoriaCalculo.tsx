import { ChevronRight } from 'lucide-react'
import { formatNumber } from '@/lib/format'
import type { MaterialItem } from '@/lib/alice'

function faixa(item: MaterialItem): string | null {
  if (!item.coefUnid) return null
  if (item.coefMin === 0 && item.coefMax === 0) return null
  const min = formatNumber(item.coefMin, undefined, { maximumFractionDigits: 4 })
  const max = formatNumber(item.coefMax, undefined, { maximumFractionDigits: 4 })
  const valor = item.coefMin === item.coefMax ? min : `${min}–${max}`
  return `${valor} ${item.coefUnid}`
}

/**
 * Memória de cálculo recolhível.
 *
 * Fechada por padrão (a lista de materiais é o resultado; a memória é a prova),
 * mas com affordance clara de que abre. É o que separa um número conferível de
 * um palpite — e é o que faz o cliente confiar na quantidade antes de comprar.
 *
 * Usa <details>/<summary>: acessível por teclado e por leitor de tela sem JS.
 */
export function MemoriaCalculo({ item }: { item: MaterialItem }) {
  const f = faixa(item)

  return (
    <details className="group rounded-md border border-gray-200 bg-gray-50/70 text-[12px] open:bg-white">
      <summary className="flex cursor-pointer list-none items-center gap-1 rounded-md px-2 py-1.5 font-medium text-brand-blue hover:bg-brand-blue/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange">
        <ChevronRight
          className="h-3.5 w-3.5 shrink-0 transition-transform group-open:rotate-90"
          aria-hidden="true"
        />
        Como cheguei nesse número
      </summary>

      <div className="space-y-1.5 px-3 pb-2.5 pt-1 text-gray-700">
        <p className="leading-snug">{item.memoria}</p>

        {f && (
          <p className="leading-snug">
            <span className="font-semibold text-gray-900">Faixa do coeficiente:</span> {f}
          </p>
        )}

        {item.fonte && (
          <p className="leading-snug">
            <span className="font-semibold text-gray-900">Fonte:</span> {item.fonte}
          </p>
        )}

        {item.observacao && (
          <p className="leading-snug text-gray-600">
            <span className="font-semibold text-gray-900">Observação:</span> {item.observacao}
          </p>
        )}
      </div>
    </details>
  )
}
