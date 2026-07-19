import { useCallback, useRef, useState } from 'react'
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  ImageOff,
  Loader2,
  Star,
  Trash2,
  Upload,
} from 'lucide-react'
import { cn } from '@/lib/cn'
import { Section } from '@/components/admin/primitives'
import { formatPercent } from '@/lib/adminFormat'
import {
  byteSavings,
  formatBytes,
  hasVariants,
  imageSrc,
  isServerSideRejection,
  rejectLabel,
} from '@/lib/adminProductFormat'
import { useProductImages } from '@/hooks/useAdminProducts'
import type { ProductImageRecord } from '@/lib/adminProductTypes'

/**
 * Galeria de imagens do produto.
 *
 * O pipeline de upload existe no backend há semanas sem nenhuma interface —
 * esta é ela. As decisões que ela carrega:
 *
 * - **Várias imagens de uma vez.** O campo `files` é repetível justamente para
 *   isso; obrigar a enviar uma por vez desperdiçaria o desenho do backend e
 *   faria o cadastro de um produto com 6 fotos custar 6 esperas.
 * - **A primeira é a capa**, e isso é dito na tela, não deduzido. O contrato de
 *   `PUT .../images/order` é explícito ("o primeiro vira a capa"), e há também
 *   um botão de capa por imagem — arrastar é preciso demais para ser o único
 *   caminho num celular.
 * - **Motivo por arquivo.** "3 de 5 imagens falharam" é inútil. Cada recusa
 *   aparece com o nome do arquivo e o que fazer a respeito, casando pelo `code`
 *   estável e nunca pelo texto da mensagem.
 * - **Imagem externa não quebra.** As fotos CC0 do Wikimedia não têm
 *   `variants`; a tela as marca como externas e omite o antes/depois em vez de
 *   tentar ler um campo que não existe.
 */

// ---------------------------------------------------------------------------
// Cartão de uma imagem
// ---------------------------------------------------------------------------

function ImageCard({
  image,
  index,
  total,
  busy,
  onMove,
  onCover,
  onRemove,
  onDragStart,
  onDragOver,
  onDrop,
  dragging,
}: {
  image: ProductImageRecord
  index: number
  total: number
  busy: boolean
  onMove: (from: number, to: number) => void
  onCover: (id: string) => void
  onRemove: (id: string) => void
  onDragStart: (index: number) => void
  onDragOver: (index: number) => void
  onDrop: () => void
  dragging: boolean
}) {
  const isCover = index === 0
  const own = hasVariants(image)
  const savings = byteSavings(image.originalBytes, image.bytes)

  return (
    <li
      draggable={!busy}
      onDragStart={() => onDragStart(index)}
      onDragOver={(e) => {
        e.preventDefault()
        onDragOver(index)
      }}
      onDrop={(e) => {
        e.preventDefault()
        onDrop()
      }}
      className={cn(
        'relative flex flex-col rounded-lg border bg-white transition-shadow',
        isCover ? 'border-brand-blue ring-1 ring-brand-blue/30' : 'border-gray-200',
        dragging && 'opacity-50',
        !busy && 'cursor-grab active:cursor-grabbing',
      )}
      data-testid={`image-card-${image.id}`}
    >
      <div className="relative aspect-square w-full overflow-hidden rounded-t-lg bg-gray-100">
        <img
          // `thumb` no cartão: é o motivo de as variantes existirem. Servir a
          // imagem de zoom numa grade de miniaturas é o que trava o 4G.
          src={imageSrc(image, 'thumb')}
          alt={image.alt || 'imagem do produto'}
          loading="lazy"
          className="h-full w-full object-contain"
        />
        {isCover && (
          <span className="absolute left-1.5 top-1.5 inline-flex items-center gap-1 rounded bg-brand-blue px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-white">
            <Star className="h-3 w-3" aria-hidden="true" />
            Capa
          </span>
        )}
        {!own && (
          <span
            className="absolute right-1.5 top-1.5 rounded bg-gray-800/80 px-1.5 py-0.5 text-[10px] font-semibold text-white"
            title="Imagem hospedada fora (URL de terceiro). Não passou pela normalização e não tem variantes."
          >
            Externa
          </span>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-1.5 p-2">
        <p className="truncate text-xs text-gray-600" title={image.alt}>
          {image.alt || <span className="text-gray-400">sem descrição</span>}
        </p>

        {own ? (
          <div className="text-[11px] leading-tight text-gray-500">
            {image.width && image.height && (
              <p className="tabular-nums">
                {image.width}×{image.height} px
              </p>
            )}
            {image.originalBytes && image.bytes && (
              <p className="tabular-nums">
                {formatBytes(image.originalBytes)} → {formatBytes(image.bytes)}
                {savings !== null && (
                  <span className="ml-1 font-semibold text-green-700">
                    −{formatPercent(savings, 0)}
                  </span>
                )}
              </p>
            )}
          </div>
        ) : (
          <p className="text-[11px] leading-tight text-gray-500">
            Sem variantes — servida direto da origem.
          </p>
        )}

        <div className="mt-auto flex items-center justify-between gap-1 pt-1">
          <div className="flex items-center gap-0.5">
            <button
              type="button"
              disabled={busy || index === 0}
              onClick={() => onMove(index, index - 1)}
              aria-label={`Mover ${image.alt || 'imagem'} para a esquerda`}
              className="rounded p-1 text-gray-500 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-30"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              disabled={busy || index === total - 1}
              onClick={() => onMove(index, index + 1)}
              aria-label={`Mover ${image.alt || 'imagem'} para a direita`}
              className="rounded p-1 text-gray-500 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-30"
            >
              <ArrowRight className="h-3.5 w-3.5" />
            </button>
          </div>
          <div className="flex items-center gap-0.5">
            {!isCover && (
              <button
                type="button"
                disabled={busy}
                onClick={() => onCover(image.id)}
                aria-label={`Definir ${image.alt || 'imagem'} como capa`}
                className="rounded p-1 text-gray-500 hover:bg-amber-50 hover:text-amber-700 disabled:opacity-30"
              >
                <Star className="h-3.5 w-3.5" />
              </button>
            )}
            <button
              type="button"
              disabled={busy}
              onClick={() => onRemove(image.id)}
              aria-label={`Remover ${image.alt || 'imagem'}`}
              className="rounded p-1 text-gray-500 hover:bg-red-50 hover:text-red-700 disabled:opacity-30"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      </div>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Gerenciador
// ---------------------------------------------------------------------------

export function ProductImageManager({ productId }: { productId: string }) {
  const g = useProductImages(productId)
  const inputRef = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const dragFrom = useRef<number | null>(null)
  const dropTarget = useRef<number | null>(null)
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)

  const busy = g.uploading || g.busy

  const pick = useCallback(
    (list: FileList | null) => {
      if (!list || list.length === 0) return
      void g.upload(Array.from(list))
      // Limpa o input: sem isso, escolher o MESMO arquivo de novo (depois de
      // corrigi-lo) não dispara `change` e o dono acha que a tela travou.
      if (inputRef.current) inputRef.current.value = ''
    },
    [g],
  )

  const onDrop = useCallback(() => {
    const from = dragFrom.current
    setDraggingIndex(null)
    dragFrom.current = null
    if (from === null || dropTarget.current === null) return
    void g.move(from, dropTarget.current)
    dropTarget.current = null
  }, [g])

  return (
    <Section
      title="Imagens"
      description="A primeira imagem é a capa da vitrine. Arraste para reordenar."
      actions={
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          disabled={busy}
          className="inline-flex items-center gap-1.5 rounded-md bg-brand-blue px-3 py-1.5 text-xs font-semibold text-white hover:bg-brand-blue/90 disabled:cursor-not-allowed disabled:bg-gray-300"
        >
          {g.uploading ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
          ) : (
            <Upload className="h-3.5 w-3.5" aria-hidden="true" />
          )}
          {g.uploading ? 'Enviando…' : 'Escolher imagens'}
        </button>
      }
    >
      <div className="space-y-3 p-3 sm:p-4">
        <input
          ref={inputRef}
          type="file"
          accept="image/jpeg,image/png,image/webp,image/gif"
          multiple
          className="sr-only"
          aria-label="Escolher imagens do produto"
          onChange={(e) => pick(e.target.files)}
        />

        {/* Área de soltar arquivos --------------------------------------- */}
        <div
          onDragOver={(e) => {
            e.preventDefault()
            setOver(true)
          }}
          onDragLeave={() => setOver(false)}
          onDrop={(e) => {
            e.preventDefault()
            setOver(false)
            pick(e.dataTransfer.files)
          }}
          className={cn(
            'rounded-lg border-2 border-dashed px-4 py-6 text-center transition-colors',
            over ? 'border-brand-blue bg-brand-blue-light/40' : 'border-gray-300 bg-gray-50',
          )}
          data-testid="image-dropzone"
        >
          <Upload className="mx-auto h-6 w-6 text-gray-400" aria-hidden="true" />
          <p className="mt-1.5 text-sm font-medium text-gray-700">
            Arraste várias imagens aqui de uma vez
          </p>
          <p className="mt-0.5 text-xs text-gray-500">
            JPEG, PNG, WebP ou GIF · até 12 MB por arquivo · até 20 por envio. Todas são recortadas
            em quadrado, com fundo branco, e ganham três tamanhos automaticamente.
          </p>
        </div>

        {/* Erro de rede/permissão ---------------------------------------- */}
        {g.error && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md border border-red-200 border-l-4 border-l-red-600 bg-red-50 p-3"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-700" aria-hidden="true" />
            <p className="text-xs leading-relaxed text-red-800">{g.error}</p>
          </div>
        )}

        {/* Recusas por arquivo -------------------------------------------- */}
        {g.rejections.length > 0 && (
          <div
            role="alert"
            data-testid="image-rejections"
            className="rounded-md border border-amber-200 border-l-4 border-l-amber-500 bg-amber-50/70 p-3"
          >
            <p className="text-sm font-semibold text-amber-900">
              {g.lastAccepted > 0
                ? `${g.lastAccepted} ${g.lastAccepted === 1 ? 'imagem entrou' : 'imagens entraram'}; ${g.rejections.length} ${g.rejections.length === 1 ? 'foi recusada' : 'foram recusadas'}`
                : `${g.rejections.length} ${g.rejections.length === 1 ? 'imagem foi recusada' : 'imagens foram recusadas'}`}
            </p>
            <ul className="mt-2 space-y-1.5">
              {g.rejections.map((r, i) => (
                <li key={`${r.filename}-${i}`} className="text-xs leading-relaxed">
                  <span className="font-mono font-semibold text-amber-900">{r.filename}</span>
                  <span className="text-amber-900"> — {rejectLabel(r.code, r.reason)}</span>
                  {isServerSideRejection(r.code) && (
                    <span className="ml-1 rounded bg-amber-200/60 px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-900">
                      tente de novo
                    </span>
                  )}
                </li>
              ))}
            </ul>
            <button
              type="button"
              onClick={g.dismissRejections}
              className="mt-2 rounded border border-amber-300 px-2 py-1 text-xs font-semibold text-amber-900 hover:bg-amber-100"
            >
              Entendi
            </button>
          </div>
        )}

        {/* Grade ---------------------------------------------------------- */}
        {g.loading ? (
          <p className="py-6 text-center text-xs text-gray-500" aria-busy="true">
            Carregando imagens…
          </p>
        ) : g.images.length === 0 ? (
          <div className="py-8 text-center">
            <ImageOff className="mx-auto h-8 w-8 text-gray-300" aria-hidden="true" />
            <p className="mt-2 text-sm font-medium text-gray-700">Nenhuma imagem ainda</p>
            <p className="mt-0.5 text-xs text-gray-500">
              Produto sem foto converte muito menos. Envie ao menos a capa antes de publicar.
            </p>
          </div>
        ) : (
          <ul
            className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5"
            data-testid="image-grid"
          >
            {g.images.map((img, i) => (
              <ImageCard
                key={img.id}
                image={img}
                index={i}
                total={g.images.length}
                busy={busy}
                onMove={(from, to) => void g.move(from, to)}
                onCover={(id) => void g.makeCover(id)}
                onRemove={(id) => void g.remove(id)}
                onDragStart={(idx) => {
                  dragFrom.current = idx
                  setDraggingIndex(idx)
                }}
                onDragOver={(idx) => {
                  dropTarget.current = idx
                }}
                onDrop={onDrop}
                dragging={draggingIndex === i}
              />
            ))}
          </ul>
        )}
      </div>
    </Section>
  )
}

export default ProductImageManager
