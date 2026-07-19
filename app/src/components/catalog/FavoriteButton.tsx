import { useTranslation } from 'react-i18next'
import { Heart } from 'lucide-react'
import { useFavoritesStore } from '@/store/favoritesStore'
import { useOptionalToast } from '@/components/ui/useToast'
import { cn } from '@/lib/cn'
import type { Product } from '@/types/product'

export interface FavoriteButtonProps {
  product: Product
  /** `card` flutua sobre a foto na vitrine; `detail` fica ao lado do CTA. */
  variant?: 'card' | 'detail'
  className?: string
}

export function FavoriteButton({ product, variant = 'card', className }: FavoriteButtonProps) {
  const { t } = useTranslation()
  const { toast } = useOptionalToast()
  // Assina o item específico: sem isso, favoritar um produto re-renderiza os
  // 40 cards da vitrine.
  const isFav = useFavoritesStore((s) => s.items.some((i) => i.productId === product.id))
  const toggle = useFavoritesStore((s) => s.toggle)

  function handleClick(e: React.MouseEvent) {
    // O card inteiro é um <Link>. Sem isto, favoritar navegaria para o produto
    // e o cliente perderia a vitrine que estava percorrendo.
    e.preventDefault()
    e.stopPropagation()
    const nowFavorite = toggle(product)
    toast(
      nowFavorite ? t('favorites.added') : t('favorites.removed'),
      nowFavorite ? 'success' : 'info'
    )
  }

  const label = isFav ? t('favorites.remove') : t('favorites.add')

  return (
    <button
      type="button"
      onClick={handleClick}
      /*
        aria-pressed comunica ESTADO (salvo / não salvo). Um `aria-label` que
        muda de texto sozinho faz o leitor de tela anunciar dois botões
        diferentes; com aria-pressed é o mesmo botão, ligado ou desligado.
        title dá o mesmo texto no hover para quem usa mouse.
      */
      aria-pressed={isFav}
      aria-label={label}
      title={label}
      className={cn(
        'flex items-center justify-center rounded-full transition-colors',
        // 40px de alvo: o mínimo confortável pra polegar em celular, que é
        // onde a maioria das compras acontece.
        'min-h-[40px] min-w-[40px]',
        'focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange focus-visible:ring-offset-1',
        variant === 'card' &&
          'absolute top-1.5 right-1.5 z-10 bg-white/90 backdrop-blur-sm shadow-sm hover:bg-white',
        variant === 'detail' && 'border border-gray-300 hover:bg-gray-50 h-12 w-12',
        className
      )}
    >
      <Heart
        className={cn(
          'h-5 w-5 transition-colors',
          isFav ? 'fill-red-500 text-red-500' : 'text-gray-400'
        )}
        aria-hidden
      />
    </button>
  )
}
