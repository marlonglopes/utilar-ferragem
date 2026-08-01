// Compressão de imagem NO CLIENTE, antes do upload.
//
// PORQUÊ: foto de celular vem com 8-12 MB. Subir 480 dessas no 4G da loja é
// sofrimento. Reduzimos para no máximo 2000px no maior lado e reencodamos em
// JPEG. O backend ainda normaliza tudo (1:1, variantes, EXIF) — isto é só para
// não trafegar o arquivo cru gigante.
//
// ⚠️ EXIF: reencodar via canvas DESCARTA o EXIF, inclusive a orientação. Se não
// aplicássemos a orientação antes, a foto tirada "de lado" no celular chegaria
// deitada ao backend SEM o EXIF para consertar. Por isso pedimos o bitmap já
// ORIENTADO (`imageOrientation: 'from-image'`) e desenhamos ele — a orientação
// entra nos pixels e o EXIF vira dispensável.

const MAX_SIDE = 2000
const QUALITY = 0.85
// Abaixo disto (e já dentro do limite de tamanho), não vale reencodar.
const SKIP_UNDER_BYTES = 1_200_000

export async function compressImage(file: File): Promise<File> {
  // Só raster comum. GIF (animação) e SVG passam intactos.
  if (
    !file.type.startsWith('image/') ||
    file.type === 'image/gif' ||
    file.type === 'image/svg+xml'
  ) {
    return file
  }
  if (typeof createImageBitmap !== 'function' || typeof document === 'undefined') {
    return file // ambiente sem canvas (ex.: teste em node) — deixa pro backend
  }

  let bitmap: ImageBitmap
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch {
    return file // não decodificou aqui; o backend tenta
  }

  const { width, height } = bitmap
  const scale = Math.min(1, MAX_SIDE / Math.max(width, height))

  // Já pequena e leve: fecha e devolve o original (mantém o formato).
  if (scale >= 1 && file.size < SKIP_UNDER_BYTES) {
    bitmap.close?.()
    return file
  }

  const w = Math.max(1, Math.round(width * scale))
  const h = Math.max(1, Math.round(height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    bitmap.close?.()
    return file
  }
  ctx.drawImage(bitmap, 0, 0, w, h)
  bitmap.close?.()

  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, 'image/jpeg', QUALITY)
  )
  if (!blob || blob.size >= file.size) {
    // Reencodar não ajudou (ex.: PNG pequeno vira JPEG maior) — fica o original.
    return file
  }
  const name = file.name.replace(/\.[^.]+$/, '') + '.jpg'
  return new File([blob], name, { type: 'image/jpeg', lastModified: file.lastModified })
}
