import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Heart, Trash2, Info } from 'lucide-react'
import { useFavoritesStore } from '@/store/favoritesStore'
import { formatCurrency } from '@/lib/format'
import { Seo } from '@/components/seo/Seo'

function FavoriteRow({
  productId,
  slug,
  name,
  icon,
  imageUrl,
  seller,
  priceSnapshot,
  addedAt,
  onRemove,
}: {
  productId: string
  slug: string
  name: string
  icon: string
  imageUrl?: string
  seller: string
  priceSnapshot: number
  addedAt: string
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  const savedOn = new Date(addedAt).toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  })

  return (
    <li className="relative flex items-center gap-3 rounded-xl border border-gray-200 bg-white p-3 transition-all hover:border-gray-300 hover:shadow-sm sm:gap-4 sm:p-4">
      <div className="flex h-16 w-16 flex-shrink-0 items-center justify-center overflow-hidden rounded-lg bg-gray-50 text-2xl sm:h-20 sm:w-20">
        {imageUrl ? (
          <img
            src={imageUrl}
            alt=""
            loading="lazy"
            decoding="async"
            className="h-full w-full object-contain"
            onError={(e) => {
              e.currentTarget.style.display = 'none'
            }}
          />
        ) : (
          icon
        )}
      </div>

      <div className="min-w-0 flex-1">
        {/*
          Link "esticado": o <a> cobre o card inteiro via ::after, então o
          cliente toca em qualquer lugar e vai pro produto — mas o botão de
          remover continua sendo um alvo próprio (z-10 abaixo). Aninhar um
          <button> dentro de um <a> seria HTML inválido e quebraria o teclado.
        */}
        <Link
          to={`/produto/${slug}`}
          className="text-sm font-medium leading-snug text-gray-900 after:absolute after:inset-0 hover:text-brand-orange sm:text-base"
        >
          {name}
        </Link>
        <p className="mt-0.5 truncate text-xs text-gray-400">{seller}</p>
        <p className="mt-1 text-base font-bold text-gray-900">{formatCurrency(priceSnapshot)}</p>
        <p className="mt-0.5 text-[11px] text-gray-400">
          {t('favorites.savedPrice')} · {t('favorites.savedOn', { date: savedOn })}
        </p>
      </div>

      <button
        type="button"
        onClick={() => onRemove(productId)}
        aria-label={`${t('favorites.remove')}: ${name}`}
        className="relative z-10 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg text-gray-300 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-orange"
      >
        <Trash2 className="h-4 w-4" aria-hidden />
      </button>
    </li>
  )
}

export default function FavoritesPage() {
  const { t } = useTranslation()
  const items = useFavoritesStore((s) => s.items)
  const remove = useFavoritesStore((s) => s.remove)
  const clear = useFavoritesStore((s) => s.clear)
  const [confirmingClear, setConfirmingClear] = useState(false)

  return (
    <>
      {/* Lista pessoal: não deve ser indexada nem aparecer em busca. */}
      <Seo title={t('favorites.title')} path="/favoritos" noIndex />

      <div className="container max-w-3xl py-6">
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 font-display text-2xl font-bold text-gray-900">
              <Heart className="h-5 w-5 text-red-500" aria-hidden />
              {t('favorites.title')}
            </h1>
            {items.length > 0 && (
              <p className="mt-0.5 text-sm text-gray-400">
                {t('favorites.count', { count: items.length })}
              </p>
            )}
          </div>

          {items.length > 0 && (
            <button
              type="button"
              onClick={() => setConfirmingClear(true)}
              className="text-sm font-medium text-gray-400 transition-colors hover:text-red-500"
            >
              {t('favorites.clearAll')}
            </button>
          )}
        </div>

        {items.length === 0 ? (
          <div className="flex flex-col items-center gap-3 py-16 text-center">
            <Heart className="h-12 w-12 text-gray-200" aria-hidden />
            <div>
              <p className="text-sm font-medium text-gray-500">{t('favorites.empty')}</p>
              <p className="mt-1 max-w-xs text-xs text-gray-400">{t('favorites.emptyHint')}</p>
            </div>
            <Link to="/" className="text-sm font-medium text-brand-orange hover:underline">
              {t('favorites.explore')}
            </Link>
          </div>
        ) : (
          <>
            {/*
              Aviso de preço defasado. O card mostra o preço de QUANDO foi
              salvo (ver favoritesStore) — dizer isso é obrigatório, senão o
              cliente monta o orçamento da obra em cima de um número velho.
            */}
            <p className="mb-4 flex items-start gap-2 rounded-xl border border-blue-100 bg-blue-50 px-3 py-2.5 text-xs text-blue-800">
              <Info className="mt-0.5 h-3.5 w-3.5 flex-shrink-0" aria-hidden />
              <span>
                {t('favorites.priceNote')} {t('favorites.localOnly')}
              </span>
            </p>

            <ul className="flex flex-col gap-3">
              {items.map((item) => (
                <FavoriteRow key={item.productId} {...item} onRemove={remove} />
              ))}
            </ul>
          </>
        )}

        {confirmingClear && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
            <div className="flex w-full max-w-sm flex-col gap-4 rounded-2xl bg-white p-6 shadow-xl">
              <p className="text-sm text-gray-700">{t('favorites.clearConfirm')}</p>
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={() => setConfirmingClear(false)}
                  className="h-10 flex-1 rounded-xl border border-gray-300 text-sm font-semibold text-gray-600 transition-colors hover:bg-gray-50"
                >
                  {t('cancel')}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    clear()
                    setConfirmingClear(false)
                  }}
                  className="h-10 flex-1 rounded-xl bg-red-500 text-sm font-semibold text-white transition-colors hover:bg-red-600"
                >
                  {t('favorites.clearAll')}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  )
}
