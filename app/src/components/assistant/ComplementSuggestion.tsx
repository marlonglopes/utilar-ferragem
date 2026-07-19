import { useState } from 'react'
import { Plus, Check, Wrench, TrendingUp } from 'lucide-react'
import { formatCurrency } from '@/lib/format'
import { useCartStore } from '@/store/cartStore'
import type { LaraComplemento } from '@/lib/alice'

/**
 * Sugestão de complemento, com 1 toque para adicionar ao carrinho.
 *
 * A sugestão SÓ existe com o motivo junto. O backend sempre manda o "por que"
 * (exigência técnica do serviço, ou padrão agregado de co-compra), e sem ele a
 * sugestão não é renderizada: oferta sem justificativa é empurrar produto, e o
 * cliente não aceita nem o vendedor repassa com confiança.
 */
export function ComplementSuggestion({ complemento }: { complemento: LaraComplemento }) {
  const addItem = useCartStore((s) => s.addItem)
  const [adicionado, setAdicionado] = useState(false)
  const { produto, motivo, origem } = complemento

  // Regra dura: sem motivo, não renderiza.
  if (!motivo || motivo.trim() === '') return null

  const tecnica = origem !== 'co-compra'
  const Icon = tecnica ? Wrench : TrendingUp

  function handleAdd() {
    addItem({
      productId: produto.id || produto.slug,
      sellerId: 'utilar',
      sellerName: 'UtiLar Ferragem',
      name: produto.name,
      icon: '📦',
      priceSnapshot: produto.price,
      quantity: 1,
      stock: produto.stock,
    })
    setAdicionado(true)
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-2.5 text-[13px]">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="line-clamp-2 font-medium text-gray-900">{produto.name}</p>
          <p className="mt-0.5 font-semibold text-brand-blue">{formatCurrency(produto.price)}</p>
        </div>
        <button
          type="button"
          onClick={handleAdd}
          disabled={adicionado}
          aria-label={`Adicionar ${produto.name} ao carrinho`}
          className="flex shrink-0 items-center gap-1 rounded-full bg-brand-orange px-3 py-1.5 text-[12px] font-bold text-white transition-colors hover:bg-brand-orange-dark focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange focus-visible:ring-offset-2 disabled:bg-green-600"
        >
          {adicionado ? (
            <>
              <Check className="h-3.5 w-3.5" aria-hidden="true" />
              No carrinho
            </>
          ) : (
            <>
              <Plus className="h-3.5 w-3.5" aria-hidden="true" />
              Adicionar
            </>
          )}
        </button>
      </div>

      <p className="mt-1.5 flex items-start gap-1.5 rounded bg-gray-50 px-2 py-1.5 text-[12px] leading-snug text-gray-700">
        <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-brand-gold" aria-hidden="true" />
        <span>
          <span className="font-semibold text-gray-900">
            {tecnica ? 'Por que: ' : 'Costuma sair junto: '}
          </span>
          {motivo}
        </span>
      </p>
    </div>
  )
}

/** Lista de complementos. Descarta silenciosamente os que vierem sem motivo. */
export function ComplementSuggestions({ complementos }: { complementos?: LaraComplemento[] }) {
  const validos = (complementos ?? []).filter((c) => c.motivo && c.motivo.trim() !== '')
  if (validos.length === 0) return null

  return (
    <section aria-label="Sugestões para completar" className="space-y-1.5">
      <h3 className="font-display text-[12px] font-bold uppercase tracking-wide text-gray-500">
        Para completar
      </h3>
      {validos.map((c) => (
        <ComplementSuggestion key={c.produto.slug} complemento={c} />
      ))}
    </section>
  )
}
