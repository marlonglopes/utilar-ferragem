import { useState, useEffect, useCallback, useRef } from 'react'
import { ChevronLeft, ChevronRight, X, ZoomIn } from 'lucide-react'
import { cn } from '@/lib/cn'
import type { ProductImage } from '@/types/product'

interface ImageGalleryProps {
  images?: ProductImage[]
  icon: string
  productName: string
}

function IconSlide({ icon, size = 'lg' }: { icon: string; size?: 'lg' | 'sm' }) {
  return (
    <div className={cn(
      'flex items-center justify-center bg-gray-50 select-none',
      size === 'lg' ? 'text-[7rem]' : 'text-3xl',
    )}>
      {icon}
    </div>
  )
}

function ImageSlide({ src, alt, size = 'lg' }: { src: string; alt: string; size?: 'lg' | 'sm' }) {
  return (
    <img
      src={src}
      alt={alt}
      className={cn('object-contain', size === 'lg' ? 'w-full h-full' : 'w-full h-full')}
      loading="lazy"
    />
  )
}

export function ImageGallery({ images, icon, productName }: ImageGalleryProps) {
  const hasImages = images && images.length > 0
  const count = hasImages ? images.length : 1
  const [active, setActive] = useState(0)
  const [lightboxOpen, setLightboxOpen] = useState(false)
  const closeRef = useRef<HTMLButtonElement>(null)

  const prev = useCallback(() => setActive((i) => (i - 1 + count) % count), [count])
  const next = useCallback(() => setActive((i) => (i + 1) % count), [count])

  useEffect(() => {
    if (!lightboxOpen) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setLightboxOpen(false)
      if (e.key === 'ArrowLeft') prev()
      if (e.key === 'ArrowRight') next()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [lightboxOpen, prev, next])

  useEffect(() => {
    if (lightboxOpen) closeRef.current?.focus()
  }, [lightboxOpen])

  const touchStart = useRef<number | null>(null)

  function onTouchStart(e: React.TouchEvent) {
    touchStart.current = e.touches[0]?.clientX ?? null
  }
  function onTouchEnd(e: React.TouchEvent) {
    if (touchStart.current == null) return
    const diff = (e.changedTouches[0]?.clientX ?? 0) - touchStart.current
    if (Math.abs(diff) > 40) diff < 0 ? next() : prev()
    touchStart.current = null
  }

  return (
    <>
      <div className="flex flex-col gap-3">
        {/* Main image */}
        <div
          className="relative w-full aspect-square rounded-2xl overflow-hidden bg-gray-50 border border-gray-200 cursor-zoom-in group"
          onTouchStart={onTouchStart}
          onTouchEnd={onTouchEnd}
          onClick={() => setLightboxOpen(true)}
          role="button"
          tabIndex={0}
          aria-label={`Ampliar imagem de ${productName}`}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') setLightboxOpen(true) }}
        >
          {hasImages
            ? <ImageSlide src={images[active].url} alt={images[active].alt} size="lg" />
            : <IconSlide icon={icon} size="lg" />
          }
          <div className="absolute inset-0 bg-black/0 group-hover:bg-black/5 transition-colors flex items-end justify-end p-3">
            <ZoomIn className="h-5 w-5 text-gray-400 opacity-0 group-hover:opacity-100 transition-opacity" />
          </div>

          {count > 1 && (
            <>
              <button
                onClick={(e) => { e.stopPropagation(); prev() }}
                className="absolute left-2 top-1/2 -translate-y-1/2 h-8 w-8 rounded-full bg-white/90 shadow flex items-center justify-center hover:bg-white transition-colors"
                aria-label="Imagem anterior"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); next() }}
                className="absolute right-2 top-1/2 -translate-y-1/2 h-8 w-8 rounded-full bg-white/90 shadow flex items-center justify-center hover:bg-white transition-colors"
                aria-label="Próxima imagem"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </>
          )}
        </div>

        {/* Thumbnail rail */}
        {hasImages && images.length > 1 && (
          <div className="flex gap-2 overflow-x-auto pb-1">
            {images.map((img, i) => (
              <button
                key={i}
                onClick={() => setActive(i)}
                className={cn(
                  'flex-shrink-0 h-16 w-16 rounded-lg overflow-hidden border-2 transition-colors',
                  i === active ? 'border-brand-orange' : 'border-gray-200 hover:border-gray-400',
                )}
                aria-label={`Ver imagem ${i + 1}`}
                aria-pressed={i === active}
              >
                <img src={img.url} alt={img.alt} className="h-full w-full object-cover" />
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Lightbox */}
      {lightboxOpen && (
        <div
          className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center"
          onClick={() => setLightboxOpen(false)}
          onTouchStart={onTouchStart}
          onTouchEnd={onTouchEnd}
        >
          <button
            ref={closeRef}
            className="absolute top-4 right-4 h-10 w-10 rounded-full bg-white/10 hover:bg-white/20 flex items-center justify-center text-white transition-colors"
            onClick={() => setLightboxOpen(false)}
            aria-label="Fechar"
          >
            <X className="h-5 w-5" />
          </button>

          {count > 1 && (
            <>
              <button
                className="absolute left-4 top-1/2 -translate-y-1/2 h-10 w-10 rounded-full bg-white/10 hover:bg-white/20 flex items-center justify-center text-white transition-colors"
                onClick={(e) => { e.stopPropagation(); prev() }}
                aria-label="Imagem anterior"
              >
                <ChevronLeft className="h-6 w-6" />
              </button>
              <button
                className="absolute right-4 top-1/2 -translate-y-1/2 h-10 w-10 rounded-full bg-white/10 hover:bg-white/20 flex items-center justify-center text-white transition-colors"
                onClick={(e) => { e.stopPropagation(); next() }}
                aria-label="Próxima imagem"
              >
                <ChevronRight className="h-6 w-6" />
              </button>
            </>
          )}

          <div
            className="max-w-3xl max-h-[85vh] w-full mx-8 flex items-center justify-center"
            onClick={(e) => e.stopPropagation()}
          >
            {hasImages
              ? <img src={images[active].url} alt={images[active].alt} className="max-h-[85vh] max-w-full object-contain rounded-xl" />
              : <span className="text-[12rem] select-none">{icon}</span>
            }
          </div>

          {count > 1 && (
            <p className="absolute bottom-4 text-white/60 text-sm">{active + 1} / {count}</p>
          )}
        </div>
      )}
    </>
  )
}
