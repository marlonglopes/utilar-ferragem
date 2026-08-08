import { useEffect, useRef, useState } from 'react'
import { RotateCcw, RotateCw, Check, Maximize2 } from 'lucide-react'
import { Modal, Button } from '@/components/ui'
import { editFile, type CropFrac } from '@/lib/imageEditor'

/**
 * Ajuste manual de UMA imagem antes do upload: rotação (90°) + recorte.
 *
 * O backend já normaliza para 1:1 e aplica a orientação EXIF — isto cobre o que
 * o automático não pega: foto girada sem EXIF (print/scan) e escolher a REGIÃO
 * do recorte, não o centro.
 *
 * O preview NÃO gira ao vivo de propósito: a caixa de recorte fica no espaço da
 * imagem original (arrastar é intuitivo, sem inverter direção por causa da
 * rotação), e a rotação é aplicada DEPOIS do recorte na saída — o mesmo que a
 * `imageEditor.editFile` faz. O estado da rotação aparece em texto.
 */

const FULL: CropFrac = { x: 0, y: 0, w: 1, h: 1 }
const MIN = 0.1 // recorte mínimo: 10% do lado, pra não gerar 1px sem querer.

function clamp(v: number, min: number, max: number) {
  return Math.max(min, Math.min(max, v))
}

function isFull(c: CropFrac) {
  return c.x === 0 && c.y === 0 && c.w === 1 && c.h === 1
}

export function ImageEditorModal({
  file,
  open,
  onApply,
  onCancel,
}: {
  file: File | null
  open: boolean
  onApply: (edited: File) => void
  onCancel: () => void
}) {
  const [url, setUrl] = useState('')
  const [rotation, setRotation] = useState(0)
  const [crop, setCrop] = useState<CropFrac>(FULL)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const wrapRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!file) return
    const u = URL.createObjectURL(file)
    setUrl(u)
    setRotation(0)
    setCrop(FULL)
    setError('')
    return () => URL.revokeObjectURL(u)
  }, [file])

  function startMove(e: React.PointerEvent) {
    e.preventDefault()
    const rect = wrapRef.current?.getBoundingClientRect()
    if (!rect || rect.width === 0) return
    const sx = e.clientX
    const sy = e.clientY
    const c0 = { ...crop }
    function move(ev: PointerEvent) {
      const dx = (ev.clientX - sx) / rect!.width
      const dy = (ev.clientY - sy) / rect!.height
      setCrop({
        ...c0,
        x: clamp(c0.x + dx, 0, 1 - c0.w),
        y: clamp(c0.y + dy, 0, 1 - c0.h),
      })
    }
    function up() {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }

  function startResize(e: React.PointerEvent) {
    e.preventDefault()
    e.stopPropagation()
    const rect = wrapRef.current?.getBoundingClientRect()
    if (!rect || rect.width === 0) return
    const sx = e.clientX
    const sy = e.clientY
    const c0 = { ...crop }
    function move(ev: PointerEvent) {
      const dw = (ev.clientX - sx) / rect!.width
      const dh = (ev.clientY - sy) / rect!.height
      setCrop({
        ...c0,
        w: clamp(c0.w + dw, MIN, 1 - c0.x),
        h: clamp(c0.h + dh, MIN, 1 - c0.y),
      })
    }
    function up() {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }

  async function apply() {
    if (!file) return
    setBusy(true)
    setError('')
    try {
      const edited = await editFile(file, {
        rotation,
        crop: isFull(crop) ? undefined : crop,
      })
      onApply(edited)
    } catch {
      setError('Não foi possível processar a imagem. Tente outra.')
    } finally {
      setBusy(false)
    }
  }

  const rotationLabel =
    rotation % 360 === 0 ? 'sem rotação' : `girada ${((rotation % 360) + 360) % 360}°`

  return (
    <Modal open={open} onClose={onCancel} title="Ajustar imagem" size="md">
      <div className="flex flex-col gap-4">
        <div className="flex justify-center overflow-hidden rounded-lg bg-gray-900/5 p-2">
          <div ref={wrapRef} className="relative inline-block leading-none">
            {url && (
              <img
                src={url}
                alt="Pré-visualização da imagem"
                draggable={false}
                className="block max-h-[50vh] max-w-full select-none"
              />
            )}
            {/* Caixa de recorte (espaço da imagem original). */}
            <div
              className="absolute cursor-move border-2 border-brand-orange bg-brand-orange/10"
              style={{
                left: `${crop.x * 100}%`,
                top: `${crop.y * 100}%`,
                width: `${crop.w * 100}%`,
                height: `${crop.h * 100}%`,
              }}
              onPointerDown={startMove}
              aria-label="Área de recorte (arraste para mover)"
            >
              <span
                onPointerDown={startResize}
                aria-label="Redimensionar recorte"
                className="absolute -bottom-2 -right-2 h-4 w-4 cursor-se-resize rounded-full border-2 border-white bg-brand-orange shadow"
              />
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => setRotation((r) => r - 90)}>
            <RotateCcw className="h-4 w-4" aria-hidden="true" />
            Girar à esquerda
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setRotation((r) => r + 90)}>
            <RotateCw className="h-4 w-4" aria-hidden="true" />
            Girar à direita
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setCrop(FULL)}
            disabled={isFull(crop)}
          >
            <Maximize2 className="h-4 w-4" aria-hidden="true" />
            Imagem inteira
          </Button>
          <span className="text-xs text-gray-500" data-testid="rotation-label">
            {rotationLabel}
          </span>
        </div>

        {error && (
          <p role="alert" className="text-sm font-semibold text-red-600">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <Button variant="secondary" fullWidth onClick={onCancel}>
            Cancelar
          </Button>
          <Button fullWidth loading={busy} onClick={() => void apply()}>
            <Check className="h-4 w-4" aria-hidden="true" />
            Usar imagem
          </Button>
        </div>
      </div>
    </Modal>
  )
}
