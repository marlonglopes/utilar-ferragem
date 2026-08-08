// Recorte e rotação de imagem NO CLIENTE, antes do upload.
//
// PORQUÊ: o backend já normaliza toda foto para 1:1 e aplica a orientação EXIF,
// então o essencial (quadrado + foto-de-lado de câmera) já é automático. Isto
// dá ao dono o controle MANUAL para os casos que o automático não cobre: uma
// foto que veio girada sem EXIF (print, scan, re-salva) ou cortar a bagunça em
// volta do produto escolhendo a região, não o centro.
//
// Duas passagens de canvas — RECORTA a imagem original, depois ROTACIONA o
// recorte. Nessa ordem (não o inverso) o recorte vive no espaço da imagem
// ORIGINAL, que é onde o usuário desenha a caixa; o preview então é só a caixa
// sobre a imagem + a rotação aplicada por CSS ao conjunto. Duas passagens
// simples valem mais que uma transform composta fácil de errar.

/** Retângulo de recorte em FRAÇÕES [0..1] das dimensões da imagem ORIGINAL. */
export interface CropFrac {
  x: number
  y: number
  w: number
  h: number
}

export interface ImageEdit {
  /** Rotação horária em graus: 0 | 90 | 180 | 270. */
  rotation: number
  /** Recorte; ausente ou {0,0,1,1} = imagem inteira. */
  crop?: CropFrac
}

const FULL: CropFrac = { x: 0, y: 0, w: 1, h: 1 }

function norm(rotation: number): number {
  return (((Math.round(rotation / 90) * 90) % 360) + 360) % 360
}

/**
 * outputSize é a geometria PURA (testável sem canvas): dado o tamanho da fonte,
 * a rotação e o recorte, qual o tamanho do resultado. A rotação de 90/270 troca
 * largura por altura; o recorte escala.
 */
export function outputSize(
  srcW: number,
  srcH: number,
  rotation: number,
  crop?: CropFrac
): { w: number; h: number } {
  const c = crop ?? FULL
  // Recorta primeiro (espaço da imagem original)…
  const cw = Math.max(1, Math.round(srcW * c.w))
  const ch = Math.max(1, Math.round(srcH * c.h))
  // …depois rotaciona: 90/270 troca largura por altura.
  const rot = norm(rotation)
  return rot === 90 || rot === 270 ? { w: ch, h: cw } : { w: cw, h: ch }
}

// rotateToCanvas desenha a imagem inteira já rotacionada (centra → rotaciona →
// desenha), num canvas com as dimensões rotacionadas.
function rotateToCanvas(img: CanvasImageSource & { width: number; height: number }, rot: number) {
  const rw = rot === 90 || rot === 270 ? img.height : img.width
  const rh = rot === 90 || rot === 270 ? img.width : img.height
  const c = document.createElement('canvas')
  c.width = rw
  c.height = rh
  const ctx = c.getContext('2d')
  if (!ctx) throw new Error('canvas 2d indisponível')
  ctx.translate(rw / 2, rh / 2)
  ctx.rotate((rot * Math.PI) / 180)
  ctx.drawImage(img, -img.width / 2, -img.height / 2)
  return c
}

/**
 * renderEditedBlob aplica rotação + recorte e devolve um Blob. Só roda em
 * navegador de verdade (canvas + toBlob) — coberto por e2e (imageEditor.spec.ts),
 * porque happy-dom não implementa canvas.
 */
export async function renderEditedBlob(
  img: CanvasImageSource & { width: number; height: number },
  edit: ImageEdit,
  mime = 'image/jpeg',
  quality = 0.9
): Promise<Blob> {
  const c = edit.crop ?? FULL
  // Passo 1: recorta a imagem original.
  const cx = Math.round(img.width * c.x)
  const cy = Math.round(img.height * c.y)
  const cw = Math.max(1, Math.round(img.width * c.w))
  const ch = Math.max(1, Math.round(img.height * c.h))
  const cropped = document.createElement('canvas')
  cropped.width = cw
  cropped.height = ch
  const cctx = cropped.getContext('2d')
  if (!cctx) throw new Error('canvas 2d indisponível')
  cctx.drawImage(img, cx, cy, cw, ch, 0, 0, cw, ch)

  // Passo 2: rotaciona o recorte.
  const out = rotateToCanvas(cropped, norm(edit.rotation))

  return await new Promise<Blob>((resolve, reject) => {
    out.toBlob((b) => (b ? resolve(b) : reject(new Error('toBlob devolveu null'))), mime, quality)
  })
}

/** loadImage decodifica um File numa imagem pronta pro canvas (com orientação EXIF). */
export async function loadImage(file: File): Promise<ImageBitmap> {
  return createImageBitmap(file, { imageOrientation: 'from-image' })
}

/** editFile é o atalho: File dentro → File editado fora (jpeg). */
export async function editFile(file: File, edit: ImageEdit): Promise<File> {
  const img = await loadImage(file)
  const blob = await renderEditedBlob(img, edit, 'image/jpeg', 0.9)
  img.close?.()
  const name = file.name.replace(/\.[^.]+$/, '') + '.jpg'
  return new File([blob], name, { type: 'image/jpeg', lastModified: file.lastModified })
}
