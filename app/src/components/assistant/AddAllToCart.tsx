import { useState } from 'react'
import { ShoppingCart, Check } from 'lucide-react'
import { useCartStore } from '@/store/cartStore'
import type { MaterialItem } from '@/lib/alice'
import { itensComProduto } from './helpers'

export interface ResumoAdicao {
  adicionados: string[]
  pulados: string[]
}

/**
 * "Adicionar tudo ao carrinho".
 *
 * Adiciona cada item que tem produto casado, na quantidade `embalagens` (que já
 * é o arredondamento para cima feito pelo backend — comprar 3,7 sacos não
 * existe). Itens sem produto casado são PULADOS e reportados: a Alice não
 * inventa um produto para fechar a lista, e o cliente precisa saber o que ficou
 * de fora para comprar por outro caminho.
 */
export function AddAllToCart({
  itens,
  onAdicionado,
}: {
  itens: MaterialItem[]
  onAdicionado?: (resumo: ResumoAdicao) => void
}) {
  const addItem = useCartStore((s) => s.addItem)
  const [resumo, setResumo] = useState<ResumoAdicao | null>(null)

  const compraveis = itensComProduto(itens)
  const pulados = itens.filter((i) => !compraveis.includes(i))

  function handleClick() {
    for (const item of compraveis) {
      const p = item.produto
      if (!p) continue
      addItem({
        productId: p.id ?? p.slug,
        sellerId: p.sellerId ?? 'utilar',
        sellerName: p.sellerName ?? 'UtiLar Ferragem',
        name: p.nome,
        icon: p.icon ?? '📦',
        priceSnapshot: p.preco,
        quantity: item.embalagens,
        stock: p.estoque,
      })
    }
    const r: ResumoAdicao = {
      adicionados: compraveis.map((i) => i.nome),
      pulados: pulados.map((i) => i.nome),
    }
    setResumo(r)
    onAdicionado?.(r)
  }

  if (compraveis.length === 0) {
    return (
      <p className="text-[12px] text-gray-500">
        Nenhum item desta lista tem produto casado no catálogo — não dá para adicionar ao carrinho
        automaticamente.
      </p>
    )
  }

  return (
    <div className="space-y-2">
      <button
        type="button"
        onClick={handleClick}
        className="flex w-full items-center justify-center gap-2 rounded-lg bg-brand-orange px-4 py-2.5 font-display text-sm font-bold text-white transition-colors hover:bg-brand-orange-dark focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange focus-visible:ring-offset-2"
      >
        <ShoppingCart className="h-4 w-4" aria-hidden="true" />
        Adicionar tudo ao carrinho
      </button>

      {resumo && (
        <div role="status" className="space-y-1 rounded-lg bg-green-50 px-3 py-2 text-[12px]">
          <p className="flex items-center gap-1.5 font-semibold text-green-800">
            <Check className="h-3.5 w-3.5" aria-hidden="true" />
            {resumo.adicionados.length}{' '}
            {resumo.adicionados.length === 1 ? 'item adicionado' : 'itens adicionados'} ao carrinho
          </p>
          <p className="text-green-900">{resumo.adicionados.join(', ')}</p>
          {resumo.pulados.length > 0 && (
            <p className="text-amber-800">
              Não adicionei {resumo.pulados.join(', ')} — sem produto correspondente no catálogo.
              Fale com um vendedor para esses.
            </p>
          )}
        </div>
      )}
    </div>
  )
}
